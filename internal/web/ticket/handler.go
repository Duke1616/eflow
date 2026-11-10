package ticket

import (
	"errors"
	"fmt"
	"time"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	"github.com/Duke1616/eflow/internal/pkg/ticketpbac"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
	ratingSvc "github.com/Duke1616/eflow/internal/service/rating"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
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
	querySvc      ticketSvc.QueryService
	userSvc       userv1.UserServiceClient
	withdrawalSvc withdrawalSvc.Service
	ratingSvc     ratingSvc.Service
	logger        *elog.Component
}

func NewHandler(svc ticketSvc.Service, querySvc ticketSvc.QueryService, userSvc userv1.UserServiceClient,
	withdrawalSvc withdrawalSvc.Service, ratingSvc ratingSvc.Service) *Handler {
	return &Handler{
		svc:           svc,
		querySvc:      querySvc,
		userSvc:       userSvc,
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
	if req.TicketId <= 0 {
		return ginx.Result{Code: 400, Msg: "工单 ID 非法"}, nil
	}
	if req.TaskId.IsZero() {
		return ginx.Result{Code: 400, Msg: "任务 ID 非法"}, nil
	}

	fields, err := h.svc.GetTaskFormConfig(ctx.Context, req.TicketId, req.TaskId.String(), req.WorkflowId)
	if err != nil {
		if res, handled := handleTaskOperationError(err); res.Code != 500 {
			return res, handled
		}
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
		if res, errHandled := handleTaskOperationError(err); res.Code != 500 {
			return res, errHandled
		}
		return systemErrorResult, fmt.Errorf("创建工单失败, %w", err)
	}

	return ginx.Result{
		Msg: "创建工单成功",
	}, nil
}

// RestartProcess 重新发送流程启动事件，并将结果反馈给工单发起人。
func (h *Handler) RestartProcess(ctx *ginx.Context, req RestartProcessReq) (ginx.Result, error) {
	if err := h.svc.RestartProcess(ctx.Context, req.TicketID); err != nil {
		if res, errHandled := handleTaskOperationError(err); res.Code != 500 {
			return res, errHandled
		}
		return systemErrorResult, fmt.Errorf("重新启动流程失败: %w", err)
	}
	return ginx.Result{Msg: "流程已重新启动"}, nil
}

func (h *Handler) TodoAll(ctx *ginx.Context, req Todo) (ginx.Result, error) {
	views, total, err := h.querySvc.ListAllTodo(ctx.Context, req.UserId, req.ProcessName, req.SortByAsc, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: h.toVoTicketViews(views),
		},
		Msg: "查看待办工单列表成功",
	}, nil
}

func (h *Handler) TodoByUser(ctx *ginx.Context, req Todo) (ginx.Result, error) {
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	views, total, err := h.querySvc.ListUserTodo(ctx.Context, username, req.ProcessName, req.SortByAsc, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: h.toVoTicketViews(views),
		},
		Msg: "查看待办工单列表成功",
	}, nil
}

// handleTaskOperationError 统一映射工单流转与任务操作中的领域业务错误
func handleTaskOperationError(err error) (ginx.Result, error) {
	if err == nil {
		return ginx.Result{}, nil
	}
	// 1. 若为 Kratos 风格标准结构化业务错误，自动提取 Code 与友好 Message
	if bizErr, ok := errors.AsType[*errs.Error](err); ok {
		return ginx.Result{Code: int(bizErr.Code), Msg: bizErr.Message}, nil
	}

	// 2. 兼容历史遗留哨兵错误
	switch {
	case errors.Is(err, domain.ErrTaskUnauthorized), errors.Is(err, domain.ErrTicketUnauthorized),
		errors.Is(err, ratingSvc.ErrTicketNotRateable):
		return ginx.Result{Code: 403, Msg: err.Error()}, nil
	case errors.Is(err, domain.ErrTaskAlreadyFinished),
		errors.Is(err, domain.ErrInvalidParameter),
		errors.Is(err, engine.ErrInvalidParameter),
		errors.Is(err, engine.ErrTaskNotFound),
		errors.Is(err, withdrawalSvc.ErrInvalidRevokeReason):
		return ginx.Result{Code: 400, Msg: err.Error()}, nil
	case errors.Is(err, repository.ErrTicketNotFound):
		return ginx.Result{Code: 404, Msg: "未找到关联工单信息"}, nil
	case errors.Is(err, withdrawalSvc.ErrAutomationRunning),
		errors.Is(err, ratingSvc.ErrAlreadyRated):
		return ginx.Result{Code: 409, Msg: err.Error()}, nil
	default:
		return systemErrorResult, err
	}
}

