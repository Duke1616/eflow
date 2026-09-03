package workflow

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/pkg/contract/model"
	"github.com/Duke1616/eflow/pkg/contract/perm"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// Handler 整合工作流定义设计与流转地图的 Web 控制层路由器
type Handler struct {
	capability.IRegistry
	svc       workflowSvc.Service
	engineSvc engineSvc.Service
}

// NewHandler 初始化工作流 Web 控制器并接入 EIAM 统一安全权限防护
func NewHandler(svc workflowSvc.Service, engineSvc engineSvc.Service) *Handler {
	return &Handler{
		svc:       svc,
		engineSvc: engineSvc,
		IRegistry: capability.NewRegistry("ticket", "workflow", "流程管理"),
	}
}

// PrivateRoutes 注册需要登录及安全 Capability 拦截防护的私有路由组
func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/workflow")

	// 流程主实体写动作防护
	g.POST("/create", h.Define("创建流程", "add").
		Needs(perm.Workflow.Get, "iam:user:view").
		Bind(ginx.B[CreateReq](h.Create)),
	)
	g.POST("/update", h.Define("修改流程", "edit").
		Needs(perm.Workflow.Get, "iam:user:view").
		Bind(ginx.B[UpdateReq](h.Update)),
	)
	g.DELETE("/delete/:id", h.Define("删除流程", "delete").
		Bind(ginx.W(h.Delete)),
	)
	g.POST("/deploy", h.Define("流程发布", "deploy").
		Needs(perm.Workflow.Get).
		Bind(ginx.B[DeployReq](h.Deploy)),
	)

	// 流程主实体读动作及模糊搜索
	g.POST("/list", h.Define("流程列表", "view").
		Needs("iam:user:view").
		Bind(ginx.B[ListReq](h.List)),
	)
	g.POST("/list/by_keyword", h.Define("模糊检索流程模板", "view_by_keyword").
		NoSync().
		Bind(ginx.B[ByKeywordReq](h.ByKeyword)),
	)
	g.POST("/by_ids", h.Define("批量获取流程详情", "view_by_ids").
		NoSync().
		Bind(ginx.B[FindByIdsReq](h.FindByIds)),
	)
	g.POST("/automation/nodes", h.Define("查询流程自动化节点", "view_automation_nodes").
		NoSync().
		Bind(ginx.B[AutomationNodesReq](h.AutomationNodes)),
	)
	g.GET("/detail/:id", h.Define("流程详情", "get").
		Bind(ginx.W(h.Detail)),
	)

	// 工单审批流转状态轨迹地图（跨领域挂载到 ticket:manager 领域）
	g.POST("/graph", h.For(model.Manager).Define("流程轨迹图", "graph").
		Group("工单中心/工单详情").
		Bind(ginx.B[OrderGraphReq](h.FindOrderGraph)),
	)
}

// Create 创建流程定义
func (h *Handler) Create(ctx *ginx.Context, req CreateReq) (ginx.Result, error) {
	t, err := h.svc.Create(ctx.Context, h.toDomain(req))
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: t,
	}, nil
}

// List 分页拉取所有工作流流程模版
func (h *Handler) List(ctx *ginx.Context, req ListReq) (ginx.Result, error) {
	ws, total, err := h.svc.List(ctx.Context, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询流程模版列表成功",
		Data: RetrieveWorkflows{
			Total: total,
			Workflows: lo.Map(ws, func(src domain.Workflow, _ int) Workflow {
				return h.toWorkflowVo(src)
			}),
		},
	}, nil
}

// ByKeyword 根据关键字(匹配流程名字及描述)进行分页检索
func (h *Handler) ByKeyword(ctx *ginx.Context, req ByKeywordReq) (ginx.Result, error) {
	ws, total, err := h.svc.FindByKeyword(ctx.Context, req.Keyword, req.Offset, req.Limit)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "根据关键字搜索流程成功",
		Data: RetrieveWorkflows{
			Total: total,
			Workflows: lo.Map(ws, func(src domain.Workflow, _ int) Workflow {
				return h.toWorkflowVo(src)
			}),
		},
	}, nil
}

// FindByIds 根据 ID 列表批量拉取工作流元数据，常用于前端展示流程名称
func (h *Handler) FindByIds(ctx *ginx.Context, req FindByIdsReq) (ginx.Result, error) {
	if len(req.Ids) == 0 {
		return ginx.Result{
			Msg: "批量查询流程成功",
			Data: RetrieveWorkflows{
				Workflows: []Workflow{},
			},
		}, nil
	}

	ws, err := h.svc.FindByIds(ctx.Context, req.Ids)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "批量查询流程成功",
		Data: RetrieveWorkflows{
			Total: int64(len(ws)),
			Workflows: lo.Map(ws, func(src domain.Workflow, _ int) Workflow {
				return h.toWorkflowVo(src)
			}),
		},
	}, nil
}

// AutomationNodes 查询工作流画布中的自动化节点及默认执行单元。
func (h *Handler) AutomationNodes(ctx *ginx.Context, req AutomationNodesReq) (ginx.Result, error) {
	nodes, err := h.svc.GetAutomationNodes(ctx.Context, req.WorkflowId)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Msg: "查询流程自动化节点成功",
		Data: RetrieveAutomationNodes{
			AutomationNodes: lo.Map(nodes, func(src easyflow.AutomationNodeRef, _ int) AutomationNodeVO {
				return AutomationNodeVO{
					ID: src.ID, Name: src.Name, CodebookID: src.CodebookID, RunnerID: src.RunnerID,
				}
			}),
		},
	}, nil
}

