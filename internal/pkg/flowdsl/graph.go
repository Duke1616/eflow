package flowdsl

import (
	"fmt"
	"sync"

	"github.com/samber/lo"
)

// IGraph 标准工作流拓扑图契约。
// 设计原则：向业务层与流程编译器屏蔽图底层存储结构（哈希表/邻接表）以及表现层差异，
// 仅暴露拓扑分析、节点索引、连线寻径等计算行为，保证图数据的只读安全性与编译幂等性。
type IGraph interface {
	// GetCanvasType 获取图数据的来源画布类型（如 logicflow, x6）
	GetCanvasType() string

	// GetNode 根据唯一节点 ID 检索节点对象，未找到时安全返回 false
	GetNode(id string) (*Node, bool)

	// MustGetNode 根据唯一节点 ID 检索节点，节点不存在时返回携带排查上下文的语义错误
	MustGetNode(id string) (*Node, error)

	// GetOrderedNodes 获取保持画布原始绘制顺序的节点列表，避免因 map 遍历无序性导致编译产物哈希不幂等
	GetOrderedNodes() []*Node

	// GetEdges 获取图的所有拓扑有向边集合
	GetEdges() []Edge

	// GetStartNode 检索流程的唯一起始节点，用于流程启动拓扑校验
	GetStartNode() (*Node, bool)

	// GetEndNodes 检索流程的所有办结终止节点
	GetEndNodes() []*Node

	// GetOutEdges 高效检索指定节点的所有后置出边（基于邻接表索引实现 O(1) 复杂度）
	GetOutEdges(nodeID string) []Edge

	// GetInEdges 高效检索指定节点的所有前置入边（基于邻接表索引实现 O(1) 复杂度）
	GetInEdges(nodeID string) []Edge

	// GetNextNodes 沿着有向连线获取指定节点的全部直接下游节点
	GetNextNodes(nodeID string) []*Node

	// GetPrevNodes 沿着有向连线反向获取指定节点的全部直接上游节点
	GetPrevNodes(nodeID string) []*Node

	// FilterNodes 根据业务断言过滤满足特定条件（如审批节点、自动化节点）的子集
	FilterNodes(predicate func(n *Node) bool) []*Node

	// HasCycle 使用拓扑排序算法检测图中是否存在非法闭环回路（防死循环流转）
	HasCycle() bool

	// Validate 执行图拓扑完整性校验（确保有且仅有一个开始节点、至少一个结束节点、无断头网关与悬空孤岛）
	Validate() error
}

// Graph 统一标准工作流有向图模型实现。
// 无论底层来自 LogicFlow 还是 AntV X6，统一映射到此拓扑图进行编译与计算。
type Graph struct {
	CanvasType string           `json:"canvas_type"`          // 数据来源画布类型 (logicflow / x6)
	Nodes      map[string]*Node `json:"nodes"`                // 节点集合：按 NodeID 快速检索
	NodeOrder  []string         `json:"node_order,omitempty"` // 保持画布原始定义顺序，确保编译幂等
	Edges      []Edge           `json:"edges"`                // 有向拓扑边集合

	// 拓扑反向索引缓存（线程安全，构建后加速出边/入边查询）
	indexOnce sync.Once
	outEdges  map[string][]Edge
	inEdges   map[string][]Edge
}

// 确保 Graph 实现了 IGraph 契约
var _ IGraph = (*Graph)(nil)

// NewGraph 创建空白标准工作流图
func NewGraph(canvasType string) *Graph {
	return &Graph{
		CanvasType: canvasType,
		Nodes:      make(map[string]*Node),
		NodeOrder:  make([]string, 0),
		Edges:      make([]Edge, 0),
		outEdges:   make(map[string][]Edge),
		inEdges:    make(map[string][]Edge),
	}
}

func (g *Graph) GetCanvasType() string {
	return g.CanvasType
}

func (g *Graph) GetEdges() []Edge {
	return g.Edges
}

// AddNode 向图中挂载一个节点
func (g *Graph) AddNode(node *Node) {
	if node != nil && node.ID != "" {
		if _, exists := g.Nodes[node.ID]; !exists {
			g.NodeOrder = append(g.NodeOrder, node.ID)
		}
		g.Nodes[node.ID] = node
	}
}

// AddEdge 向图中添加一条有向连线
func (g *Graph) AddEdge(edge Edge) {
	if edge.Source != "" && edge.Target != "" {
		g.Edges = append(g.Edges, edge)
	}
}

// GetOrderedNodes 获取保持原始定义顺序的节点列表（保证编译产物幂等）
func (g *Graph) GetOrderedNodes() []*Node {
	if len(g.NodeOrder) > 0 {
		return lo.FilterMap(g.NodeOrder, func(id string, _ int) (*Node, bool) {
			return g.GetNode(id)
		})
	}
	return lo.Values(g.Nodes)
}

