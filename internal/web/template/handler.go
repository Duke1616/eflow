package template

import (
	"encoding/json"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
)

// Handler 整合工单模板及分类分组的 Web 路由处理器
type Handler struct {
	capability.IRegistry
	svc templateSvc.Service
}

// NewHandler 初始化工单模板控制器并接入 EIAM 统一安全权限保护
func NewHandler(svc templateSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		IRegistry: capability.NewRegistry("ticket", "template", "工单模板管理"),
	}
}

// PrivateRoutes 注册需要经过登陆校验及安全 Capability 策略检查的私有路由
func (h *Handler) PrivateRoutes(server *gin.Engine) {
	// --- Template 工单模板业务路由 ---
	g := server.Group("/api/template")
	g.GET("/detail/:id", h.Capability("查询工单模板详情", "get").
		Handle(ginx.W(h.DetailTemplate)),
	)
	g.POST("/list", h.Capability("查询工单模板列表", "view").
		Handle(ginx.B[ListTemplateReq](h.ListTemplate)),
	)
	g.POST("/list/pipeline", h.Capability("分类聚合工单模板", "pipeline").
		Handle(ginx.W(h.Pipeline)),
	)
	g.POST("/by_ids", h.Capability("批量获取模板详情", "view_by_ids").
		Handle(ginx.B[FindByTemplateIds](h.FindByTemplateIds)),
	)
	g.POST("/get_by_workflow_id", h.Capability("按流程ID获取模板", "view_by_workflow_id").
		Handle(ginx.B[GetTemplatesByWorkFlowIdReq](h.GetTemplatesByWorkflowId)),
	)
	g.POST("/rules/by_workflow_id", h.Capability("获取流程绑定模板校验链", "rules_by_workflow_id").
		Handle(ginx.B[GetRulesByWorkFlowIdReq](h.GetRulesByWorkFlowId)),
	)
	g.POST("/list/by_keyword", h.Capability("模糊搜索模板", "view_by_keyword").
		Handle(ginx.B[ByKeywordReq](h.ByKeyword)),
	)
	g.POST("/favorite/toggle", h.Capability("收藏或取消工单模板", "toggle_favorite").
		Handle(ginx.B[ToggleFavoriteReq](h.ToggleFavorite)),
	)
	g.POST("/favorite/list", h.Capability("查询模板收藏夹", "view_favorite").
		Handle(ginx.W(h.ListFavoriteTemplates)),
	)
	g.POST("/create", h.Capability("创建工单模板", "add").
		Handle(ginx.B[CreateTemplateReq](h.CreateTemplate)),
	)
	g.POST("/update", h.Capability("修改工单模板", "edit").
		Handle(ginx.B[UpdateTemplateReq](h.UpdateTemplate)),
	)
	g.DELETE("/delete/:id", h.Capability("删除工单模板", "delete").
		Handle(ginx.W(h.DeleteTemplate)),
	)

	// --- TemplateGroup 工单分类分组路由 ---
	gg := server.Group("/api/template/group")
	gg.POST("/list", h.Capability("查询模板分组列表", "view_group").
		Handle(ginx.B[Page](h.ListTemplateGroup)),
	)
	gg.POST("/by_ids", h.Capability("批量查询模板组", "view_group_by_ids").
		Handle(ginx.B[FindTemplateGroupsByIdsReq](h.FindTemplateGroupByIds)),
	)
	gg.POST("/create", h.Capability("创建模板分类", "add_group").
		Handle(ginx.B[CreateTemplateGroupReq](h.CreateTemplateGroup)),
	)
}

// CreateTemplate 创建模板
func (h *Handler) CreateTemplate(ctx *ginx.Context, req CreateTemplateReq) (ginx.Result, error) {
	d, err := h.toDomain(req)
	if err != nil {
		return ErrInvalidParameter(err), err
	}

	id, err := h.svc.CreateTemplate(ctx.Context, d)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: id,
	}, nil
}