func (h *Handler) Transfer(ctx *ginx.Context, req TransferReq) (ginx.Result, error) {
	if req.TicketId <= 0 {
		return ginx.Result{Code: 400, Msg: "工单 ID 非法"}, nil
	}
	if req.TaskId.IsZero() {
		return ginx.Result{Code: 400, Msg: "任务 ID 非法"}, nil
	}
	if len(req.Usernames) == 0 {
		return ginx.Result{Code: 400, Msg: "转签目标用户不能为空"}, nil
	}

	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Transfer(ctx.Context, req.TicketId, req.TaskId.String(), user.Username, user.IsAdmin, req.Usernames)
	if err != nil {
		return handleTaskOperationError(err)
	}

	return ginx.Result{
		Msg: "转签成功",
	}, nil
}

func (h *Handler) Revoke(ctx *ginx.Context, req RevokeOrderReq) (ginx.Result, error) {
	if req.InstanceId.IsZero() {
		return ginx.Result{Code: 400, Msg: "撤销工单失败: 流程实例标识不能为空 (instance_id)"}, nil
	}

	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	// 权限判定：只有工单发起人或系统管理员允许撤回该工单
	ticket, err := h.svc.GetByProcessInstance(ctx.Context, req.InstanceId)
	if err != nil {
		if errors.Is(err, domain.ErrTicketNotAccessible) {
			return ticketNotAccessibleResult, nil
		}
		if res, handled := handleTaskOperationError(err); res.Code != 500 {
			return res, handled
		}
		h.logger.Error("撤回工单前检索工单失败", elog.String("instance_id", req.InstanceId.String()), elog.FieldErr(err))
		return systemErrorResult, fmt.Errorf("查询待撤回工单信息失败 [instance_id=%s]: %w", req.InstanceId, err)
	}
	// 由工单领域模型统一防卫：仅允许发起人本人或管理员撤回，并断言当前处于可撤回状态
	if err = ticket.CanRevokeBy(user.Username, user.IsAdmin); err != nil {
		if errors.Is(err, domain.ErrTicketUnauthorized) {
			return ginx.Result{Code: 403, Msg: "无权撤回他人发起的工单"}, nil
		}
		return ginx.Result{Code: 400, Msg: err.Error()}, nil
	}

	err = h.withdrawalSvc.Revoke(ctx.Context, ticket.Process.ResolveEngineType(), req.InstanceId, req.Force, user.Username, req.Reason)
	if err != nil {
		if res, errHandled := handleTaskOperationError(err); res.Code != 500 {
			return res, errHandled
		}
		h.logger.Error("执行工单撤回失败", elog.String("instance_id", req.InstanceId.String()), elog.FieldErr(err))
		return systemErrorResult, fmt.Errorf("执行工单撤回失败 [instance_id=%s]: %w", req.InstanceId, err)
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
		return handleTaskOperationError(err)
	}
	return ginx.Result{Msg: "评价提交成功", Data: toRatingVO(rating)}, nil
}

func (h *Handler) Pass(ctx *ginx.Context, req PassOrderReq) (ginx.Result, error) {
	if req.TicketId <= 0 {
		return ginx.Result{Code: 400, Msg: "工单 ID 非法"}, nil
	}
	if req.TaskId.IsZero() {
		return ginx.Result{Code: 400, Msg: "任务 ID 非法"}, nil
	}

	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}
	if err = h.svc.Pass(ctx.Context, req.TicketId, req.TaskId.String(), user.Username, user.IsAdmin, req.Comment, req.ExtraData); err != nil {
		return handleTaskOperationError(err)
	}

	return ginx.Result{
		Msg:  "同意审批",
		Data: nil,
	}, nil
}

