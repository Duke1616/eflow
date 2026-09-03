package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/ticketpbac"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	ratingSvc "github.com/Duke1616/eflow/internal/service/rating"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/pkg/contract/permission"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
)

var systemErrorResult = ginx.Result{Code: 500, Msg: "系统内部错误"}
var ticketNotAccessibleResult = ginx.Result{Code: 403, Msg: "工单不存在或无权访问"}

type Handler struct {
	capability.IRegistry
	svc           ticketSvc.Service
	userSvc       userv1.UserServiceClient
	engineSvc     engineSvc.Service
	workflowSvc   workflowSvc.Service
	withdrawalSvc withdrawalSvc.Service
	ratingSvc     ratingSvc.Service
	logger        *elog.Component
}

func NewHandler(svc ticketSvc.Service, engineSvc engineSvc.Service, userSvc userv1.UserServiceClient,
	workflowSvc workflowSvc.Service, withdrawalSvc withdrawalSvc.Service, ratingSvc ratingSvc.Service) *Handler {
	return &Handler{
		svc:           svc,
		userSvc:       userSvc,
		engineSvc:     engineSvc,
		workflowSvc:   workflowSvc,
		withdrawalSvc: withdrawalSvc,
		ratingSvc:     ratingSvc,
		logger:        elog.DefaultLogger,
		IRegistry:     capability.NewRegistry("ticket", "manager", "工单中心"),
	}
}

func (h *Handler) PublicRoutes(server *gin.Engine) {
	// 目前无公共 API
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/ticket")

	g.POST("/submit", h.Define("提交工单", "submit").
		Group("工单中心/工单操作").
		Needs(permission.Template.Get, permission.Template.ToggleFavorite, permission.Template.ViewFavorite,
			permission.Template.View, permission.Template.ViewGroupSummary, "cmdb:tools:upload").
		Bind(ginx.B[CreateTicketReq](h.CreateTicket)),
	)
	g.POST("/process/restart", h.Define("重新启动流程", "process_restart").
		Group("工单中心/工单操作").
		Bind(ginx.B[RestartProcessReq](h.RestartProcess)),
	)
	g.POST("/detail/process_inst_id", h.Define("工单详情", "get").
		Group("工单中心/工单详情").
		Needs(permission.Manager.Graph, permission.Manager.Timeline, "cmdb:tools:download").
		AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
		Bind(ginx.B[DetailProcessInstIdReq](h.Detail)),
	)
	g.POST("/task/timeline", h.Define("流转时间线", "timeline").
		Group("工单中心/工单详情").
		AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
		Bind(ginx.B[TaskTimelineReq](h.TaskTimeline)),
	)
	g.POST("/todo", h.Define("所有待办工单", "todo").
		Group("工单中心/工单列表").
		Needs(permission.Template.ViewByIds, permission.Manager.Get).
		AccessScope(ticketpbac.TodoProfile, ticketpbac.TodoPresets...).
		Bind(ginx.B[Todo](h.TodoAll)),
	)
	g.POST("/todo/user", h.Define("我的待办工单", "my_todo").
		Group("工单中心/工单列表").
		Needs(permission.Template.ViewByIds, permission.Manager.Get).
		Bind(ginx.B[Todo](h.TodoByUser)),
	)
	g.POST("/history", h.Define("历史工单", "history").
		Group("工单中心/工单列表").
		Needs(permission.Template.ViewByIds, permission.Manager.Get, permission.Manager.Rate).
		AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
		Bind(ginx.B[HistoryReq](h.History)),
	)
	g.POST("/start/user", h.Define("我发起的工单", "my_start").
		Group("工单中心/工单列表").
		Needs(permission.Template.ViewByIds, permission.Manager.Get).
		Bind(ginx.B[StartUserReq](h.StartUser)),
	)
	g.POST("/pass", h.Define("同意审批", "pass").
		Group("工单中心/工单操作").
		Needs(permission.Manager.FormConfig).
		Bind(ginx.B[PassOrderReq](h.Pass)),
	)
	g.POST("/reject", h.Define("驳回审批", "reject").
		Group("工单中心/工单操作").
		Needs(permission.Manager.FormConfig).
		Bind(ginx.B[RejectOrderReq](h.Reject)),
	)
	g.POST("/transfer", h.Define("转交审批人", "transfer").
		Group("工单中心/工单操作").
		Needs("iam:user:view").
		Bind(ginx.B[TransferReq](h.Transfer)),
	)
	g.POST("/revoke", h.Define("撤销工单", "revoke").
		Group("工单中心/工单操作").
		Bind(ginx.B[RevokeOrderReq](h.Revoke)),
	)
	g.POST("/rating/submit", h.Define("评价工单", "rate").
		Group("工单中心/工单操作").
		NoSync().
		Bind(ginx.B[SubmitRatingReq](h.SubmitRating)),
	)
	g.POST("/task/form_config", h.Define("任务节点表单配置", "form_config").
		Group("工单中心/工单详情").
		Needs(permission.Template.Get, permission.Manager.Get).
		Bind(ginx.B[TaskFormConfigReq](h.GetTaskFormConfig)),
	)
}

