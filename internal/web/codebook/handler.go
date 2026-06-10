package codebook

import (
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	codebookSvc "github.com/Duke1616/eflow/internal/service/codebook"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

type Handler struct {
	capability.IRegistry
	svc codebookSvc.Service
}

func NewHandler(svc codebookSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("ticket", "codebook", "脚本引擎"),
	}
}

func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/codebook")

	g.POST("/create", h.Capability("创建脚本模板", "add").
		Handle(ginx.B[CreateReq](h.Create)),
	)
	g.POST("/list", h.Capability("脚本模板列表", "view").
		Handle(ginx.B[ListReq](h.List)),
	)
	g.GET("/detail/:id", h.Capability("脚本模板详情", "get").
		Handle(ginx.W(h.Detail)),
	)
	g.POST("/update", h.Capability("更新脚本模板", "edit").
		Handle(ginx.B[UpdateReq](h.Update)),
	)
	g.DELETE("/delete/:id", h.Capability("删除脚本模板", "delete").
		Handle(ginx.W(h.Delete)),
	)
}

func (h *Handler) Create(ctx *ginx.Context, req CreateReq) (ginx.Result, error) {
	id, err := h.svc.Create(ctx.Context, h.toDomain(req))
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: id,
	}, nil
}

func (h *Handler) Detail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return ErrCodebookInvalidId, err
	}

	t, err := h.svc.GetByID(ctx.Context, id)
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Data: h.toCodebookVo(t),
	}, nil
}

func (h *Handler) List(ctx *ginx.Context, req ListReq) (ginx.Result, error) {
	rts, total, err := h.svc.List(ctx.Context, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询脚本模板列表成功",
		Data: ListCodebooksResp{
			Total: total,
			Codebooks: slice.Map(rts, func(idx int, src domain.Codebook) CodebookVO {
				return h.toCodebookVo(src)
			}),
		},
	}, nil
}

func (h *Handler) Update(ctx *ginx.Context, req UpdateReq) (ginx.Result, error) {
	count, err := h.svc.Update(ctx.Context, h.toUpdateDomain(req))
	if err != nil {
		return h.translateError(err), err
	}

	return ginx.Result{
		Data: count,
	}, nil
}

func (h *Handler) Delete(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return ErrCodebookInvalidId, err
	}

	count, err := h.svc.Delete(ctx.Context, id)
	if err != nil {
		return h.translateError(err), err
	}
	return ginx.Result{
		Data: count,
	}, nil
}

func (h *Handler) translateError(err error) ginx.Result {
	if errors.Is(err, errs.ErrInvalidParameter) {
		return ErrInvalidParameter(err)
	}
	return SystemErrorResult
}

func (h *Handler) toDomain(req CreateReq) domain.Codebook {
	return domain.Codebook{
		Name:       req.Name,
		Owner:      req.Owner,
		Code:       req.Code,
		Language:   req.Language,
		Identifier: req.Identifier,
	}
}

func (h *Handler) toUpdateDomain(req UpdateReq) domain.Codebook {
	return domain.Codebook{
		Id:       req.Id,
		Name:     req.Name,
		Owner:    req.Owner,
		Code:     req.Code,
		Language: req.Language,
	}
}

func (h *Handler) toCodebookVo(req domain.Codebook) CodebookVO {
	return CodebookVO{
		Id:         req.Id,
		Name:       req.Name,
		Owner:      req.Owner,
		Code:       req.Code,
		Language:   req.Language,
		Secret:     req.Secret,
		Identifier: req.Identifier,
	}
}