// Deploy 发布流程拓扑至引擎控制端，并在物理数据层加锁持久化生成此刻画布快照
func (h *Handler) Deploy(ctx *ginx.Context, req DeployReq) (ginx.Result, error) {
	flow, err := h.svc.Find(ctx.Context, req.Id)
	if err != nil {
		return SystemErrorResult, fmt.Errorf("查询流程定义元数据失败: %w", err)
	}

	err = h.svc.Deploy(ctx.Context, flow)
	if err != nil {
		return SystemErrorResult, fmt.Errorf("发布流程失败: %w", err)
	}

	return ginx.Result{
		Data: h.toWorkflowVo(flow),
	}, nil
}

// Detail 获取指定流程定义主键 ID 的完整明细配置 (含 Edge/Node JSON 画布数据)
func (h *Handler) Detail(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return SystemErrorResult, fmt.Errorf("ID 格式错误: %w", err)
	}

	flow, err := h.svc.Find(ctx.Context, id)
	if err != nil {
		return SystemErrorResult, fmt.Errorf("查询流程模板详情失败: %w", err)
	}

	return ginx.Result{
		Data: h.toWorkflowVo(flow),
	}, nil
}

// Update 覆盖更新选定的流程元数据及画布节点线规则拓扑
func (h *Handler) Update(ctx *ginx.Context, req UpdateReq) (ginx.Result, error) {
	t, err := h.svc.Update(ctx.Context, h.toUpdateDomain(req))
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: t,
	}, nil
}

// Delete 物理删除选定的流程定义图，返回受影响行数
func (h *Handler) Delete(ctx *ginx.Context) (ginx.Result, error) {
	id, err := ctx.Param("id").AsInt64()
	if err != nil {
		return SystemErrorResult, fmt.Errorf("ID 格式错误: %w", err)
	}

	count, err := h.svc.Delete(ctx.Context, id)
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: count,
	}, nil
}

// FindOrderGraph 计算解析并生成已部署流程的流转地图，通过快照历史和任务记录点亮流转路径连线
func (h *Handler) FindOrderGraph(ctx *ginx.Context, req OrderGraphReq) (ginx.Result, error) {
	flow, err := h.svc.GetHighlightedGraph(ctx.Context, req.Id, req.ProcessInstanceId, domain.Status(req.Status))
	if err != nil {
		return SystemErrorResult, err
	}

	return ginx.Result{
		Data: RetrieveOrderGraph{
			Workflow: h.toWorkflowVo(flow),
		},
	}, nil
}

// --- 实体与表现层 VO 互相转换映射逻辑 ---

func (h *Handler) toDomain(req CreateReq) domain.Workflow {
	res := domain.Workflow{
		Name:         req.Name,
		Desc:         req.Desc,
		Icon:         req.Icon,
		Owner:        req.Owner,
		IsNotify:     req.IsNotify,
		NotifyMethod: domain.NotifyMethod(req.NotifyMethod),
		TemplateId:   req.TemplateId,
	}

	if req.FlowData != nil {
		edges := make([]domain.FlowEdge, len(req.FlowData.Edges))
		for i, e := range req.FlowData.Edges {
			edges[i] = e
		}
		nodes := make([]domain.FlowNode, len(req.FlowData.Nodes))
		for i, n := range req.FlowData.Nodes {
			nodes[i] = n
		}
		res.FlowData = domain.LogicFlow{
			Edges: edges,
			Nodes: nodes,
		}
	}

	return res
}

func (h *Handler) toUpdateDomain(req UpdateReq) domain.Workflow {
	res := domain.Workflow{
		Id:           req.Id,
		Name:         req.Name,
		Desc:         req.Desc,
		Owner:        req.Owner,
		IsNotify:     req.IsNotify,
		NotifyMethod: domain.NotifyMethod(req.NotifyMethod),
	}

	if req.FlowData != nil {
		edges := make([]domain.FlowEdge, len(req.FlowData.Edges))
		for i, e := range req.FlowData.Edges {
			edges[i] = e
		}
		nodes := make([]domain.FlowNode, len(req.FlowData.Nodes))
		for i, n := range req.FlowData.Nodes {
			nodes[i] = n
		}
		res.FlowData = domain.LogicFlow{
			Edges: edges,
			Nodes: nodes,
		}
	}

	return res
}

func (h *Handler) toWorkflowVo(req domain.Workflow) Workflow {
	res := Workflow{
		Id:           req.Id,
		Name:         req.Name,
		Desc:         req.Desc,
		Icon:         req.Icon,
		Owner:        req.Owner,
		IsNotify:     req.IsNotify,
		NotifyMethod: req.NotifyMethod.ToUint8(),
		TemplateId:   req.TemplateId,
	}

	if len(req.FlowData.Nodes) > 0 || len(req.FlowData.Edges) > 0 {
		nodes := make([]map[string]interface{}, len(req.FlowData.Nodes))
		for i, n := range req.FlowData.Nodes {
			nodes[i] = n
		}
		edges := make([]map[string]interface{}, len(req.FlowData.Edges))
		for i, e := range req.FlowData.Edges {
			edges[i] = e
		}
		res.FlowData = &LogicFlow{
			Nodes: nodes,
			Edges: edges,
		}
	}

	return res
}