func (h *Handler) GetTaskFormConfig(ctx *ginx.Context, req TaskFormConfigReq) (ginx.Result, error) {
	fields, err := h.svc.GetTaskFormConfig(ctx.Context, req.TaskId, req.WorkflowId)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: fields,
		Msg:  "获取任务表单配置成功",
	}, nil
}

func (h *Handler) CreateTicket(ctx *ginx.Context, req CreateTicketReq) (ginx.Result, error) {
	if req.CreateBy == "" {
		username, err := h.getSessUsername(ctx)
		if err != nil {
			return systemErrorResult, err
		}
		req.CreateBy = username
	}

	err := h.svc.CreateTicket(ctx.Context, h.toDomain(req))
	if err != nil {
		return systemErrorResult, fmt.Errorf("创建工单失败, %w", err)
	}

	return ginx.Result{
		Msg: "创建工单成功",
	}, nil
}

// RestartProcess 重新发送流程启动事件，并将结果反馈给工单发起人。
func (h *Handler) RestartProcess(ctx *ginx.Context, req RestartProcessReq) (ginx.Result, error) {
	if err := h.svc.RestartProcess(ctx.Context, req.TicketID); err != nil {
		return systemErrorResult, fmt.Errorf("重新启动流程失败: %w", err)
	}
	return ginx.Result{Msg: "流程已重新启动"}, nil
}

func (h *Handler) TodoAll(ctx *ginx.Context, req Todo) (ginx.Result, error) {
	instances, total, err := h.engineSvc.ListAllTodoTasks(ctx.Context, req.UserId, req.ProcessName, req.SortByAsc, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}

	tickets, err := h.toVoEngineTicket(ctx.Context, instances)
	if err != nil {
		return systemErrorResult, err
	}
	failedTickets, failedTotal, err := h.svc.ListProcessStartFailed(ctx.Context, "", req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	for _, failed := range failedTickets {
		tickets = append([]Ticket{h.toVoTicket(failed)}, tickets...)
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total + failedTotal,
			Tasks: tickets,
		},
		Msg: "查看待办工单列表成功",
	}, nil
}

func (h *Handler) TodoByUser(ctx *ginx.Context, req Todo) (ginx.Result, error) {
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	instances, total, err := h.engineSvc.ListTodoTasks(ctx.Context, username, req.ProcessName, req.SortByAsc, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}

	tickets, err := h.toVoEngineTicket(ctx.Context, instances)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: tickets,
		},
		Msg: "查看待办工单列表成功",
	}, nil
}

func (h *Handler) Transfer(ctx *ginx.Context, req TransferReq) (ginx.Result, error) {
	_, err := h.engineSvc.Transfer(ctx.Context, req.TaskId, req.Usernames)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg: "转签成功",
	}, nil
}

