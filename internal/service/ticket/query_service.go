package ticket

import (
	"context"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	ratingSvc "github.com/Duke1616/eflow/internal/service/rating"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

//go:generate mockgen -source=./query_service.go -package=ticketmocks -destination=./mocks/query_svc.mock.go QueryService

// QueryService 负责工单读场景的多源数据聚合门面（工单实体 + 工作流引擎步骤 + 用户微服务昵称 + 评价）
type QueryService interface {
	// ListUserInitiated 分页查询指定用户发起的工单列表（聚合流转状态与当前待办步骤）
	ListUserInitiated(ctx context.Context, username string, offset, limit int64) ([]domain.TicketTaskView, int64, error)
	// ListUserTodo 分页查询指派给当前用户的待办审批任务
	ListUserTodo(ctx context.Context, username, processName string, sortByAsc bool, offset, limit int64) ([]domain.TicketTaskView, int64, error)
	// ListAllTodo 分页查询系统全量待办任务（包含流程启动失败置顶提示）
	ListAllTodo(ctx context.Context, userID, processName string, sortByAsc bool, offset, limit int64) ([]domain.TicketTaskView, int64, error)
	// ListHistory 分页查询历史已办结/已撤销工单（聚合评价与提单人信息）
	ListHistory(ctx context.Context, currentUsername, targetUserID string, offset, limit int64) ([]domain.TicketTaskView, int64, error)
}

type queryService struct {
	repo      repository.TicketRepository
	engineSvc engineSvc.Service
	userSvc   userv1.UserServiceClient
	ratingSvc ratingSvc.Service
	logger    *elog.Component
}

// NewQueryService 实例化工单读场景聚合门面服务
func NewQueryService(repo repository.TicketRepository, engineSvc engineSvc.Service,
	userSvc userv1.UserServiceClient, ratingSvc ratingSvc.Service) QueryService {
	return &queryService{
		repo:      repo,
		engineSvc: engineSvc,
		userSvc:   userSvc,
		ratingSvc: ratingSvc,
		logger:    elog.DefaultLogger,
	}
}

// ListUserInitiated 查询当前登录用户发起的工单列表，以业务工单为主干反查引擎待办步骤与处理人真名
func (q *queryService) ListUserInitiated(ctx context.Context, username string, offset, limit int64) ([]domain.TicketTaskView, int64, error) {
	statuses := []int{
		domain.START.ToInt(),
		domain.PROCESS.ToInt(),
		domain.WITHDRAWING.ToInt(),
		domain.START_FAILED.ToInt(),
	}

	// 1. 并发查询用户工单主记录与总数，降低数据库 I/O 耗时
	tickets, total, err := fetchPagedParallel(
		func() ([]domain.Ticket, error) {
			return q.repo.ListTicket(ctx, username, statuses, offset, limit)
		},
		func() (int64, error) {
			return q.repo.CountTicket(ctx, username, statuses)
		},
	)
	if err != nil {
		return nil, 0, err
	}
	if len(tickets) == 0 {
		return []domain.TicketTaskView{}, total, nil
	}

	// 2. 提取有效流程实例 ID，反查流程引擎挂起的待办步骤
	procInstIds := lo.FilterMap(tickets, func(src domain.Ticket, _ int) (int, bool) {
		return src.Process.InstanceId, src.Process.InstanceId > 0
	})

	processTasks, err := q.engineSvc.ListPendingStepsOfMyTask(ctx, procInstIds, username)
	if err != nil {
		return nil, 0, err
	}

	tasksByProcInst := lo.GroupBy(processTasks, func(inst domain.Instance) int {
		return inst.ProcInstID
	})

	// 3. 批量解析待办处理人真实姓名字典
	displayNames := q.resolveDisplayNames(ctx, processTasks)

	// 4. 以工单实体为基准 1:1 装配，确保没有未完工人工待办的任务（如已撤回、自动化中）不被意外漏掉
	views := lo.Map(tickets, func(ticket domain.Ticket, _ int) domain.TicketTaskView {
		view := domain.TicketTaskView{Ticket: ticket}
		view.ResolveStep(tasksByProcInst[ticket.Process.InstanceId], displayNames)
		return view
	})

	return views, total, nil
}

// ListUserTodo 分页检索指派给当前用户的待办任务，反查关联工单信息
func (q *queryService) ListUserTodo(ctx context.Context, username, processName string, sortByAsc bool, offset, limit int64) ([]domain.TicketTaskView, int64, error) {
	instances, total, err := q.engineSvc.ListTodoTasks(ctx, username, processName, sortByAsc, int(offset), int(limit))
	if err != nil {
		return nil, 0, err
	}
	views, err := q.assembleEngineInstances(ctx, instances)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// ListAllTodo 分页检索全量待办任务，首屏自动置顶流程启动失败的异常单据并做溢出截断保护
func (q *queryService) ListAllTodo(ctx context.Context, userID, processName string, sortByAsc bool, offset, limit int64) ([]domain.TicketTaskView, int64, error) {
	instances, total, err := q.engineSvc.ListAllTodoTasks(ctx, userID, processName, sortByAsc, int(offset), int(limit))
	if err != nil {
		return nil, 0, err
	}

	views, err := q.assembleEngineInstances(ctx, instances)
	if err != nil {
		return nil, 0, err
	}

	// NOTE: 流程启动失败的工单需要高优先级置顶提示，让运维或业务人员第一时间重试或排查
	if offset == 0 {
		failedTickets, failedErr := q.repo.ListProcessStartFailed(ctx, "", 0, limit)
		if failedErr != nil {
			return nil, 0, failedErr
		}
		failedCount, countErr := q.repo.CountProcessStartFailed(ctx, "")
		if countErr == nil {
			total += failedCount
		}
		failedViews := lo.Map(failedTickets, func(t domain.Ticket, _ int) domain.TicketTaskView {
			return domain.TicketTaskView{Ticket: t, CurrentStep: "流程启动失败"}
		})
		views = append(failedViews, views...)
	} else {
		failedCount, countErr := q.repo.CountProcessStartFailed(ctx, "")
		if countErr == nil {
			total += failedCount
		}
	}

	// 保护机制：追加置顶记录后严格截断，防止单页数量超出请求 limit 破坏前端分页
	if limit > 0 && int64(len(views)) > limit {
		views = views[:limit]
	}

	return views, total, nil
}

// ListHistory 分页检索已结束/已撤销的历史工单，同时挂载用户评分与提单人真名
func (q *queryService) ListHistory(ctx context.Context, currentUsername, targetUserID string, offset, limit int64) ([]domain.TicketTaskView, int64, error) {
	endStatuses := []int{domain.END.ToInt(), domain.WITHDRAW.ToInt()}

	// 并发拉取历史工单列表切片与可见总数，消除样板代码
	tickets, total, err := fetchPagedParallel(
		func() ([]domain.Ticket, error) {
			return q.repo.ListHistory(ctx, targetUserID, endStatuses, offset, limit)
		},
		func() (int64, error) {
			return q.repo.CountHistory(ctx, targetUserID, endStatuses)
		},
	)
	if err != nil {
		return nil, 0, err
	}

	// 批量反查评分记录与提单人展示名，杜绝循环单次查询产生的 N+1 问题
	ticketIDs := lo.Map(tickets, func(t domain.Ticket, _ int) int64 { return t.Id })
	ratings, err := q.ratingSvc.ListByTicketIDs(ctx, ticketIDs)
	if err != nil {
		ratings = make(map[int64]domain.TicketRating)
	}

	creatorNames := lo.Uniq(lo.Map(tickets, func(t domain.Ticket, _ int) string { return t.CreateBy }))
	userMap := q.getUserMap(ctx, creatorNames)

	views := lo.Map(tickets, func(src domain.Ticket, _ int) domain.TicketTaskView {
		starter := src.CreateBy
		if displayName, ok := userMap[src.CreateBy]; ok && displayName != "" {
			starter = displayName
		}
		viewTicket := src
		viewTicket.CreateBy = starter

		rating, rated := ratings[src.Id]
		canRate := src.Status == domain.END && src.CreateBy == currentUsername && !rated
		view := domain.TicketTaskView{
			Ticket:  viewTicket,
			CanRate: canRate,
		}
		if rated {
			view.Rating = &rating
		}
		return view
	})

	return views, total, nil
}

// =============================================================================
// 内部数据装配与用户昵称解析辅助函数
// =============================================================================

// assembleEngineInstances 负责将引擎实例列表与工单主表实体进行关联对齐
func (q *queryService) assembleEngineInstances(ctx context.Context, instances []domain.Instance) ([]domain.TicketTaskView, error) {
	if len(instances) == 0 {
		return []domain.TicketTaskView{}, nil
	}

	procInstIds := lo.Uniq(lo.Map(instances, func(inst domain.Instance, _ int) int {
		return inst.ProcInstID
	}))

	displayNames := q.resolveDisplayNames(ctx, instances)

	tickets, err := q.repo.ListTicketByProcessInstanceIds(ctx, procInstIds)
	if err != nil {
		return nil, err
	}
	ticketsByProcInst := lo.KeyBy(tickets, func(t domain.Ticket) int {
		return t.Process.InstanceId
	})

	return lo.Map(instances, func(inst domain.Instance, _ int) domain.TicketTaskView {
		view := domain.TicketTaskView{
			TaskID:             inst.TaskID,
			ProcInstCreateTime: inst.CreateTime,
			CurrentStep:        inst.CurrentNodeName,
		}
		if name, ok := displayNames[inst.ApprovedBy]; ok && name != "" {
			view.ApprovedBy = name
		} else {
			view.ApprovedBy = inst.ApprovedBy
		}

		if matchedTicket, ok := ticketsByProcInst[inst.ProcInstID]; ok {
			view.Ticket = matchedTicket
		}
		return view
	}), nil
}

// resolveDisplayNames 批量反查步骤集合中涉及的处理人展示姓名
func (q *queryService) resolveDisplayNames(ctx context.Context, instances []domain.Instance) map[string]string {
	nonEmptyUsers := lo.FilterMap(instances, func(inst domain.Instance, _ int) (string, bool) {
		return inst.ApprovedBy, inst.ApprovedBy != ""
	})
	return q.getUserMap(ctx, lo.Uniq(nonEmptyUsers))
}

// getUserMap 调取用户服务批量映射账号名称；如果调用失败则平滑降级，不阻塞核心流转查询
func (q *queryService) getUserMap(ctx context.Context, usernames []string) map[string]string {
	if len(usernames) == 0 {
		return make(map[string]string)
	}
	resp, err := q.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{Usernames: usernames})
	if err != nil || resp == nil {
		return make(map[string]string)
	}
	return lo.Associate(resp.Users, func(u *userv1.User) (string, string) {
		return u.Username, u.DisplayName
	})
}

// fetchPagedParallel 通用轻量分页并发拉取辅助函数，将明细切片与记录总数解耦并行查询，消除样板代码
func fetchPagedParallel[T any](
	fetchList func() ([]T, error),
	fetchCount func() (int64, error),
) ([]T, int64, error) {
	var (
		eg    errgroup.Group
		items []T
		total int64
	)
	eg.Go(func() (err error) {
		items, err = fetchList()
		return err
	})
	eg.Go(func() (err error) {
		total, err = fetchCount()
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}
