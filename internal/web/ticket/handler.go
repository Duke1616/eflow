package ticket

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/ticketpbac"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	ratingSvc "github.com/Duke1616/eflow/internal/service/rating"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
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
		IRegistry:     capability.NewRegistry("ticket", "manager", "工单中心"),
	}
}

func (h *Handler) PublicRoutes(server *gin.Engine) {
	// 目前无公共 API
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/ticket")

	op := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("工单中心/工单操作")
	}
	detail := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("工单中心/工单详情")
	}
	list := func(name, code string) *capability.Builder {
		return h.Capability(name, code).Group("工单中心/工单列表")
	}

	g.POST("/submit", op("提交工单", "submit").
		Needs("ticket:template:get", "ticket:template:toggle_favorite", "ticket:template:view_favorite",
			"ticket:template:view", "ticket:template:view_group_summary", "cmdb:tools:upload").
		Handle(ginx.B[CreateTicketReq](h.CreateTicket)),
	)
	g.POST("/detail/process_inst_id", detail("工单详情", "get").
		Needs("cmdb:tools:download").
		AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
		Handle(ginx.B[DetailProcessInstIdReq](h.Detail)),
	)
	// 已由按节点批次聚合的 /task/timeline 替代；保留原始任务查询实现，
	// 供流程内部审计逻辑继续复用，但不再暴露逐任务的 HTTP 接口。
	// g.POST("/task/record", detail("流转记录", "record").
	// 	AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
	// 	Handle(ginx.B[RecordTaskReq](h.TaskRecord)),
	// )
	g.POST("/task/timeline", detail("流转时间线", "timeline").
		AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
		Handle(ginx.B[TaskTimelineReq](h.TaskTimeline)),
	)
	g.POST("/todo", list("所有待办工单", "todo").
		Needs("ticket:template:view_by_ids").
		AccessScope(ticketpbac.TodoProfile, ticketpbac.TodoPresets...).
		Handle(ginx.B[Todo](h.TodoAll)),
	)
	g.POST("/todo/user", list("我的待办工单", "my_todo").
		Needs("ticket:template:view_by_ids").
		Handle(ginx.B[Todo](h.TodoByUser)),
	)
	g.POST("/history", list("历史工单", "history").
		Needs("ticket:template:view_by_ids").
		AccessScope(ticketpbac.HistoryProfile, ticketpbac.HistoryPresets...).
		Handle(ginx.B[HistoryReq](h.History)),
	)
	g.POST("/start/user", list("我发起的工单", "my_start").
		Needs("ticket:template:view_by_ids").
		Handle(ginx.B[StartUserReq](h.StartUser)),
	)
	g.POST("/pass", op("同意审批", "pass").
		Handle(ginx.B[PassOrderReq](h.Pass)),
	)
	g.POST("/reject", op("驳回审批", "reject").
		Handle(ginx.B[RejectOrderReq](h.Reject)),
	)
	g.POST("/transfer", op("转交审批人", "transfer").
		Needs("iam:user:view").
		Handle(ginx.B[TransferReq](h.Transfer)),
	)
	g.POST("/revoke", op("撤销工单", "revoke").
		Handle(ginx.B[RevokeOrderReq](h.Revoke)),
	)
	g.POST("/rating/submit", op("评价工单", "rate").
		Handle(ginx.B[SubmitRatingReq](h.SubmitRating)),
	)
	g.POST("/task/form_config", detail("任务节点表单配置", "form_config").
		Needs("ticket:template:get", "ticket:manager:get").
		Handle(ginx.B[TaskFormConfigReq](h.GetTaskFormConfig)),
	)
	//g.POST("/upstream/:task_id", detail("查询上游处理节点", "upstream").
	//	Handle(ginx.W(h.Upstream)),
	//)
}