func (h *Handler) Revoke(ctx *ginx.Context, req RevokeOrderReq) (ginx.Result, error) {
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	err = h.withdrawalSvc.Revoke(ctx.Context, req.InstanceId, req.Force, username, req.Reason)
	if err != nil {
		if errors.Is(err, withdrawalSvc.ErrInvalidRevokeReason) {
			return ginx.Result{Code: 400, Msg: err.Error()}, nil
		}
		if errors.Is(err, withdrawalSvc.ErrAutomationRunning) {
			return ginx.Result{Code: 409, Msg: err.Error()}, nil
		}
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg:  "撤回请求已提交",
		Data: true,
	}, nil
}

func (h *Handler) SubmitRating(ctx *ginx.Context, req SubmitRatingReq) (ginx.Result, error) {
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}
	rating, err := h.ratingSvc.Submit(ctx.Context, req.TicketID, username, req.Score, req.Comment)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidParameter):
			return ginx.Result{Code: 400, Msg: err.Error()}, nil
		case errors.Is(err, ratingSvc.ErrTicketNotRateable):
			return ginx.Result{Code: 403, Msg: ratingSvc.ErrTicketNotRateable.Error()}, nil
		case errors.Is(err, ratingSvc.ErrAlreadyRated):
			return ginx.Result{Code: 409, Msg: ratingSvc.ErrAlreadyRated.Error()}, nil
		default:
			return systemErrorResult, err
		}
	}
	return ginx.Result{Msg: "评价提交成功", Data: toRatingVO(rating)}, nil
}

func (h *Handler) Pass(ctx *ginx.Context, req PassOrderReq) (ginx.Result, error) {
	err := h.verifyUser(ctx, req.TaskId)
	if err != nil {
		return systemErrorResult, err
	}
	if err = h.svc.Pass(ctx.Context, req.TaskId, req.Comment, req.ExtraData); err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg:  "同意审批",
		Data: nil,
	}, nil
}

func (h *Handler) Reject(ctx *ginx.Context, req RejectOrderReq) (ginx.Result, error) {
	err := h.verifyUser(ctx, req.TaskId)
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Reject(ctx.Context, req.TaskId, req.Comment)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg:  "驳回审批",
		Data: nil,
	}, nil
}

func (h *Handler) StartUser(ctx *ginx.Context, req StartUserReq) (ginx.Result, error) {
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	tickets, total, err := h.svc.ListByUser(ctx.Context, username, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}

	procInstIds := lo.FilterMap(tickets, func(src domain.Ticket, _ int) (int, bool) {
		return src.Process.InstanceId, src.Process.InstanceId > 0
	})

	processTasks, err := h.engineSvc.ListPendingStepsOfMyTask(ctx.Context, procInstIds, username)
	if err != nil {
		return systemErrorResult, err
	}

	tasks, err := h.toVoEngineTicket(ctx.Context, processTasks)
	if err != nil {
		return systemErrorResult, err
	}
	for _, failed := range tickets {
		if failed.Status == domain.START_FAILED || failed.Status == domain.START {
			tasks = append(tasks, h.toVoTicket(failed))
		}
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: tasks,
		},
		Msg: "查看我的工单列表成功",
	}, nil
}

func (h *Handler) Detail(ctx *ginx.Context, req DetailProcessInstIdReq) (ginx.Result, error) {
	ticket, err := h.svc.GetByProcessInstanceID(ctx.Context, req.ProcessInstanceId)
	if err != nil {
		if errors.Is(err, domain.ErrTicketNotAccessible) {
			return ticketNotAccessibleResult, nil
		}
		return systemErrorResult, err
	}

	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}
	rating, exists, err := h.ratingSvc.FindByTicketID(ctx.Context, ticket.Id)
	if err != nil {
		return systemErrorResult, err
	}
	result := h.toVoTicket(ticket)
	result.CanRate = canRateTicket(ticket, username, exists)
	if exists {
		result.Rating = toRatingVO(rating)
	}
	return ginx.Result{Data: result}, nil
}

func (h *Handler) TaskRecord(ctx *ginx.Context, req RecordTaskReq) (ginx.Result, error) {
	ts, total, err := h.engineSvc.AccessibleTaskRecord(ctx.Context, req.ProcessInstId, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}
	records := h.toTaskRecords(ctx.Context, ts)

	return ginx.Result{
		Data: RetrieveTaskRecords{
			TaskRecords: records,
			Total:       total,
		},
	}, nil
}

