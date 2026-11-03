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
	querySvc      ticketSvc.QueryService
	userSvc       userv1.UserServiceClient
	engineSvc     engineSvc.Service
	workflowSvc   workflowSvc.Service
	withdrawalSvc withdrawalSvc.Service
	ratingSvc     ratingSvc.Service
	logger        *elog.Component
}

func NewHandler(svc ticketSvc.Service, querySvc ticketSvc.QueryService, engineSvc engineSvc.Service, userSvc userv1.UserServiceClient,
	workflowSvc workflowSvc.Service, withdrawalSvc withdrawalSvc.Service, ratingSvc ratingSvc.Service) *Handler {
	return &Handler{
		svc:           svc,
		querySvc:      querySvc,
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

func (h *Handler) Transfer(ctx *ginx.Context, req TransferReq) (ginx.Result, error) {
	if req.TaskId <= 0 {
		return ginx.Result{Code: 400, Msg: "任务 ID 非法"}, nil
	}
	if len(req.Usernames) == 0 {
		return ginx.Result{Code: 400, Msg: "转签目标用户不能为空"}, nil
	}

	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Transfer(ctx.Context, req.TaskId, user.Username, user.IsAdmin, req.Usernames)
	if err != nil {
		if errors.Is(err, domain.ErrTaskUnauthorized) {
			return ginx.Result{Code: 403, Msg: err.Error()}, nil
		}
		if errors.Is(err, domain.ErrTaskAlreadyFinished) {
			return ginx.Result{Code: 400, Msg: err.Error()}, nil
		}
		if errors.Is(err, domain.ErrInvalidParameter) {
			return ginx.Result{Code: 400, Msg: err.Error()}, nil
		}
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg: "转签成功",
	}, nil
}

func (h *Handler) Revoke(ctx *ginx.Context, req RevokeOrderReq) (ginx.Result, error) {
	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	// 权限判定：只有工单发起人或系统管理员允许撤回该工单
	ticket, err := h.svc.GetByProcessInstanceID(ctx.Context, req.InstanceId)
	if err != nil {
		if errors.Is(err, domain.ErrTicketNotAccessible) {
			return ticketNotAccessibleResult, nil
		}
		return systemErrorResult, fmt.Errorf("查询撤回工单信息失败: %w", err)
	}
	// 由工单领域模型统一防卫：仅允许发起人本人或管理员撤回，并断言当前处于可撤回状态
	if err = ticket.CanRevokeBy(user.Username, user.IsAdmin); err != nil {
		if errors.Is(err, domain.ErrTicketUnauthorized) {
			return ginx.Result{Code: 403, Msg: "无权撤回他人发起的工单"}, nil
		}
		return ginx.Result{Code: 400, Msg: err.Error()}, nil
	}

	err = h.withdrawalSvc.Revoke(ctx.Context, req.InstanceId, req.Force, user.Username, req.Reason)
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
	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}
	if err = h.svc.Pass(ctx.Context, req.TaskId, user.Username, user.IsAdmin, req.Comment, req.ExtraData); err != nil {
		if errors.Is(err, domain.ErrTaskUnauthorized) {
			return ginx.Result{Code: 403, Msg: err.Error()}, nil
		}
		if errors.Is(err, domain.ErrTaskAlreadyFinished) {
			return ginx.Result{Code: 400, Msg: err.Error()}, nil
		}
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg:  "同意审批",
		Data: nil,
	}, nil
}

func (h *Handler) Reject(ctx *ginx.Context, req RejectOrderReq) (ginx.Result, error) {
	user, err := h.getSessUser(ctx)
	if err != nil {
		return systemErrorResult, err
	}

	err = h.svc.Reject(ctx.Context, req.TaskId, user.Username, user.IsAdmin, req.Comment)
	if err != nil {
		if errors.Is(err, domain.ErrTaskUnauthorized) {
			return ginx.Result{Code: 403, Msg: err.Error()}, nil
		}
		if errors.Is(err, domain.ErrTaskAlreadyFinished) {
			return ginx.Result{Code: 400, Msg: err.Error()}, nil
		}
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

// TaskTimeline 以节点执行批次为单位返回流转历史；成员明细仅作为事件的展开内容
func (h *Handler) TaskTimeline(ctx *ginx.Context, req TaskTimelineReq) (ginx.Result, error) {
	groups, total, err := h.engineSvc.AccessibleTaskTimeline(ctx.Context, req.ProcessInstId, int(req.Offset), int(req.Limit))
	if err != nil {
		return systemErrorResult, err
	}

	events := h.assembleTimelineEvents(ctx.Context, groups)
	return ginx.Result{Data: RetrieveTaskTimeline{Events: events, Total: total}}, nil
}

func (h *Handler) assembleTimelineEvents(ctx context.Context, groups []engineSvc.TaskTimelineGroup) []TaskTimelineEvent {
	allMembers := lo.FlatMap(groups, func(g engineSvc.TaskTimelineGroup, _ int) []model.Task {
		return g.Members
	})
	allRecords := h.toTaskRecords(ctx, allMembers)
	recordsMap := make(map[int]TaskRecord, len(allMembers))
	for idx, record := range allRecords {
		recordsMap[allMembers[idx].TaskID] = record
	}

	return lo.Map(groups, func(group engineSvc.TaskTimelineGroup, _ int) TaskTimelineEvent {
		members := lo.Map(group.Members, func(m model.Task, _ int) TaskRecord {
			return recordsMap[m.TaskID]
		})
		return TaskTimelineEvent{
			ID:         group.NodeID + ":" + group.BatchCode,
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
		}
	})
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
	if view.TaskID > 0 {
		vo.TaskId = view.TaskID
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