// FindByTemplateIds 根据模板 ID 列表批量拉取模板详情
func (h *Handler) FindByTemplateIds(ctx *ginx.Context, req FindByTemplateIds) (ginx.Result, error) {
	if len(req.Ids) == 0 {
		return ErrInvalidParameter(fmt.Errorf("输入模板 ID 列表不能为空")), nil
	}

	ts, err := h.svc.FindByTemplateIds(ctx.Context, req.Ids)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "获取多个模板信息成功",
		Data: RetrieveTemplates{
			Total: int64(len(ts)),
			Templates: slice.Map(ts, func(idx int, src domain.Template) TemplateJson {
				return h.toTemplateJsonVo(src)
			}),
		},
	}, nil
}

// DetailTemplate 获取单个模板的详细属性
func (h *Handler) DetailTemplate(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").Int64()
	if err != nil {
		return ErrTemplateInvalidId, err
	}

	t, err := h.svc.DetailTemplate(ctx.Context, id)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: h.toTemplateVo(t),
	}, nil
}

// GetRulesByWorkFlowId 提取并解析流程图绑定的所有表单校验与控件规则
func (h *Handler) GetRulesByWorkFlowId(ctx *ginx.Context, req GetRulesByWorkFlowIdReq) (ginx.Result, error) {
	wfs, err := h.svc.GetByWorkflowId(ctx.Context, req.WorkFlowId)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询流程绑定的表单规则成功",
		Data: RetrieveTemplateRules{
			TemplateRules: slice.Map(wfs, func(idx int, src domain.Template) TemplateRules {
				rs, _ := rule.ParseRules(src.Rules)
				r := slice.Map(rs, func(idx int, src rule.Rule) Rule {
					return Rule{
						Type:  src.Type,
						Field: src.Field,
						Title: src.Title,
					}
				})

				return TemplateRules{
					Rules: r,
					Id:    src.Id,
					Name:  src.Name,
				}
			}),
		},
	}, nil
}

// GetTemplatesByWorkflowId 查询指定工作流关联挂载的全部工单模板
func (h *Handler) GetTemplatesByWorkflowId(ctx *ginx.Context, req GetTemplatesByWorkFlowIdReq) (ginx.Result, error) {
	wfs, err := h.svc.GetByWorkflowId(ctx.Context, req.WorkFlowId)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询流程绑定的工单模板成功",
		Data: RetrieveTemplates{
			Templates: slice.Map(wfs, func(idx int, src domain.Template) TemplateJson {
				return h.toTemplateJsonVo(src)
			}),
		},
	}, nil
}

// ListTemplate 分页获取所有可用的工单模板
func (h *Handler) ListTemplate(ctx *ginx.Context, req ListTemplateReq) (ginx.Result, error) {
	ts, total, err := h.svc.ListTemplate(ctx.Context, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询工单模板列表成功",
		Data: RetrieveTemplates{
			Total: total,
			Templates: slice.Map(ts, func(idx int, src domain.Template) TemplateJson {
				return h.toTemplateJsonVo(src)
			}),
		},
	}, nil
}

// DeleteTemplate 删除指定的模板实体
func (h *Handler) DeleteTemplate(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").Int64()
	if err != nil {
		return ErrTemplateInvalidId, err
	}

	count, err := h.svc.DeleteTemplate(ctx.Context, id)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: count,
	}, nil
}

// UpdateTemplate 覆盖更新当前模板相关的表单及校验控制链
func (h *Handler) UpdateTemplate(ctx *ginx.Context, req UpdateTemplateReq) (ginx.Result, error) {
	if req.Id <= 0 {
		return ErrTemplateInvalidId, nil
	}

	d, err := h.toUpdateDomain(req)
	if err != nil {
		return ErrInvalidParameter(err), err
	}

	affectedRows, err := h.svc.UpdateTemplate(ctx.Context, d)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: affectedRows,
	}, nil
}