// TaskTimeline 以节点执行批次为单位返回流转历史；成员明细仅作为事件的展开内容。
func (h *Handler) TaskTimeline(ctx *ginx.Context, req TaskTimelineReq) (ginx.Result, error) {
	groups, total, err := h.engineSvc.AccessibleTaskTimeline(ctx.Context, req.ProcessInstId, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}

	allMembers := make([]model.Task, 0)
	for _, group := range groups {
		allMembers = append(allMembers, group.Members...)
	}
	recordsByTaskID := make(map[int]TaskRecord, len(allMembers))
	allRecords := h.toTaskRecords(ctx.Context, allMembers)
	for idx, record := range allRecords {
		recordsByTaskID[allMembers[idx].TaskID] = record
	}

	events := make([]TaskTimelineEvent, 0, len(groups))
	for _, group := range groups {
		members := make([]TaskRecord, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, recordsByTaskID[member.TaskID])
		}
		id := group.NodeID + ":" + group.BatchCode
		events = append(events, TaskTimelineEvent{
			ID:         id,
			NodeID:     group.NodeID,
			NodeName:   group.NodeName,
			BatchCode:  group.BatchCode,
			IsCosigned: group.IsCosigned,
			OccurredAt: group.OccurredAt.String(),
			Actors:     timelineActors(members),
			Summary: TaskTimelineSummary{
				Total:          group.TaskCount,
				Passed:         group.PassedCount,
				Rejected:       group.RejectedCount,
				SystemPassed:   group.SystemPassedCount,
				SystemRejected: group.SystemRejectedCount,
				Skipped:        group.SkippedCount,
				Linked:         group.LinkedCount,
				Pending:        group.PendingCount,
			},
			Members: members,
		})
	}

	return ginx.Result{Data: RetrieveTaskTimeline{Events: events, Total: total}}, nil
}

func (h *Handler) toTaskRecords(ctx context.Context, ts []model.Task) []TaskRecord {
	uniqueUserIDs := lo.Uniq(lo.Map(ts, func(src model.Task, _ int) string {
		return src.UserID
	}))

	uMap, err := h.getUserMap(ctx, uniqueUserIDs)
	if err != nil {
		uMap = make(map[string]string)
	}

	taskIds := lo.Map(ts, func(src model.Task, _ int) int {
		return src.TaskID
	})
	taskDataMap, err := h.svc.ListTaskFormsByTaskIDs(ctx, taskIds)
	if err != nil {
		taskDataMap = make(map[int][]domain.FormValue)
	}

	return lo.Map(ts, func(src model.Task, _ int) TaskRecord {
		userName := uMap[src.UserID]
		if userName == "" {
			userName = src.UserID
		}

		return TaskRecord{
			Nodename:     src.NodeName,
			ApprovedBy:   userName,
			IsCosigned:   src.IsCosigned,
			Status:       src.Status,
			Comment:      src.Comment,
			IsFinished:   src.IsFinished,
			FinishedTime: src.FinishedTime,
			FormValues: lo.Map(taskDataMap[src.TaskID], func(val domain.FormValue, _ int) FormValue {
				return FormValue{
					Name:  val.Name,
					Key:   val.Key,
					Type:  val.Type,
					Value: val.Value,
				}
			}),
		}
	})
}

func timelineActors(records []TaskRecord) []string {
	filtered := lo.Filter(records, func(r TaskRecord, _ int) bool {
		return r.Status == 1 || r.Status == 2
	})
	return lo.Uniq(lo.Map(filtered, func(r TaskRecord, _ int) string {
		return r.ApprovedBy
	}))
}

func (h *Handler) toDomain(req CreateTicketReq) domain.Ticket {
	return domain.Ticket{
		CreateBy:   req.CreateBy,
		TemplateId: req.TemplateId,
		WorkflowId: req.WorkflowId,
		Data:       req.Data,
		Status:     domain.START,
		Provide:    domain.SYSTEM,
	}
}

