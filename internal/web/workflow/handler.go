package workflow

import (
	"encoding/json"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	easyflow2 "github.com/Duke1616/eflow/internal/pkg/easyflow"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/web/capability"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
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
		IRegistry: capability.NewRegistry("ticket", "workflow", "工作流管理"),
	}
}

// PrivateRoutes 注册需要登录及安全 Capability 拦截防护的私有路由组
func (h *Handler) PrivateRoutes(server *gin.Engine) {
	g := server.Group("/api/workflow")

	// 流程主实体写动作防护
	g.POST("/create", h.Capability("创建流程定义", "add").
		Handle(ginx.B[CreateReq](h.Create)),
	)
	g.POST("/update", h.Capability("修改流程定义", "edit").
		Handle(ginx.B[UpdateReq](h.Update)),
	)
	g.DELETE("/delete/:id", h.Capability("删除流程定义", "delete").
		Handle(ginx.W(h.Delete)),
	)
	g.POST("/deploy", h.Capability("发布部署流程", "deploy").
		Handle(ginx.B[DeployReq](h.Deploy)),
	)

	// 流程主实体读动作及模糊搜索
	g.POST("/list", h.Capability("查询流程模板列表", "view").
		Handle(ginx.B[ListReq](h.List)),
	)
	g.POST("/list/by_keyword", h.Capability("模糊检索流程模板", "view_by_keyword").
		Handle(ginx.B[ByKeywordReq](h.ByKeyword)),
	)
	g.GET("/detail/:id", h.Capability("查看工作流详情", "get").
		Handle(ginx.W(h.Detail)),
	)

	// 工单审批流转状态轨迹轨迹地图
	g.POST("/graph", h.Capability("查询工单流转地图", "graph").
		Handle(ginx.B[OrderGraphReq](h.FindOrderGraph)),
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
			Workflows: slice.Map(ws, func(idx int, src domain.Workflow) Workflow {
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
			Workflows: slice.Map(ws, func(idx int, src domain.Workflow) Workflow {
				return h.toWorkflowVo(src)
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
	id, err := ctx.Param("id").Int64()
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
	id, err := ctx.Param("id").Int64()
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
	// 1. 获取当前流转实例详情，以读取绑定的引擎模板 Process ID 及当前的 ProcVersion 版本
	inst, err := h.engineSvc.GetInstanceByID(ctx.Context, req.ProcessInstanceId)
	if err != nil {
		return SystemErrorResult, err
	}

	// 2. 根据实例运行时的版本信息，从物理快照层回溯读取锁死的画布拓扑 FlowData (保障版本敏感)
	flow, err := h.svc.FindInstanceFlow(ctx.Context, req.Id, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return SystemErrorResult, err
	}

	// 3. 全量提取流转过的审批任务记录，用于追溯辨识 status = 5 被系统抛弃/自动跳过的流转分支
	tasks, _, err := h.engineSvc.TaskRecord(ctx.Context, req.ProcessInstanceId, 0, 1000)
	if err != nil {
		return SystemErrorResult, err
	}

	// 4. 将审批记录整理生成 NodeID -> Status 最新状态的聚合 Map
	nodeStatusMap := make(map[string]int)
	for _, task := range tasks {
		nodeStatusMap[task.NodeID] = task.Status
	}

	// 5. 根据审批轨迹流转情况，计算出被点亮激活的边路径映射集
	edgeMap, err := h.engineSvc.GetTraversedEdges(ctx.Context, tasks, req.ProcessInstanceId, flow.ProcessId, req.Status)
	if err != nil {
		return SystemErrorResult, err
	}

	// 6. 将原始 LogicFlow 中的 Edges 序列化，使用 easyflow 画布算法根据轨迹状态点亮目标连线
	edgesJSON, _ := json.Marshal(flow.FlowData.Edges)
	var edges []easyflow2.Edge
	if err = json.Unmarshal(edgesJSON, &edges); err != nil {
		return SystemErrorResult, err
	}

	edges = easyflow2.UpdateEdgeProperties(edges, edgeMap, nodeStatusMap)

	// 7. 将更新好点亮轨迹属性后的 Edges 重新解包转换回前端渲染所需的 domain.FlowEdge 结构并呈递
	var newEdges []domain.FlowEdge
	newEdgesJSON, _ := json.Marshal(edges)
	_ = json.Unmarshal(newEdgesJSON, &newEdges)
	flow.FlowData.Edges = newEdges

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
			edges[i] = domain.FlowEdge(e)
		}
		nodes := make([]domain.FlowNode, len(req.FlowData.Nodes))
		for i, n := range req.FlowData.Nodes {
			nodes[i] = domain.FlowNode(n)
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
			edges[i] = domain.FlowEdge(e)
		}
		nodes := make([]domain.FlowNode, len(req.FlowData.Nodes))
		for i, n := range req.FlowData.Nodes {
			nodes[i] = domain.FlowNode(n)
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
			nodes[i] = map[string]interface{}(n)
		}
		edges := make([]map[string]interface{}, len(req.FlowData.Edges))
		for i, e := range req.FlowData.Edges {
			edges[i] = map[string]interface{}(e)
		}
		res.FlowData = &LogicFlow{
			Nodes: nodes,
			Edges: edges,
		}
	}

	return res
}