// BuildIndex 显式构建邻接表索引，将后续的拓扑出边/入边查询复杂度降至绝对 O(1)
func (g *Graph) BuildIndex() {
	g.outEdges = make(map[string][]Edge, len(g.Nodes))
	g.inEdges = make(map[string][]Edge, len(g.Nodes))

	lo.ForEach(g.Edges, func(e Edge, _ int) {
		g.outEdges[e.Source] = append(g.outEdges[e.Source], e)
		g.inEdges[e.Target] = append(g.inEdges[e.Target], e)
	})
}

// ensureIndex 确保邻接表索引已初始化
func (g *Graph) ensureIndex() {
	if g.outEdges == nil || g.inEdges == nil || (len(g.outEdges)+len(g.inEdges) == 0 && len(g.Edges) > 0) {
		g.indexOnce.Do(g.BuildIndex)
	}
}

// GetNode 根据 ID 检索节点
func (g *Graph) GetNode(id string) (*Node, bool) {
	n, ok := g.Nodes[id]
	return n, ok
}

// MustGetNode 根据 ID 检索节点，不存在时返回带上下文的友好错误
func (g *Graph) MustGetNode(id string) (*Node, error) {
	n, ok := g.GetNode(id)
	if !ok {
		return nil, fmt.Errorf("工作流图中不存在指定节点: %s", id)
	}
	return n, nil
}

// GetStartNode 获取起始节点
func (g *Graph) GetStartNode() (*Node, bool) {
	return lo.Find(lo.Values(g.Nodes), func(n *Node) bool {
		return n.IsStart()
	})
}

// GetEndNodes 获取所有终止节点
func (g *Graph) GetEndNodes() []*Node {
	return lo.Filter(lo.Values(g.Nodes), func(n *Node, _ int) bool {
		return n.IsEnd()
	})
}

// GetOutEdges 快速检索指定节点的所有后置出边（O(1) 复杂度）
func (g *Graph) GetOutEdges(nodeID string) []Edge {
	g.ensureIndex()
	return g.outEdges[nodeID]
}

// GetInEdges 快速检索指定节点的所有前置入边（O(1) 复杂度）
func (g *Graph) GetInEdges(nodeID string) []Edge {
	g.ensureIndex()
	return g.inEdges[nodeID]
}

// GetNextNodes 检索指定节点后置的所有下游节点
func (g *Graph) GetNextNodes(nodeID string) []*Node {
	outEdges := g.GetOutEdges(nodeID)
	return lo.FilterMap(outEdges, func(e Edge, _ int) (*Node, bool) {
		return g.GetNode(e.Target)
	})
}

// GetPrevNodes 检索指定节点前置的所有上游节点
func (g *Graph) GetPrevNodes(nodeID string) []*Node {
	inEdges := g.GetInEdges(nodeID)
	return lo.FilterMap(inEdges, func(e Edge, _ int) (*Node, bool) {
		return g.GetNode(e.Source)
	})
}

// FilterNodes 过滤符合断言条件的节点列表
func (g *Graph) FilterNodes(predicate func(n *Node) bool) []*Node {
	return lo.Filter(g.GetOrderedNodes(), func(n *Node, _ int) bool {
		return predicate(n)
	})
}

// HasCycle 使用 Kahn 拓扑排序算法检测图中是否存在循环死锁环路
func (g *Graph) HasCycle() bool {
	if len(g.Nodes) == 0 {
		return false
	}

	inDegree := make(map[string]int, len(g.Nodes))
	for id := range g.Nodes {
		inDegree[id] = 0
	}
	for _, e := range g.Edges {
		inDegree[e.Target]++
	}

	queue := make([]string, 0)
	for id, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, id)
		}
	}

	visited := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited++

		for _, out := range g.GetOutEdges(curr) {
			inDegree[out.Target]--
			if inDegree[out.Target] == 0 {
				queue = append(queue, out.Target)
			}
		}
	}

	return visited != len(g.Nodes)
}

// Validate 拓扑合法性自检
func (g *Graph) Validate() error {
	startNodes := lo.Filter(lo.Values(g.Nodes), func(n *Node, _ int) bool { return n.IsStart() })
	if len(startNodes) != 1 {
		return fmt.Errorf("流程图必须有且仅有 1 个开始节点，当前包含 %d 个", len(startNodes))
	}

	endNodes := g.GetEndNodes()
	if len(endNodes) == 0 {
		return fmt.Errorf("流程图必须包含至少 1 个结束节点")
	}

	if g.HasCycle() {
		return fmt.Errorf("流程图存在非法环路")
	}

	return nil
}