func (h *Handler) toVoTicket(req domain.Ticket) Ticket {
	return Ticket{
		Id:                   req.Id,
		TemplateId:           req.TemplateId,
		Starter:              req.CreateBy,
		ProcessInstanceId:    req.Process.InstanceId,
		Provide:              req.Provide.ToUint8(),
		Status:               req.Status.ToUint8(),
		WorkflowId:           req.WorkflowId,
		Ctime:                time.Unix(req.Ctime/1000, 0).Format("2006-01-02 15:04:05"),
		Wtime:                time.Unix(req.Wtime/1000, 0).Format("2006-01-02 15:04:05"),
		Data:                 req.Data,
		RevokeReason:         req.RevokeReason,
		ProcessStartError:    req.ProcessStartError,
		ProcessStartAttempts: req.ProcessStartAttempts,
	}
}

func (h *Handler) History(ctx *ginx.Context, req HistoryReq) (ginx.Result, error) {
	os, total, err := h.svc.ListHistory(ctx.Context, req.UserId, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}
	ticketIDs := lo.Map(os, func(ticket domain.Ticket, _ int) int64 { return ticket.Id })
	ratings, err := h.ratingSvc.ListByTicketIDs(ctx.Context, ticketIDs)
	if err != nil {
		return systemErrorResult, err
	}

	uns := lo.Uniq(lo.Map(os, func(ticket domain.Ticket, _ int) string {
		return ticket.CreateBy
	}))

	uMap, err := h.getUserMap(ctx.Context, uns)
	if err != nil {
		uMap = make(map[string]string)
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: lo.Map(os, func(src domain.Ticket, _ int) Ticket {
				starter, ok := uMap[src.CreateBy]
				if !ok {
					starter = src.CreateBy
				}

				result := Ticket{
					Id:                src.Id,
					TemplateId:        src.TemplateId,
					Starter:           starter,
					Status:            src.Status.ToUint8(),
					Provide:           src.Provide.ToUint8(),
					ProcessInstanceId: src.Process.InstanceId,
					WorkflowId:        src.WorkflowId,
					Ctime:             time.Unix(src.Ctime/1000, 0).Format("2006-01-02 15:04:05"),
					Wtime:             time.Unix(src.Wtime/1000, 0).Format("2006-01-02 15:04:05"),
					Data:              src.Data,
					RevokeReason:      src.RevokeReason,
				}
				rating, rated := ratings[src.Id]
				result.CanRate = canRateTicket(src, username, rated)
				if rated {
					result.Rating = toRatingVO(rating)
				}
				return result
			}),
		},
	}, nil
}

func canRateTicket(ticket domain.Ticket, actor string, alreadyRated bool) bool {
	return ticket.Status == domain.END && ticket.CreateBy == actor && !alreadyRated
}

func toRatingVO(rating domain.TicketRating) *Rating {
	return &Rating{
		Score: rating.Score, Comment: rating.Comment,
		Rater: rating.RaterUsername, RatedAt: rating.CTime,
	}
}

func (h *Handler) toVoEngineTicket(ctx context.Context, instances []domain.Instance) ([]Ticket, error) {
	if len(instances) == 0 {
		return nil, nil
	}

	procInstIds := lo.Uniq(lo.Map(instances, func(inst domain.Instance, _ int) int {
		return inst.ProcInstID
	}))

	us, err := h.getUsers(ctx, instances)
	if err != nil {
		us = make(map[string]string)
	}

	os, err := h.svc.ListByProcessInstanceIDs(ctx, procInstIds)
	if err != nil {
		return nil, err
	}
	m := lo.KeyBy(os, func(element domain.Ticket) int {
		return element.Process.InstanceId
	})

	return lo.Map(instances, func(src domain.Instance, _ int) Ticket {
		val := m[src.ProcInstID]
		starter, ok := us[src.Starter]
		if !ok {
			starter = src.Starter
		}
		approved, ok := us[src.ApprovedBy]
		if !ok {
			approved = src.ApprovedBy
		}

		ctime := ""
		if val.Ctime > 0 {
			ctime = time.Unix(val.Ctime/1000, 0).Format("2006-01-02 15:04:05")
		}

		var createTimeStr string
		if src.CreateTime != nil {
			createTimeStr = src.CreateTime.Format("2006-01-02 15:04:05")
		}

		return Ticket{
			Id:                   val.Id,
			TaskId:               src.TaskID,
			ProcessInstanceId:    src.ProcInstID,
			Starter:              starter,
			CurrentStep:          src.CurrentNodeName,
			ApprovedBy:           approved,
			ProcInstCreateTime:   createTimeStr,
			Provide:              val.Provide.ToUint8(),
			Status:               val.Status.ToUint8(),
			TemplateId:           val.TemplateId,
			WorkflowId:           val.WorkflowId,
			Ctime:                ctime,
			ProcessStartError:    val.ProcessStartError,
			ProcessStartAttempts: val.ProcessStartAttempts,
		}
	}), nil
}

