package dispatch

import (
	"github.com/Duke1616/eflow/internal/domain"
	dispatchSvc "github.com/Duke1616/eflow/internal/service/dispatch"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

var systemErrorResult = ginx.Result{Code: 500, Msg: "系统内部错误"}

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
	g.POST("/create", h.Capability("创建执行单元路由", "add").
		Needs("ticket:workflow:view_automation_nodes", "task:runner:view_by_ids").
		Handle(ginx.B[CreateDispatchReq](h.Create)),
	)
	g.POST("/update", h.Capability("修改执行单元路由", "edit").
		Needs("ticket:workflow:view_automation_nodes", "task:runner:view_by_ids").
		Handle(ginx.B[UpdateDispatchReq](h.Update)),
	)
	g.POST("/delete", h.Capability("删除执行单元路由", "delete").
		Handle(ginx.B[DeleteDispatchReq](h.Delete)),
	)
	g.POST("/sync", h.Capability("复制执行单元路由", "sync").
		Needs("ticket:template:view_by_workflow_id").
		Handle(ginx.B[SyncDispatchReq](h.Sync)),
	)
	g.POST("/list/by_template_id", h.Capability("执行单元路由列表", "view").
		Needs("ticket:template:get", "task:runner:view_by_ids", "task:runner:view_by_codebook_id",
			"ticket:workflow:view_automation_nodes").
		Handle(ginx.B[ListByTemplateId](h.ListByTemplateId)),
	)
}

func (h *Handler) Create(ctx *ginx.Context, req CreateDispatchReq) (ginx.Result, error) {
	id, err := h.svc.Create(ctx.Context, h.toDomain(req))
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg:  "创建成功",
		Data: id,
	}, nil
}

func (h *Handler) Delete(ctx *ginx.Context, req DeleteDispatchReq) (ginx.Result, error) {
	id, err := h.svc.Delete(ctx.Context, req.Id)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg:  "删除成功",
		Data: id,
	}, nil
}

func (h *Handler) ListByTemplateId(ctx *ginx.Context, req ListByTemplateId) (ginx.Result, error) {
	rts, total, err := h.svc.ListByTemplateId(ctx.Context, req.Offset, req.Limit, req.TemplateId)
	if err != nil {
		return systemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询执行单元路由规则成功",
		Data: RetrieveDispatches{
			Total: total,
			Dispatches: slice.Map(rts, func(idx int, src domain.Dispatch) Dispatch {
				return h.toDispatchVo(src)
			}),
		},
	}, nil
}

func (h *Handler) Sync(ctx *ginx.Context, req SyncDispatchReq) (ginx.Result, error) {
	count, err := h.svc.ReplaceFromTemplate(ctx.Context, req.TemplateId, req.SyncTemplateId)
	if err != nil {
		return systemErrorResult, err
	}
	return ginx.Result{
		Msg:  "复制成功",
		Data: count,
	}, nil
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateDispatchReq) (ginx.Result, error) {
	id, err := h.svc.Update(ctx.Context, h.toUpdateDomain(req))
	if err != nil {
		return systemErrorResult, err
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