func (h *Handler) GetTaskFormConfig(ctx *ginx.Context, req TaskFormConfigReq) (ginx.Result, error) {
	info, err := h.engineSvc.TaskInfo(ctx.Context, req.TaskId)
	if err != nil {
		return systemErrorResult, err
	}

	inst, err := h.engineSvc.GetInstanceByID(ctx.Context, info.ProcInstID)
	if err != nil {
		return systemErrorResult, err
	}

	wf, err := h.workflowSvc.FindInstanceFlow(ctx.Context, req.WorkflowId, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return systemErrorResult, err
	}

	nodes, err := easyflow.ParseNodes(wf.FlowData.Nodes)
	if err != nil {
		return systemErrorResult, err
	}

	for _, node := range nodes {
		if node.ID != info.NodeID {
			continue
		}

		property, err1 := easyflow.ToNodeProperty[easyflow.UserProperty](node)
		if err1 != nil {
			return systemErrorResult, err1
		}

		return ginx.Result{
			Data: property.Fields,
			Msg:  "获取任务表单配置成功",
		}, nil
	}

	return ginx.Result{
		Data: []easyflow.Field{},
		Msg:  "未找到对应任务配置",
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

func (h *Handler) TodoAll(ctx *ginx.Context, req Todo) (ginx.Result, error) {
	instances, total, err := h.engineSvc.ListAllTodoTasks(ctx.Context, req.UserId, req.ProcessName, req.SortByAsc, int(req.Offset), int(req.Limit))
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

	procInstIds := slice.Map(tickets, func(idx int, src domain.Ticket) int {
		return src.Process.InstanceId
	})

	processTasks, err := h.engineSvc.ListPendingStepsOfMyTask(ctx.Context, procInstIds, username)
	if err != nil {
		return systemErrorResult, err
	}

	tasks, err := h.toVoEngineTicket(ctx.Context, processTasks)
	if err != nil {
		return systemErrorResult, err
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

	userIDs := slice.Map(ts, func(idx int, src model.Task) string {
		return src.UserID
	})

	uniqueUserIDs := make([]string, 0, len(userIDs))
	for _, uid := range userIDs {
		if !slice.Contains(uniqueUserIDs, uid) {
			uniqueUserIDs = append(uniqueUserIDs, uid)
		}
	}

	uMap, err := h.getUserMap(ctx, uniqueUserIDs)
	if err != nil {
		uMap = make(map[string]string)
	}

	taskIds := slice.Map(ts, func(idx int, src model.Task) int {
		return src.TaskID
	})
	taskDataMap, err := h.svc.ListTaskFormsByTaskIDs(ctx, taskIds)
	if err != nil {
		taskDataMap = make(map[int][]domain.FormValue)
	}

	return slice.Map(ts, func(idx int, src model.Task) TaskRecord {
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
			FormValues: slice.Map(taskDataMap[src.TaskID], func(idx int, src domain.FormValue) FormValue {
				return FormValue{
					Name:  src.Name,
					Key:   src.Key,
					Type:  src.Type,
					Value: src.Value,
				}
			}),
		}
	})
}

func timelineActors(records []TaskRecord) []string {
	actors := make([]string, 0, len(records))
	for _, record := range records {
		// 1 和 2 分别表示人工通过、人工驳回；系统联动状态不作为主处理人。
		if record.Status != 1 && record.Status != 2 {
			continue
		}
		if !slice.Contains(actors, record.ApprovedBy) {
			actors = append(actors, record.ApprovedBy)
		}
	}
	return actors
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
		Id:                req.Id,
		TemplateId:        req.TemplateId,
		Starter:           req.CreateBy,
		ProcessInstanceId: req.Process.InstanceId,
		Provide:           req.Provide.ToUint8(),
		Status:            req.Status.ToUint8(),
		WorkflowId:        req.WorkflowId,
		Ctime:             time.Unix(req.Ctime/1000, 0).Format("2006-01-02 15:04:05"),
		Wtime:             time.Unix(req.Wtime/1000, 0).Format("2006-01-02 15:04:05"),
		Data:              req.Data,
		RevokeReason:      req.RevokeReason,
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
	ticketIDs := slice.Map(os, func(_ int, ticket domain.Ticket) int64 { return ticket.Id })
	ratings, err := h.ratingSvc.ListByTicketIDs(ctx.Context, ticketIDs)
	if err != nil {
		return systemErrorResult, err
	}

	uniqueMap := make(map[string]bool)
	uns := slice.FilterMap(os, func(idx int, src domain.Ticket) (string, bool) {
		if !uniqueMap[src.CreateBy] {
			uniqueMap[src.CreateBy] = true
			return src.CreateBy, true
		}

		return src.CreateBy, false
	})

	uMap, err := h.getUserMap(ctx.Context, uns)
	if err != nil {
		uMap = make(map[string]string)
	}

	return ginx.Result{
		Data: RetrieveTickets{
			Total: total,
			Tasks: slice.Map(os, func(idx int, src domain.Ticket) Ticket {
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

	uniqueProcInstIds := make(map[int]bool)
	procInstIds := slice.FilterMap(instances, func(idx int, src domain.Instance) (int, bool) {
		if !uniqueProcInstIds[src.ProcInstID] {
			uniqueProcInstIds[src.ProcInstID] = true
			return src.ProcInstID, true
		}

		return src.ProcInstID, false
	})

	us, err := h.getUsers(ctx, instances)
	if err != nil {
		us = make(map[string]string)
	}

	os, err := h.svc.ListByProcessInstanceIDs(ctx, procInstIds)
	if err != nil {
		return nil, err
	}
	m := slice.ToMap(os, func(element domain.Ticket) int {
		return element.Process.InstanceId
	})

	return slice.Map(instances, func(idx int, src domain.Instance) Ticket {
		val, _ := m[src.ProcInstID]
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
			Id:                 val.Id,
			TaskId:             src.TaskID,
			ProcessInstanceId:  src.ProcInstID,
			Starter:            starter,
			CurrentStep:        src.CurrentNodeName,
			ApprovedBy:         approved,
			ProcInstCreateTime: createTimeStr,
			Provide:            val.Provide.ToUint8(),
			Status:             val.Status.ToUint8(),
			TemplateId:         val.TemplateId,
			WorkflowId:         val.WorkflowId,
			Ctime:              ctime,
		}
	}), nil
}

func (h *Handler) getUsers(ctx context.Context, instances []domain.Instance) (map[string]string, error) {
	var uns []string
	uniqueMap := make(map[string]bool)

	approved := slice.FilterMap(instances, func(idx int, src domain.Instance) (string, bool) {
		if src.ApprovedBy != "" && !uniqueMap[src.ApprovedBy] {
			uniqueMap[src.ApprovedBy] = true
			return src.ApprovedBy, true
		}
		return "", false
	})

	starter := slice.FilterMap(instances, func(idx int, src domain.Instance) (string, bool) {
		if src.Starter != "" && !uniqueMap[src.Starter] {
			uniqueMap[src.Starter] = true
			return src.Starter, true
		}
		return "", false
	})

	uns = append(uns, approved...)
	uns = append(uns, starter...)

	return h.getUserMap(ctx, uns)
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

	// 记录 admin 操作别人任务的日志
	if resp.User.IsAdmin && tInfo.UserID != resp.User.Username {
		fmt.Printf("Admin %s 操作了非自己提交的任务 taskId=%d, 原指派人=%s\n", resp.User.Username, taskId, tInfo.UserID)
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

	return slice.ToMapV(resp.Users, func(element *userv1.User) (string, string) {
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