func (h *Handler) getUsers(ctx context.Context, instances []domain.Instance) (map[string]string, error) {
	users := lo.FlatMap(instances, func(inst domain.Instance, _ int) []string {
		return []string{inst.ApprovedBy, inst.Starter}
	})
	nonEmptyUsers := lo.Filter(users, func(u string, _ int) bool {
		return u != ""
	})
	return h.getUserMap(ctx, lo.Uniq(nonEmptyUsers))
}

func (h *Handler) getSessUsername(ctx *ginx.Context) (string, error) {
	uid := ctxutil.GetUserID(ctx).Int64()
	if uid == 0 {
		return "", fmt.Errorf("获取 UserID 失败: %d", uid)
	}
	resp, err := h.userSvc.QueryByIds(ctx.Context, &userv1.QueryByIdsReq{
		Ids: []int64{uid},
	})
	if err != nil || len(resp.Users) == 0 {
		return "", fmt.Errorf("查询 gRPC 用户信息失败: %w", err)
	}

	return resp.Users[0].Username, nil
}

func (h *Handler) verifyUser(ctx *ginx.Context, taskId int) error {
	uid := ctxutil.GetUserID(ctx).Int64()
	if uid == 0 {
		return fmt.Errorf("获取 UserID 失败: %d", uid)
	}

	// 1. 获取操作用户信息
	resp, err := h.userSvc.QueryById(ctx.Context, &userv1.QueryByIdReq{
		Id: uid,
	})
	if err != nil {
		return fmt.Errorf("获取用户信息失败: %w", err)
	}

	// 2. 检索流程任务详情
	tInfo, err := h.engineSvc.TaskInfo(ctx.Context, taskId)
	if err != nil {
		return err
	}

	// 3. 权限判定：如果不是管理员用户，则必须校验当前任务指派处理人与当前操作用户一致
	if !resp.User.IsAdmin && tInfo.UserID != resp.User.Username {
		return fmt.Errorf("无法操作，当前审批任务指派处理人与您账号不一致")
	}

	// 记录 admin 操作别人任务的审计日志
	if resp.User.IsAdmin && tInfo.UserID != resp.User.Username {
		h.logger.Info("管理员代办他人任务",
			elog.String("admin", resp.User.Username),
			elog.Int("taskId", taskId),
			elog.String("originalAssignee", tInfo.UserID),
		)
	}

	return nil
}

func (h *Handler) getUserMap(ctx context.Context, uns []string) (map[string]string, error) {
	if len(uns) == 0 {
		return make(map[string]string), nil
	}
	resp, err := h.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: uns,
	})
	if err != nil {
		return nil, err
	}

	return lo.Associate(resp.Users, func(element *userv1.User) (string, string) {
		return element.Username, element.DisplayName
	}), nil
}

// Upstream 查询指定任务节点的上游处理节点及历史流转进度
func (h *Handler) Upstream(ctx *ginx.Context) (ginx.Result, error) {
	taskID, err := ctx.Param("task_id").AsInt()
	if err != nil {
		return systemErrorResult, err
	}

	upstream, err := h.engineSvc.Upstream(ctx.Context, taskID)
	if err != nil {
		return ginx.Result{}, err
	}

	return ginx.Result{Data: upstream}, nil
}