// ByKeyword 模糊搜索模板列表及相应计数汇总
func (h *Handler) ByKeyword(ctx *ginx.Context, req ByKeywordReq) (ginx.Result, error) {
	ts, total, err := h.svc.FindByKeyword(ctx.Context, req.Keyword, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "根据关键字搜索模板成功",
		Data: RetrieveTemplates{
			Total: total,
			Templates: slice.Map(ts, func(idx int, src domain.Template) TemplateJson {
				return h.toTemplateJsonVo(src)
			}),
		},
	}, nil
}

// Pipeline 获取系统默认模板并按其分类聚类输出（聚合及排序业务均已下沉下托至 Repository 内部完成）
func (h *Handler) Pipeline(ctx *ginx.Context) (ginx.Result, error) {
	pipeline, err := h.svc.Pipeline(ctx.Context)
	if err != nil {
		return SystemErrorResult, err
	}

	tc := slice.Map(pipeline, func(idx int, src domain.TemplateCombination) TemplateCombination {
		return TemplateCombination{
			Id:    src.Id,
			Name:  src.Name,
			Icon:  src.Icon,
			Total: int64(src.Total),
			Templates: slice.Map(src.Templates, func(idx int, src domain.Template) Template {
				return h.toTemplateVo(src)
			}),
		}
	})

	return ginx.Result{
		Data: RetrieveTemplateCombination{TemplateCombinations: tc},
	}, nil
}

// ToggleFavorite 切换当前用户针对工单模板的收藏状态
func (h *Handler) ToggleFavorite(ctx *ginx.Context, req ToggleFavoriteReq) (ginx.Result, error) {
	uid, err := h.getUid(ctx)
	if err != nil {
		return SystemErrorResult, err
	}

	status, err := h.svc.ToggleFavorite(ctx.Context, uid, req.TemplateId)
	if err != nil {
		return SystemErrorResult, err
	}

	msg := "已收藏"
	if !status {
		msg = "已取消收藏"
	}

	return ginx.Result{
		Data: status,
		Msg:  msg,
	}, nil
}

// ListFavoriteTemplates 拉取并呈现当前关联用户的全部模板收藏夹
func (h *Handler) ListFavoriteTemplates(ctx *ginx.Context) (ginx.Result, error) {
	uid, err := h.getUid(ctx)
	if err != nil {
		return SystemErrorResult, err
	}

	templates, err := h.svc.ListFavoriteTemplates(ctx.Context, uid)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "获取收藏的工单模板成功",
		Data: TemplateCombination{
			Total: int64(len(templates)),
			Templates: slice.Map(templates, func(idx int, src domain.Template) Template {
				return h.toTemplateVo(src)
			}),
		},
	}, nil
}

// --- TemplateGroup 工单分类分组 Web 实现 ---

// CreateTemplateGroup 新建模板分类分组
func (h *Handler) CreateTemplateGroup(ctx *ginx.Context, req CreateTemplateGroupReq) (ginx.Result, error) {
	id, err := h.svc.CreateGroup(ctx.Context, domain.TemplateGroup{
		Name: req.Name,
		Icon: req.Icon,
	})
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: id,
	}, nil
}

// FindTemplateGroupByIds 批量拉取特定的分组信息
func (h *Handler) FindTemplateGroupByIds(ctx *ginx.Context, req FindTemplateGroupsByIdsReq) (ginx.Result, error) {
	gs, err := h.svc.ListGroupsByIds(ctx.Context, req.Ids)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "根据 IDS 查询工单模板组成功",
		Data: RetrieveTemplateGroup{
			TemplateGroups: slice.Map(gs, func(idx int, src domain.TemplateGroup) TemplateGroup {
				return TemplateGroup{
					Id:   src.Id,
					Name: src.Name,
					Icon: src.Icon,
				}
			}),
		},
	}, nil
}

