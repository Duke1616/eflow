package task

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
	maxLogPageSize  = 1000
)

// Handler 提供流程自动化任务和执行尝试查询接口。
type Handler struct {
	capability.IRegistry
	svc taskSvc.Service
}

// NewHandler 创建自动化任务 HTTP Handler。
func NewHandler(svc taskSvc.Service) *Handler {
	return &Handler{svc: svc,
		IRegistry: capability.NewRegistry("ticket", "task", "工单中心/自动化任务")}
}

// PrivateRoutes 注册自动化任务私有接口。
func (h *Handler) PrivateRoutes(server *gin.Engine) {
	group := server.Group("/api/task")
	group.POST("/list", h.Capability("自动化任务列表", "view").
		Handle(ginx.B[ListTaskReq](h.ListTask)))
	group.POST("/list/by_instance_id", h.Capability("关联自动化任务", "view_tasks").
		Module("center").Group("工单中心/工单详情").
		Handle(ginx.B[ListTaskByInstanceIDReq](h.ListTaskByInstanceID)))
	group.POST("/retry", h.Capability("重试自动化任务", "retry").
		Handle(ginx.B[RetryReq](h.Retry)))
	group.POST("/attempt/list", h.Capability("执行尝试列表", "view_attempts").
		Needs("ticket:task:logs").
		Handle(ginx.B[ListAttemptsReq](h.ListAttempts)))
	group.POST("/attempt/logs", h.Capability("执行尝试日志", "logs").
		NoSync().
		Handle(ginx.B[LogsReq](h.Logs)))
}
func (h *Handler) ListTask(ctx *ginx.Context, req ListTaskReq) (ginx.Result, error) {
	if err := normalizePage(&req.Page); err != nil {
		return invalidParameterResult(err), nil
	}
	tasks, total, err := h.svc.ListTask(ctx, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Msg: "success", Data: RetrieveTasks{Total: total, Tasks: mapTasks(tasks)}}, nil
}

func (h *Handler) ListTaskByInstanceID(ctx *ginx.Context,
	req ListTaskByInstanceIDReq) (ginx.Result, error) {
	if req.InstanceID <= 0 {
		return invalidParameterResult(fmt.Errorf("流程实例 ID 非法")), nil
	}
	if err := normalizePage(&req.Page); err != nil {
		return invalidParameterResult(err), nil
	}
	tasks, total, err := h.svc.ListTaskByInstanceID(ctx, req.Offset, req.Limit, req.InstanceID)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Msg: "success", Data: RetrieveTasks{Total: total, Tasks: mapTasks(tasks)}}, nil
}

func (h *Handler) Retry(ctx *ginx.Context, req RetryReq) (ginx.Result, error) {
	if req.ID <= 0 {
		return invalidParameterResult(fmt.Errorf("自动化任务 ID 非法")), nil
	}
	if err := h.svc.RetryTask(ctx, req.ID); err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Msg: "success"}, nil
}

func (h *Handler) ListAttempts(ctx *ginx.Context, req ListAttemptsReq) (ginx.Result, error) {
	if req.TaskID <= 0 {
		return invalidParameterResult(fmt.Errorf("自动化任务 ID 非法")), nil
	}
	attempts, err := h.svc.ListAttempts(ctx, req.TaskID)
	if err != nil {
		return systemErrorResult, err
	}
	result := make([]Attempt, 0, len(attempts))
	for _, attempt := range attempts {
		result = append(result, toAttemptVO(attempt))
	}
	return ginx.Result{Msg: "success", Data: ListAttemptsResp{Attempts: result}}, nil
}

func (h *Handler) Logs(ctx *ginx.Context, req LogsReq) (ginx.Result, error) {
	if req.AttemptID <= 0 || req.MinID < 0 {
		return invalidParameterResult(fmt.Errorf("执行尝试日志查询参数非法")), nil
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}
	if limit > maxLogPageSize {
		limit = maxLogPageSize
	}
	logs, maxID, err := h.svc.Logs(ctx, req.AttemptID, req.MinID, limit)
	if err != nil {
		return systemErrorResult, err
	}
	result := make([]ExecutionLog, 0, len(logs))
	for _, log := range logs {
		result = append(result, ExecutionLog{ID: log.ID, Time: log.Time, Content: log.Content})
	}
	return ginx.Result{Msg: "success", Data: LogsResp{Logs: result, MaxID: maxID}}, nil
}

func normalizePage(page *Page) error {
	if page.Offset < 0 {
		return fmt.Errorf("分页偏移量不能小于 0")
	}
	if page.Limit <= 0 {
		page.Limit = defaultPageSize
	}
	if page.Limit > maxPageSize {
		page.Limit = maxPageSize
	}
	return nil
}

func mapTasks(tasks []domain.Task) []Task {
	result := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		result = append(result, Task{
			ID: task.ID, TicketID: task.TicketID, ProcessInstanceID: task.ProcessInstanceID,
			NodeID: task.NodeID, NodeName: task.NodeName, ProcessVersion: task.ProcessVersion,
			Status: task.Status.ToUint8(), Phase: string(task.Phase),
			ScheduledAt: task.ScheduledAt, CurrentAttemptID: task.CurrentAttemptID,
			AdvancedAt: task.AdvancedAt, LastError: task.LastError,
			CTime: task.CTime, UTime: task.UTime,
		})
	}
	return result
}

func toAttemptVO(attempt domain.TaskAttempt) Attempt {
	return Attempt{
		ID: attempt.ID, TaskID: attempt.TaskID, AttemptNo: attempt.AttemptNo,
		RequestID: attempt.RequestID, RunnerID: attempt.RunnerID, ExecutionID: attempt.ExecutionID,
		Status: string(attempt.Status), Input: attempt.Input, Output: attempt.Output, Error: attempt.Error,
		SubmittedAt: attempt.SubmittedAt, CompletedAt: attempt.CompletedAt,
		CTime: attempt.CTime, UTime: attempt.UTime,
	}
}
