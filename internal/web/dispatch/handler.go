package dispatch

import (
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	dispatchSvc "github.com/Duke1616/eflow/internal/service/dispatch"
	"github.com/Duke1616/eflow/pkg/contract/permission"
	"github.com/Duke1616/eiam/pkg/web/capability"
	etaskperm "github.com/Duke1616/etask/pkg/contract/permission"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

var systemErrorResult = ginx.Result{Code: 500, Msg: "系统内部错误"}

// handleDispatchError 统一提取执行单元路由中的业务结构化错误，避免参数或冲突错误被兜底为系统内部错误
func handleDispatchError(err error) (ginx.Result, error) {
	if err == nil {
		return ginx.Result{}, nil
	}
	if bizErr, ok := errors.AsType[*errs.Error](err); ok {
		return ginx.Result{Code: int(bizErr.Code), Msg: bizErr.Message}, nil
	}
	return systemErrorResult, err
}

type Handler struct {
	capability.IRegistry
	svc dispatchSvc.Service
}

func NewHandler(svc dispatchSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("ticket", "dispatch", "工单模板/执行单元路由"),
	}
}

func (h *Handler) PublicRoutes(server *gin.Engine) {
	// 目前无公共 API
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/dispatch")
	g.POST("/create", h.Define("创建执行单元路由", "add").
		Needs(permission.Workflow.ViewAutomationNodes, etaskperm.Runner.Ids).
		Bind(ginx.B[CreateDispatchReq](h.Create)),
	)
	g.POST("/update", h.Define("修改执行单元路由", "edit").
		Needs(permission.Workflow.ViewAutomationNodes, etaskperm.Runner.Ids).
		Bind(ginx.B[UpdateDispatchReq](h.Update)),
	)
	g.POST("/delete", h.Define("删除执行单元路由", "delete").
		Bind(ginx.B[DeleteDispatchReq](h.Delete)),
	)
	g.POST("/sync", h.Define("复制执行单元路由", "sync").
		Needs(permission.Template.ViewByWorkflowId).
		Bind(ginx.B[SyncDispatchReq](h.Sync)),
	)
	g.POST("/list/by_template_id", h.Define("执行单元路由列表", "view").
		Needs(permission.Template.Get, etaskperm.Runner.Ids, etaskperm.Runner.ViewExcludeCodebookId,
			permission.Workflow.ViewAutomationNodes).
		Bind(ginx.B[ListByTemplateId](h.ListByTemplateId)),
	)
}

func (h *Handler) Create(ctx *ginx.Context, req CreateDispatchReq) (ginx.Result, error) {
	id, err := h.svc.Create(ctx.Context, h.toDomain(req))
	if err != nil {
		return handleDispatchError(err)
	}

	return ginx.Result{
		Msg:  "创建成功",
		Data: id,
	}, nil
}

func (h *Handler) Delete(ctx *ginx.Context, req DeleteDispatchReq) (ginx.Result, error) {
	id, err := h.svc.Delete(ctx.Context, req.Id)
	if err != nil {
		return handleDispatchError(err)
	}

	return ginx.Result{
		Msg:  "删除成功",
		Data: id,
	}, nil
}

func (h *Handler) ListByTemplateId(ctx *ginx.Context, req ListByTemplateId) (ginx.Result, error) {
	rts, total, err := h.svc.ListByTemplateId(ctx.Context, req.Offset, req.Limit, req.TemplateId)
	if err != nil {
		return handleDispatchError(err)
	}

	return ginx.Result{
		Msg: "查询执行单元路由规则成功",
		Data: RetrieveDispatches{
			Total: total,
			Dispatches: lo.Map(rts, func(src domain.Dispatch, _ int) Dispatch {
				return h.toDispatchVo(src)
			}),
		},
	}, nil
}

func (h *Handler) Sync(ctx *ginx.Context, req SyncDispatchReq) (ginx.Result, error) {
	count, err := h.svc.ReplaceFromTemplate(ctx.Context, req.TemplateId, req.SyncTemplateId)
	if err != nil {
		return handleDispatchError(err)
	}
	return ginx.Result{
		Msg:  "复制成功",
		Data: count,
	}, nil
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateDispatchReq) (ginx.Result, error) {
	id, err := h.svc.Update(ctx.Context, h.toUpdateDomain(req))
	if err != nil {
		return handleDispatchError(err)
	}

	return ginx.Result{
		Msg:  "修改成功",
		Data: id,
	}, nil
}

func (h *Handler) toDomain(src CreateDispatchReq) domain.Dispatch {
	return domain.Dispatch{
		Field:            src.Field,
		RunnerId:         src.RunnerId,
		TemplateId:       src.TemplateId,
		AutomationNodeID: src.AutomationNodeID,
		Value:            src.Value,
		Priority:         src.Priority,
	}
}

func (h *Handler) toUpdateDomain(src UpdateDispatchReq) domain.Dispatch {
	return domain.Dispatch{
		Id:               src.Id,
		TemplateId:       src.TemplateId,
		AutomationNodeID: src.AutomationNodeID,
		Field:            src.Field,
		RunnerId:         src.RunnerId,
		Value:            src.Value,
		Priority:         src.Priority,
	}
}

func (h *Handler) toDispatchVo(src domain.Dispatch) Dispatch {
	return Dispatch{
		Id:               src.Id,
		Field:            src.Field,
		RunnerId:         src.RunnerId,
		TemplateId:       src.TemplateId,
		AutomationNodeID: src.AutomationNodeID,
		Value:            src.Value,
		Priority:         src.Priority,
	}
}