// ListTemplateGroup 分页检索分类模板分组
func (h *Handler) ListTemplateGroup(ctx *ginx.Context, req Page) (ginx.Result, error) {
	gs, total, err := h.svc.ListGroup(ctx.Context, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询工单模板组列表成功",
		Data: RetrieveTemplateGroup{
			Total: total,
			TemplateGroups: slice.Map(gs, func(idx int, src domain.TemplateGroup) TemplateGroup {
				return TemplateGroup{
					Id:   src.Id,
					Name: src.Name,
					Icon: src.Icon,
				}
			}),
		},
	}, nil
}

// --- 辅助映射处理转换 ---
func (h *Handler) getUid(ctx *ginx.Context) (int64, error) {
	uid := ctxutil.GetUserID(ctx).Int64()
	if uid == 0 {
		return 0, fmt.Errorf("获取 UserID 失败: %d", uid)
	}

	return uid, nil
}

func (h *Handler) toDomain(req CreateTemplateReq) (domain.Template, error) {
	var rulesData []map[string]interface{}
	if req.Rules != "" {
		if err := json.Unmarshal([]byte(req.Rules), &rulesData); err != nil {
			return domain.Template{}, err
		}
	}
	var optionsData map[string]interface{}
	if req.Options != "" {
		if err := json.Unmarshal([]byte(req.Options), &optionsData); err != nil {
			return domain.Template{}, err
		}
	}

	rules := slice.Map(rulesData, func(idx int, src map[string]interface{}) domain.Rule {
		return domain.Rule(src)
	})

	return domain.Template{
		Name:       req.Name,
		WorkflowId: req.WorkflowId,
		GroupId:    req.GroupId,
		Icon:       req.Icon,
		CreateType: domain.SystemCreate,
		Rules:      rules,
		Options:    domain.TemplateOptions(optionsData),
		Desc:       req.Desc,
	}, nil
}

func (h *Handler) toTemplateVo(req domain.Template) Template {
	rules, _ := json.Marshal(req.Rules)
	options, _ := json.Marshal(req.Options)
	return Template{
		Id:         req.Id,
		Name:       req.Name,
		WorkflowId: req.WorkflowId,
		GroupId:    req.GroupId,
		Icon:       req.Icon,
		Rules:      string(rules),
		Options:    string(options),
		CreateType: CreateType(req.CreateType),
		Desc:       req.Desc,
	}
}

func (h *Handler) toTemplateJsonVo(req domain.Template) TemplateJson {
	rules := slice.Map(req.Rules, func(idx int, src domain.Rule) map[string]interface{} {
		return src
	})

	return TemplateJson{
		Id:         req.Id,
		Name:       req.Name,
		WorkflowId: req.WorkflowId,
		GroupId:    req.GroupId,
		Icon:       req.Icon,
		CreateType: CreateType(req.CreateType),
		Rules:      rules,
		Options:    req.Options,
		Desc:       req.Desc,
	}
}

func (h *Handler) toUpdateDomain(req UpdateTemplateReq) (domain.Template, error) {
	var rulesData []map[string]interface{}
	if req.Rules != "" {
		if err := json.Unmarshal([]byte(req.Rules), &rulesData); err != nil {
			return domain.Template{}, err
		}
	}
	var optionsData map[string]interface{}
	if req.Options != "" {
		if err := json.Unmarshal([]byte(req.Options), &optionsData); err != nil {
			return domain.Template{}, err
		}
	}

	rules := slice.Map(rulesData, func(idx int, src map[string]interface{}) domain.Rule {
		return src
	})

	return domain.Template{
		Id:         req.Id,
		Name:       req.Name,
		Desc:       req.Desc,
		Icon:       req.Icon,
		GroupId:    req.GroupId,
		WorkflowId: req.WorkflowId,
		Rules:      rules,
		Options:    optionsData,
	}, nil
}
