package task

import (
	"encoding/json"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	capability.IRegistry
	svc taskSvc.Service
}

func NewHandler(svc taskSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("ticket", "task", "自动化任务管理"),
	}
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/task")
	g.POST("/list", h.Capability("查询任务列表", "view").
		Handle(ginx.B[ListTaskReq](h.ListTask)),
	)
	g.POST("/list/by_instance_id", h.Capability("按流程实例查询任务", "view_by_instance_id").
		Handle(ginx.B[ListTaskByInstanceIDReq](h.ListTaskByInstanceID)),
	)
	g.POST("/update/args", h.Capability("修改任务参数", "update_args").
		Handle(ginx.B[UpdateArgsReq](h.UpdateArgs)),
	)
	g.POST("/update/variables", h.Capability("修改任务变量", "update_variables").
		Handle(ginx.B[UpdateVariablesReq](h.UpdateVariableReq)),
	)
	g.POST("/retry", h.Capability("重试任务", "retry").
		Handle(ginx.B[RetryReq](h.Retry)),
	)
	g.POST("/success", h.Capability("手动置为成功", "success").
		Handle(ginx.B[UpdateStatusToSuccessReq](h.UpdateStatusToSuccess)),
	)
	g.GET("/logs/:task_id", h.Capability("查询任务日志", "logs").
		Handle(ginx.W(h.Logs)),
	)
}

func (h *Handler) ListTask(ctx *ginx.Context, req ListTaskReq) (ginx.Result, error) {
	ws, total, err := h.svc.ListTask(ctx.Context, req.Offset, req.Limit)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{
		Msg: "查询 task 列表成功",
		Data: RetrieveTasks{
			Total: total,
			Tasks: slice.Map(ws, func(idx int, src domain.Task) Task {
				return h.toTaskVo(src)
			}),
		},
	}, nil
}

func (h *Handler) ListTaskByInstanceID(ctx *ginx.Context, req ListTaskByInstanceIDReq) (ginx.Result, error) {
	ws, total, err := h.svc.ListTaskByInstanceID(ctx.Context, req.Offset, req.Limit, req.InstanceID)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{
		Msg: "查询 task 列表成功",
		Data: RetrieveTasks{
			Total: total,
			Tasks: slice.Map(ws, func(idx int, src domain.Task) Task {
				return h.toTaskVo(src)
			}),
		},
	}, nil
}

func (h *Handler) UpdateStatusToSuccess(ctx *ginx.Context, req UpdateStatusToSuccessReq) (ginx.Result, error) {
	count, err := h.svc.UpdateTaskStatus(ctx.Context, domain.TaskResult{
		Id:              req.Id,
		TriggerPosition: domain.TriggerPositionManualSuccess.ToString(),
		Status:          domain.SUCCESS,
	})
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Data: count, Msg: "消息状态修改为成功"}, nil
}

func (h *Handler) Logs(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("task_id").Int64()
	if err != nil {
		return systemErrorResult, err
	}
	tInfo, err := h.svc.FindTaskByID(ctx.Context, id)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Code: 0, Msg: "获取日志成功", Data: tInfo.Result}, nil
}

func (h *Handler) UpdateArgs(ctx *ginx.Context, req UpdateArgsReq) (ginx.Result, error) {
	count, err := h.svc.UpdateArgs(ctx.Context, req.Id, req.Args)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Data: count, Msg: "修改Args成功"}, nil
}

func (h *Handler) UpdateVariableReq(ctx *ginx.Context, req UpdateVariablesReq) (ginx.Result, error) {
	variables, err := h.toVariablesDomain(req.Variables)
	if err != nil {
		return systemErrorResult, err
	}
	count, err := h.svc.UpdateVariables(ctx.Context, req.Id, variables)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Data: count, Msg: "修改Variables成功"}, nil
}

func (h *Handler) Retry(ctx *ginx.Context, req RetryReq) (ginx.Result, error) {
	if err := h.svc.RetryTask(ctx.Context, req.Id); err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{Msg: "重试任务成功"}, nil
}

func (h *Handler) toTaskVo(req domain.Task) Task {
	args, _ := json.Marshal(req.Args)
	scheduledTime := ""
	if req.ScheduledTime > 0 {
		scheduledTime = time.UnixMilli(req.ScheduledTime).Format("2006-01-02 15:04:05")
	} else if req.Utime > 0 {
		scheduledTime = time.UnixMilli(req.Utime).Format("2006-01-02 15:04:05")
	}
	return Task{
		Id:              req.Id,
		TicketID:        req.TicketID,
		Language:        req.Language,
		Code:            req.Code,
		Kind:            string(req.Kind),
		CodebookUid:     req.CodebookUid,
		Target:          req.Target,
		Handler:         req.Handler,
		Status:          Status(req.Status),
		Result:          req.Result,
		Args:            string(args),
		IsTiming:        req.IsTiming,
		ScheduledTime:   scheduledTime,
		StartTime:       formatMilli(req.StartTime),
		EndTime:         formatMilli(req.EndTime),
		RetryCount:      req.RetryCount,
		TriggerPosition: req.TriggerPosition,
		Variables:       desensitization(req.Variables),
	}
}

func formatMilli(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.UnixMilli(ts).Format("2006-01-02 15:04:05")
}

func desensitization(req []domain.Variables) string {
	variablesJson := slice.Map(req, func(idx int, src domain.Variables) Variables {
		if src.Secret {
			return Variables{Key: src.Key, Value: "********", Secret: src.Secret}
		}
		return Variables{Key: src.Key, Value: src.Value, Secret: src.Secret}
	})
	vars, _ := json.Marshal(variablesJson)
	return string(vars)
}

func (h *Handler) toVariablesDomain(variables string) ([]domain.Variables, error) {
	var vars []Variables
	if variables == "" {
		return nil, nil
	}
	err := json.Unmarshal([]byte(variables), &vars)
	if err != nil {
		return nil, err
	}
	return slice.Map(vars, func(idx int, src Variables) domain.Variables {
		return domain.Variables{Key: src.Key, Value: src.Value, Secret: src.Secret}
	}), nil
}
