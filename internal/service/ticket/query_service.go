package ticket

import (
	"context"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
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
	// TaskRecord 分页查询经过工单权限约束的任务流转执行记录，并聚合处理人真名与步骤表单
	TaskRecord(ctx context.Context, processInstID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error)
	// TaskTimeline 以节点执行批次为单位聚合可见任务时间线事件，聚合审批汇总与参与人真名
	TaskTimeline(ctx context.Context, processInstID domain.ID, offset, limit int) ([]domain.TaskTimelineEvent, int64, error)
	// GetLatestReject 解析流程实例最近一次被驳回的快照信息
	GetLatestReject(ctx context.Context, processInstID domain.ID) (*domain.LatestReject, error)
}

type queryService struct {
	repo      repository.TicketRepository
	engine    engine.IEngine
	userSvc   userv1.UserServiceClient
	ratingSvc ratingSvc.Service
	logger    *elog.Component
}

// NewQueryService 实例化工单读场景聚合门面服务
func NewQueryService(repo repository.TicketRepository, engine engine.IEngine,
	userSvc userv1.UserServiceClient, ratingSvc ratingSvc.Service) QueryService {
	return &queryService{
		repo:      repo,
		engine:    engine,
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
	return q.assembleTicketViews(ctx, tickets, total)
}

// ListUserTodo 分页检索指派给当前用户的待办任务，委托统一流程引擎门面进行多态并发聚合
func (q *queryService) ListUserTodo(ctx context.Context, username, processName string, sortByAsc bool, offset, limit int64) ([]domain.TicketTaskView, int64, error) {
	todos, total, err := q.engine.ListTodoTasks(ctx, domain.TodoQueryReq{
		Username:    username,
		ProcessName: processName,
		SortByAsc:   sortByAsc,
		Offset:      int(offset),
		Limit:       int(limit),
	})
	if err != nil {
		return nil, 0, err
	}

	views, err := q.assemblePendingTasks(ctx, todos)
	if err != nil {
		return nil, 0, err
	}
	return views, total, nil
}

// ListAllTodo 查询系统全量流转中的待办工单列表，以业务工单主表为权威事实源，天然实现全局时间倒序与精准分页
func (q *queryService) ListAllTodo(ctx context.Context, _ string, _ string, _ bool, offset, limit int64) ([]domain.TicketTaskView, int64, error) {
	statuses := []int{
		domain.START.ToInt(),
		domain.PROCESS.ToInt(),
		domain.WITHDRAWING.ToInt(),
		domain.START_FAILED.ToInt(),
	}

	// 1. 并发查询全量流转中工单主记录与总数，天然按 ctime 倒序排列，分页绝对精确
	tickets, total, err := fetchPagedParallel(
		func() ([]domain.Ticket, error) {
			return q.repo.ListTicket(ctx, "", statuses, offset, limit)
		},
		func() (int64, error) {
			return q.repo.CountTicket(ctx, "", statuses)
		},
	)
	if err != nil {
		return nil, 0, err
	}
	return q.assembleTicketViews(ctx, tickets, total)
}

func (q *queryService) assembleTicketViews(ctx context.Context, tickets []domain.Ticket, total int64) ([]domain.TicketTaskView, int64, error) {
	if len(tickets) == 0 {
		return []domain.TicketTaskView{}, total, nil
	}

	// 1. 提取有效绑定的流程实例（包含所属的 EngineType），定向反查待办步骤
	processes := lo.Filter(
		lo.Map(tickets, func(src domain.Ticket, _ int) domain.Process { return src.Process }),
		func(p domain.Process, _ int) bool { return p.IsBound() },
	)

	processTasks, err := q.engine.ListPendingSteps(ctx, processes)
	if err != nil {
		return nil, 0, err
	}

	tasksByProcInst := lo.GroupBy(processTasks, func(t domain.TodoTask) string {
		return t.InstanceID.String()
	})

	// 2. 批量解析待办处理人与工单提单人的展示姓名
	approvers := lo.FilterMap(processTasks, func(t domain.TodoTask, _ int) (string, bool) {
		return t.ApprovedBy, t.ApprovedBy != ""
	})
	starters := lo.FilterMap(tickets, func(t domain.Ticket, _ int) (string, bool) {
		return t.CreateBy, t.CreateBy != ""
	})
	displayNames := q.getUserMap(ctx, lo.Uniq(append(approvers, starters...)))

	// 3. 以工单实体为基准展开待办视图，将提单人映射为真实展示姓名；当存在并行多分支时展开为多条独立的视图记录供前端进行 Rowspan 合并
	views := lo.FlatMap(tickets, func(ticket domain.Ticket, _ int) []domain.TicketTaskView {
		viewTicket := ticket.WithDisplayName(displayNames)
		return viewTicket.ResolveViews(tasksByProcInst[ticket.Process.InstanceID.String()], displayNames)
	})

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
		rating, rated := ratings[src.Id]
		view := domain.TicketTaskView{
			Ticket:  src.WithDisplayName(userMap),
			CanRate: src.CanBeRatedBy(currentUsername, rated),
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

// pendingTaskItem 跨引擎待办任务统一轻量中间件模型
// assemblePendingTasks 负责将跨引擎待办领域模型列表与工单主表实体进行关联对齐与真名组装
func (q *queryService) assemblePendingTasks(ctx context.Context, todos []domain.TodoTask) ([]domain.TicketTaskView, error) {
	if len(todos) == 0 {
		return []domain.TicketTaskView{}, nil
	}

	// 1. 统一提取待办流程实例全局标识，消除对底层存储字段差异的分流与重复查询
	instIDs := lo.Uniq(lo.FilterMap(todos, func(t domain.TodoTask, _ int) (string, bool) {
		id := t.InstanceID.String()
		return id, id != ""
	}))

	tickets, err := q.repo.ListTicketByProcessInstances(ctx, instIDs)
	if err != nil {
		return nil, err
	}

	ticketsByInstID := lo.KeyBy(tickets, func(t domain.Ticket) string {
		return t.Process.InstanceID.String()
	})

	// 2. 批量并发解析处理人与提单人真名
	approvers := lo.FilterMap(todos, func(t domain.TodoTask, _ int) (string, bool) {
		return t.ApprovedBy, t.ApprovedBy != ""
	})
	starters := lo.FilterMap(tickets, func(t domain.Ticket, _ int) (string, bool) {
		return t.CreateBy, t.CreateBy != ""
	})
	displayNames := q.getUserMap(ctx, lo.Uniq(append(approvers, starters...)))

	// 3. 统一装配任务视图并映射展示姓名
	return lo.Map(todos, func(t domain.TodoTask, _ int) domain.TicketTaskView {
		view := domain.TicketTaskView{
			TaskID:      t.TaskID,
			CurrentStep: t.NodeName,
		}
		if !t.CreateTime.IsZero() {
			view.ProcInstCreateTime = &t.CreateTime
		}
		if name, ok := displayNames[t.ApprovedBy]; ok && name != "" {
			view.ApprovedBy = name
		} else {
			view.ApprovedBy = t.ApprovedBy
		}

		if matchedTicket, found := ticketsByInstID[t.InstanceID.String()]; found {
			view.Ticket = matchedTicket.WithDisplayName(displayNames)
		}
		return view
	}), nil
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

// TaskRecord 查询指定流程实例的流转执行记录，并聚合处理人真名与步骤表单
func (q *queryService) TaskRecord(ctx context.Context, processInstID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error) {
	engineType := q.resolveEngineType(ctx, processInstID)
	ts, total, err := q.engine.GetTaskRecords(ctx, engineType, processInstID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	records := q.toTaskRecords(ctx, ts)
	return records, total, nil
}

// TaskTimeline 以节点执行批次为单位返回流转历史时间线，聚合审批汇总与参与人真名
func (q *queryService) TaskTimeline(ctx context.Context, processInstID domain.ID, offset, limit int) ([]domain.TaskTimelineEvent, int64, error) {
	engineType := q.resolveEngineType(ctx, processInstID)
	groups, total, err := q.engine.GetTaskTimeline(ctx, engineType, processInstID, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	events := q.assembleTimelineEvents(ctx, groups)
	return events, total, nil
}

// resolveEngineType 根据流程实例标识反查工单绑定的流程引擎驱动类型，未找到或老数据为空时兜底为 EasyFlow
func (q *queryService) resolveEngineType(ctx context.Context, processInstID domain.ID) domain.EngineType {
	ticket, err := q.repo.DetailByProcessInstance(ctx, processInstID)
	if err == nil && ticket.Process.EngineType != "" {
		return ticket.Process.EngineType
	}
	return domain.EngineTypeEasyFlow
}

func (q *queryService) assembleTimelineEvents(ctx context.Context, groups []domain.TaskTimelineGroup) []domain.TaskTimelineEvent {
	allMembers := lo.FlatMap(groups, func(g domain.TaskTimelineGroup, _ int) []domain.TaskRecord {
		return g.Members
	})
	allRecords := q.toTaskRecords(ctx, allMembers)
	recordsMap := make(map[domain.ID]domain.TaskRecord, len(allMembers))
	for idx, record := range allRecords {
		recordsMap[allMembers[idx].TaskID] = record
	}

	return lo.Map(groups, func(group domain.TaskTimelineGroup, _ int) domain.TaskTimelineEvent {
		members := lo.Map(group.Members, func(m domain.TaskRecord, _ int) domain.TaskRecord {
			return recordsMap[m.TaskID]
		})
		return domain.NewTaskTimelineEvent(
			group.NodeID,
			group.NodeName,
			group.BatchCode,
			group.IsCosigned,
			group.OccurredAt,
			domain.TaskTimelineSummary{
				Total:          group.TaskCount,
				Passed:         group.PassedCount,
				Rejected:       group.RejectedCount,
				SystemPassed:   group.SystemPassedCount,
				SystemRejected: group.SystemRejectedCount,
				Skipped:        group.SkippedCount,
				Linked:         group.LinkedCount,
				Pending:        group.PendingCount,
			},
			members,
		)
	})
}

func (q *queryService) toTaskRecords(ctx context.Context, ts []domain.TaskRecord) []domain.TaskRecord {
	uniqueUserIDs := lo.Uniq(lo.FilterMap(ts, func(src domain.TaskRecord, _ int) (string, bool) {
		return src.ApprovedBy, src.ApprovedBy != ""
	}))

	uMap := q.getUserMap(ctx, uniqueUserIDs)

	taskIDs := lo.FilterMap(ts, func(src domain.TaskRecord, _ int) (int, bool) {
		idInt := src.TaskID.Int()
		return idInt, idInt > 0
	})
	taskDataMap, err := q.repo.FindTaskFormsBatch(ctx, taskIDs)
	if err != nil {
		taskDataMap = make(map[int][]domain.FormValue)
	}

	return lo.Map(ts, func(src domain.TaskRecord, _ int) domain.TaskRecord {
		userName := uMap[src.ApprovedBy]
		if userName == "" {
			userName = src.ApprovedBy
		}

		return domain.TaskRecord{
			TaskID:       src.TaskID,
			NodeID:       src.NodeID,
			PrevNodeID:   src.PrevNodeID,
			BatchCode:    src.BatchCode,
			NodeName:     src.NodeName,
			ApprovedBy:   userName,
			IsCosigned:   src.IsCosigned,
			Status:       src.Status,
			Comment:      src.Comment,
			IsFinished:   src.IsFinished,
			FinishedTime: src.FinishedTime,
			FormValues:   taskDataMap[src.TaskID.Int()],
		}
	})
}

// GetLatestReject 查询当前流程实例中处于活跃未闭环状态的最近一次驳回快照
// 若未发生驳回、或驳回后已被重新推进办结（已走回来），流程引擎会返回 nil
func (q *queryService) GetLatestReject(ctx context.Context, processInstID domain.ID) (*domain.LatestReject, error) {
	if processInstID.IsZero() {
		return nil, nil
	}

	engineType := q.resolveEngineType(ctx, processInstID)
	snapshot, err := q.engine.GetActiveReject(ctx, engineType, processInstID)
	if err != nil {
		q.logger.Warn("获取驳回快照失败，降级返回空驳回信息", elog.FieldErr(err), elog.String("process_instance_id", processInstID.String()))
		return nil, nil
	}
	if snapshot == nil {
		return nil, nil
	}

	// 仅定向解析驳回人真名，避免全员 RPC 浪费
	uMap := q.getUserMap(ctx, []string{snapshot.Operator})
	displayName := uMap[snapshot.Operator]
	if displayName == "" {
		displayName = snapshot.Operator
	}

	return &domain.LatestReject{
		RejectedBy:     snapshot.Operator,
		RejectedByName: displayName,
		NodeID:         snapshot.NodeID,
		NodeName:       snapshot.NodeName,
		Reason:         snapshot.Comment,
		RejectedAt:     snapshot.FinishTime,
	}, nil
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