func (h *Handler) Reject(ctx *ginx.Context, req RejectOrderReq) (ginx.Result, error) {
	if req.TicketId <= 0 {
		return ginx.Result{Code: 400, Msg: "工单 ID 非法"}, nil
	}
	if req.TaskId.IsZero() {
		return ginx.Result{Code: 400, Msg: "任务 ID 非法"}, nil
	}

	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Reject(ctx.Context, req.TicketId, req.TaskId.String(), user.Username, user.IsAdmin, req.Comment)
	if err != nil {
		return handleTaskOperationError(err)
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

	views, total, err := h.querySvc.ListUserInitiated(ctx.Context, username, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: h.toVoTicketViews(views),
		},
		Msg: "查看我的工单列表成功",
	}, nil
}

func (h *Handler) Detail(ctx *ginx.Context, req DetailProcessInstIdReq) (ginx.Result, error) {
	if req.ProcessInstanceId.IsZero() {
		return ginx.Result{Code: 400, Msg: "流程实例标识不能为空 (process_instance_id)"}, nil
	}

	ticket, err := h.svc.GetByProcessInstance(ctx.Context, req.ProcessInstanceId)
	if err != nil {
		if errors.Is(err, domain.ErrTicketNotAccessible) {
			return ticketNotAccessibleResult, nil
		}
		if res, handled := handleTaskOperationError(err); res.Code != 500 {
			return res, handled
		}
		h.logger.Error("获取工单详情失败: 未找到流程实例关联工单", elog.String("process_instance_id", req.ProcessInstanceId.String()), elog.FieldErr(err))
		return systemErrorResult, fmt.Errorf("根据流程实例标识 [%s] 查询工单详情失败: %w", req.ProcessInstanceId, err)
	}

	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}
	rating, exists, err := h.ratingSvc.FindByTicketID(ctx.Context, ticket.Id)
	if err != nil {
		h.logger.Error("查询工单评价信息失败", elog.Int64("ticket_id", ticket.Id), elog.FieldErr(err))
		return systemErrorResult, fmt.Errorf("查询工单 [%d] 评价信息失败: %w", ticket.Id, err)
	}
	result := h.toVoTicket(ticket)
	result.CanRate = canRateTicket(ticket, username, exists)
	if exists {
		result.Rating = toRatingVO(rating)
	}
	return ginx.Result{Data: result}, nil
}

func (h *Handler) TaskRecord(ctx *ginx.Context, req RecordTaskReq) (ginx.Result, error) {
	ts, total, err := h.querySvc.TaskRecord(ctx.Context, req.ProcessInstId, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: RetrieveTaskRecords{
			TaskRecords: toVoTaskRecords(ts),
			Total:       total,
		},
	}, nil
}

