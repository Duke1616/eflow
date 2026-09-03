package template

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	"github.com/Duke1616/eflow/pkg/contract/permission"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
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
		IRegistry: capability.NewRegistry("ticket", "template", "工单模板"),
	}
}

// PrivateRoutes 注册需要经过登陆校验及安全 Capability 策略检查的私有路由
func (h *Handler) PrivateRoutes(server *gin.Engine) {
	// --- Template 工单模板业务路由 ---
	g := server.Group("/api/template")
	g.GET("/detail/:id", h.Define("工单模板详情", "get").
		Bind(ginx.W(h.DetailTemplate)),
	)
	g.POST("/list", h.Define("工单模板列表", "view").
		Needs(permission.Workflow.ViewByIds, permission.Template.ViewGroupSummary).
		Bind(ginx.B[ListTemplateReq](h.ListTemplate)),
	)

	g.POST("/by_ids", h.Define("批量获取模板详情", "view_by_ids").
		NoSync().
		Bind(ginx.B[FindByTemplateIds](h.FindByTemplateIds)),
	)
	g.POST("/get_by_workflow_id", h.Define("根据流程获取模板", "view_by_workflow_id").
		NoSync().
		Bind(ginx.B[GetTemplatesByWorkFlowIdReq](h.GetTemplatesByWorkflowId)),
	)
	g.POST("/rules/by_workflow_id", h.Define("获取流程绑定模板校验链", "rules_by_workflow_id").
		NoSync().
		Bind(ginx.B[GetRulesByWorkFlowIdReq](h.GetRulesByWorkFlowId)),
	)
	g.POST("/create", h.Define("创建工单模板", "add").
		Needs(permission.Template.ViewGroup, permission.Workflow.View, permission.Workflow.Get).
		Bind(ginx.B[CreateTemplateReq](h.CreateTemplate)),
	)
	g.POST("/update", h.Define("修改工单模板", "edit").
		Needs(permission.Template.Get, permission.Template.ViewGroup, permission.Workflow.View, permission.Workflow.Get).
		Bind(ginx.B[UpdateTemplateReq](h.UpdateTemplate)),
	)
	g.DELETE("/delete/:id", h.Define("删除工单模板", "delete").
		Bind(ginx.W(h.DeleteTemplate)),
	)

	// 收藏功能
	g.POST("/favorite/toggle", h.Define("收藏状态变更", "toggle_favorite").
		NoSync().
		Bind(ginx.B[ToggleFavoriteReq](h.ToggleFavorite)),
	)
	g.POST("/favorite/list", h.Define("模板收藏夹", "view_favorite").
		NoSync().
		Bind(ginx.W(h.ListFavoriteTemplates)),
	)

	// --- TemplateGroup 工单分类分组路由 ---
	gg := server.Group("/api/template/group")
	gg.POST("/list", h.Define("查询模板分组列表", "view_group").
		Group("工单模板/模板分类").
		NoSync().
		Bind(ginx.B[Page](h.ListTemplateGroup)),
	)
	gg.POST("/summary", h.Define("查询模板分组摘要", "view_group_summary").
		Group("工单模板/模板分类").
		NoSync().
		Bind(ginx.W(h.ListTemplateGroupSummary)),
	)
	gg.POST("/create", h.Define("创建模板分类", "add_group").
		Group("工单模板/模板分类").
		Bind(ginx.B[CreateTemplateGroupReq](h.CreateTemplateGroup)),
	)
	gg.POST("/update", h.Define("修改模板分类", "edit_group").
		Group("工单模板/模板分类").
		Bind(ginx.B[UpdateTemplateGroupReq](h.UpdateTemplateGroup)),
	)
	gg.DELETE("/delete/:id", h.Define("删除模板分类", "delete_group").
		Group("工单模板/模板分类").
		Bind(ginx.W(h.DeleteTemplateGroup)),
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
			Templates: lo.Map(ts, func(src domain.Template, _ int) TemplateJson {
				return h.toTemplateJsonVo(src)
			}),
		},
	}, nil
}

// DetailTemplate 获取单个模板的详细属性
func (h *Handler) DetailTemplate(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
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
			TemplateRules: lo.Map(wfs, func(src domain.Template, _ int) TemplateRules {
				rs, _ := rule.ParseRules(src.Rules)
				r := lo.Map(rs, func(src rule.Rule, _ int) Rule {
					return Rule{
						Type:  src.Type,
						Field: src.Field,
						Title: src.Title,
						Props: src.Props,
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
			Templates: lo.Map(wfs, func(src domain.Template, _ int) TemplateJson {
				return h.toTemplateJsonVo(src)
			}),
		},
	}, nil
}

// ListTemplate 分页获取所有可用的工单模板
func (h *Handler) ListTemplate(ctx *ginx.Context, req ListTemplateReq) (ginx.Result, error) {
	ts, total, err := h.svc.ListTemplate(ctx.Context, req.GroupId, req.Keyword, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询工单模板列表成功",
		Data: RetrieveTemplateList{
			Total: total,
			Templates: lo.Map(ts, func(src domain.Template, _ int) TemplateListItem {
				return h.toTemplateListItemVo(src)
			}),
		},
	}, nil
}

// DeleteTemplate 删除指定的模板实体
func (h *Handler) DeleteTemplate(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
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
			Templates: lo.Map(templates, func(src domain.Template, _ int) Template {
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

// UpdateTemplateGroup 修改模板分类分组
func (h *Handler) UpdateTemplateGroup(ctx *ginx.Context, req UpdateTemplateGroupReq) (ginx.Result, error) {
	if req.Id <= 0 {
		return ErrTemplateGroupInvalidId, nil
	}

	affectedRows, err := h.svc.UpdateGroup(ctx.Context, domain.TemplateGroup{
		Id:   req.Id,
		Name: req.Name,
		Icon: req.Icon,
	})
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg:  "修改工单模板组成功",
		Data: affectedRows,
	}, nil
}

// DeleteTemplateGroup 删除模板分类分组，分组下存在模板时拒绝删除
func (h *Handler) DeleteTemplateGroup(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return ErrTemplateGroupInvalidId, err
	}

	affectedRows, err := h.svc.DeleteGroup(ctx.Context, id)
	if err != nil {
		return h.translateGroupError(err), err
	}

	return ginx.Result{
		Msg:  "删除工单模板组成功",
		Data: affectedRows,
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
			TemplateGroups: lo.Map(gs, func(src domain.TemplateGroup, _ int) TemplateGroup {
				return TemplateGroup{
					Id:   src.Id,
					Name: src.Name,
					Icon: src.Icon,
				}
			}),
		},
	}, nil
}

// ListTemplateGroupSummary 查询模板分组摘要及每组模板数量
func (h *Handler) ListTemplateGroupSummary(ctx *ginx.Context) (ginx.Result, error) {
	summaries, err := h.svc.ListGroupSummaries(ctx.Context)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询工单模板组摘要成功",
		Data: RetrieveTemplateGroupSummary{
			Total: int64(len(summaries)),
			TemplateGroups: lo.Map(summaries, func(src domain.TemplateGroupSummary, _ int) TemplateGroupSummary {
				return TemplateGroupSummary{
					Id:    src.Id,
					Name:  src.Name,
					Icon:  src.Icon,
					Total: src.Total,
				}
			}),
		},
	}, nil
}

func (h *Handler) translateGroupError(err error) ginx.Result {
	if errors.Is(err, templateSvc.ErrTemplateGroupNotEmpty) {
		return ErrTemplateGroupNotEmpty
	}
	return SystemErrorResult
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

	rules := lo.Map(rulesData, func(src map[string]interface{}, _ int) domain.Rule {
		return domain.Rule(src)
	})

	return domain.Template{
		Name:              req.Name,
		WorkflowId:        req.WorkflowId,
		GroupId:           req.GroupId,
		Icon:              req.Icon,
		CreateType:        domain.SystemCreate,
		Rules:             rules,
		Options:           domain.TemplateOptions(optionsData),
		ScheduleOverrides: toDomainScheduleOverrides(req.ScheduleOverrides),
		Desc:              req.Desc,
	}, nil
}

func (h *Handler) toTemplateVo(req domain.Template) Template {
	rules, _ := json.Marshal(req.Rules)
	options, _ := json.Marshal(req.Options)
	return Template{
		Id:                req.Id,
		Name:              req.Name,
		WorkflowId:        req.WorkflowId,
		GroupId:           req.GroupId,
		Icon:              req.Icon,
		Rules:             string(rules),
		Options:           string(options),
		ScheduleOverrides: toScheduleOverridesVO(req.ScheduleOverrides),
		CreateType:        CreateType(req.CreateType),
		Desc:              req.Desc,
	}
}

func (h *Handler) toTemplateJsonVo(req domain.Template) TemplateJson {
	rules := lo.Map(req.Rules, func(src domain.Rule, _ int) map[string]interface{} {
		return src
	})

	return TemplateJson{
		Id:                req.Id,
		Name:              req.Name,
		WorkflowId:        req.WorkflowId,
		GroupId:           req.GroupId,
		Icon:              req.Icon,
		CreateType:        CreateType(req.CreateType),
		Rules:             rules,
		Options:           req.Options,
		ScheduleOverrides: toScheduleOverridesVO(req.ScheduleOverrides),
		Desc:              req.Desc,
	}
}

func (h *Handler) toTemplateListItemVo(req domain.Template) TemplateListItem {
	return TemplateListItem{
		Id:         req.Id,
		Name:       req.Name,
		WorkflowId: req.WorkflowId,
		GroupId:    req.GroupId,
		Icon:       req.Icon,
		CreateType: CreateType(req.CreateType),
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

	rules := lo.Map(rulesData, func(src map[string]interface{}, _ int) domain.Rule {
		return src
	})

	return domain.Template{
		Id:                req.Id,
		Name:              req.Name,
		Desc:              req.Desc,
		Icon:              req.Icon,
		GroupId:           req.GroupId,
		WorkflowId:        req.WorkflowId,
		Rules:             rules,
		Options:           optionsData,
		ScheduleOverrides: toDomainScheduleOverrides(req.ScheduleOverrides),
	}, nil
}

func toDomainScheduleOverrides(overrides map[string]ScheduleOverrideVO) domain.ScheduleOverrides {
	result := make(domain.ScheduleOverrides, len(overrides))
	for nodeID, override := range overrides {
		result[nodeID] = domain.ScheduleOverride{
			Type:      override.Type,
			Field:     override.Field,
			TimeField: override.TimeField,
			Unit:      override.Unit,
		}
	}
	return result
}

func toScheduleOverridesVO(overrides domain.ScheduleOverrides) map[string]ScheduleOverrideVO {
	result := make(map[string]ScheduleOverrideVO, len(overrides))
	for nodeID, override := range overrides {
		result[nodeID] = ScheduleOverrideVO{
			Type:      override.Type,
			Field:     override.Field,
			TimeField: override.TimeField,
			Unit:      override.Unit,
		}
	}
	return result
}