// TaskTimeline 以节点执行批次为单位返回流转历史；成员明细仅作为事件的展开内容
func (h *Handler) TaskTimeline(ctx *ginx.Context, req TaskTimelineReq) (ginx.Result, error) {
	events, total, err := h.querySvc.TaskTimeline(ctx.Context, req.ProcessInstId, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{Data: RetrieveTaskTimeline{Events: toVoTimelineEvents(events), Total: total}}, nil
}

func toVoTaskRecords(records []domain.TaskRecord) []TaskRecord {
	return lo.Map(records, func(src domain.TaskRecord, _ int) TaskRecord {
		return TaskRecord{
			NodeName:     src.NodeName,
			ApprovedBy:   src.ApprovedBy,
			IsCosigned:   src.IsCosigned,
			Status:       src.Status,
			Comment:      src.Comment,
			IsFinished:   src.IsFinished,
			FinishedTime: src.FinishedTime,
			FormValues: lo.Map(src.FormValues, func(val domain.FormValue, _ int) FormValue {
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

func toVoTimelineEvents(events []domain.TaskTimelineEvent) []TaskTimelineEvent {
	return lo.Map(events, func(group domain.TaskTimelineEvent, _ int) TaskTimelineEvent {
		return TaskTimelineEvent{
			ID:         group.ID,
			NodeID:     group.NodeID,
			NodeName:   group.NodeName,
			BatchCode:  group.BatchCode,
			IsCosigned: group.IsCosigned,
			OccurredAt: group.OccurredAt,
			Actors:     group.Actors,
			Summary: TaskTimelineSummary{
				Total:          group.Summary.Total,
				Passed:         group.Summary.Passed,
				Rejected:       group.Summary.Rejected,
				SystemPassed:   group.Summary.SystemPassed,
				SystemRejected: group.Summary.SystemRejected,
				Skipped:        group.Summary.Skipped,
				Linked:         group.Summary.Linked,
				Pending:        group.Summary.Pending,
			},
			Members: toVoTaskRecords(group.Members),
		}
	})
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
	var ctime, wtime string
	if req.Ctime > 0 {
		ctime = time.Unix(req.Ctime/1000, 0).Format("2006-01-02 15:04:05")
	}
	if req.Wtime > 0 {
		wtime = time.Unix(req.Wtime/1000, 0).Format("2006-01-02 15:04:05")
	}

	var procInstID InstanceID
	if !req.Process.InstanceID.IsZero() {
		procInstID = InstanceID(req.Process.InstanceID.String())
	}

	return Ticket{
		Id:                   req.Id,
		EngineType:           req.Process.EngineType.String(),
		TemplateId:           req.TemplateId,
		Starter:              req.CreateBy,
		ProcessInstanceId:    procInstID,
		Provide:              req.Provide.ToUint8(),
		Status:               req.Status.ToUint8(),
		WorkflowId:           req.WorkflowId,
		Ctime:                ctime,
		Wtime:                wtime,
		Data:                 req.Data,
		RevokeReason:         req.RevokeReason,
		ProcessStartError:    req.ProcessStartError,
		ProcessStartAttempts: req.ProcessStartAttempts,
	}
}

func (h *Handler) History(ctx *ginx.Context, req HistoryReq) (ginx.Result, error) {
	username, err := h.getSessUsername(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	views, total, err := h.querySvc.ListHistory(ctx.Context, username, req.UserId, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: h.toVoTicketViews(views),
		},
		Msg: "查看历史工单列表成功",
	}, nil
}

func (h *Handler) toVoTicketViews(views []domain.TicketTaskView) []Ticket {
	return lo.Map(views, func(v domain.TicketTaskView, _ int) Ticket {
		return h.toVoTicketView(v)
	})
}

func (h *Handler) toVoTicketView(view domain.TicketTaskView) Ticket {
	vo := h.toVoTicket(view.Ticket)
	if !view.TaskID.IsZero() {
		vo.TaskId = TaskID(view.TaskID.String())
	}
	if view.CurrentStep != "" {
		vo.CurrentStep = view.CurrentStep
	}
	if view.ApprovedBy != "" {
		vo.ApprovedBy = view.ApprovedBy
	}
	if view.ProcInstCreateTime != nil {
		if t, ok := view.ProcInstCreateTime.(*time.Time); ok && t != nil {
			vo.ProcInstCreateTime = t.Format("2006-01-02 15:04:05")
		} else {
			vo.ProcInstCreateTime = view.ProcInstCreateTime
		}
	}
	// 兜底保障：若底层引擎实例未返回创建时间（如流程启动中、启动失败），统一回退为工单提单创建时间，确保前端列绝不为空
	if vo.ProcInstCreateTime == nil || vo.ProcInstCreateTime == "" {
		vo.ProcInstCreateTime = vo.Ctime
	}
	vo.CanRate = view.CanRate
	if view.Rating != nil {
		vo.Rating = toRatingVO(*view.Rating)
	}
	return vo
}

func toRatingVO(rating domain.TicketRating) *Rating {
	return &Rating{
		Score: rating.Score, Comment: rating.Comment,
		Rater: rating.RaterUsername, RatedAt: rating.CTime,
	}
}

func canRateTicket(ticket domain.Ticket, actor string, alreadyRated bool) bool {
	return ticket.CanBeRatedBy(actor, alreadyRated)
}

func (h *Handler) getSessUser(ctx *ginx.Context) (*userv1.User, error) {
	uid := ctxutil.GetUserID(ctx).Int64()
	if uid <= 0 {
		return nil, fmt.Errorf("上下文缺少有效的用户身份 ID: %d", uid)
	}

	resp, err := h.userSvc.QueryById(ctx.Context, &userv1.QueryByIdReq{Id: uid})
	if err != nil {
		return nil, fmt.Errorf("查询当前登录用户信息失败: %w", err)
	}
	if resp == nil || resp.User == nil {
		return nil, fmt.Errorf("未检索到 ID 为 %d 的有效用户信息", uid)
	}

	return resp.User, nil
}

func (h *Handler) getSessUsername(ctx *ginx.Context) (string, error) {
	user, err := h.getSessUser(ctx)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}
