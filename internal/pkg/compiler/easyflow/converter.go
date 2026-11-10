package easyflow

import (
	"fmt"
	"strings"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
)

// DefaultConverter easy-workflow 流程图编译器默认实现
type DefaultConverter struct {
	handlers map[flowdsl.NodeType]INodeHandler
}

// NewDefaultConverter 创建空白转换器
func NewDefaultConverter() *DefaultConverter {
	return &DefaultConverter{
		handlers: make(map[flowdsl.NodeType]INodeHandler),
	}
}

// NewDefaultConverterWithHandlers 创建已注册所有标准处理器的转换器
func NewDefaultConverterWithHandlers() *DefaultConverter {
	c := NewDefaultConverter()
	c.Register(&StartNodeHandler{})
	c.Register(&EndNodeHandler{})
	c.Register(&UserNodeHandler{})
	c.Register(&ParallelHandler{})
	c.Register(&SelectiveHandler{})
	c.Register(&ConditionHandler{})
	c.Register(&InclusionHandler{})
	c.Register(&AutomationNodeHandler{})
	c.Register(&ChatGroupNodeHandler{})
	return c
}

// Register 注册节点处理器
func (c *DefaultConverter) Register(handler INodeHandler) {
	c.handlers[handler.Type()] = handler
}

// Convert 执行转换流程 (Pipeline)
func (c *DefaultConverter) Convert(procName string, graph flowdsl.IGraph) (*model.Process, error) {
	if graph == nil {
		return nil, fmt.Errorf("流程拓扑图 (graph) 不能为空")
	}

	// 1. 初始化编译拓扑上下文
	ctx, err := c.initContext(procName, graph)
	if err != nil {
		return nil, err
	}

	// 2. 拓扑合法性校验：在节点转换前先验证流程图结构，将不合理的设计拦截在发布阶段
	if err = ValidateAll(ctx); err != nil {
		return nil, fmt.Errorf("流程图校验失败: %w", err)
	}

	// 3. 数据转换：遍历领域标准有序节点
	for _, node := range graph.GetOrderedNodes() {
		handler, ok := c.handlers[node.Type]
		if !ok {
			return nil, fmt.Errorf("unsupported node type: %s", node.Type)
		}

		generatedNodes, err := handler.Handle(ctx, node)
		if err != nil {
			return nil, fmt.Errorf("handle node [%s] failed: %w", node.ID, err)
		}
		ctx.OutputNodes = append(ctx.OutputNodes, generatedNodes...)
	}

	process := &model.Process{
		ProcessName:  procName,
		Source:       "工单系统",
		RevokeEvents: []string{EventRevoke},
		Nodes:        ctx.OutputNodes,
	}

	return process, nil
}

func (c *DefaultConverter) initContext(procName string, graph flowdsl.IGraph) (*Context, error) {
	orderedNodes := graph.GetOrderedNodes()
	ctx := &Context{
		ProcName:     procName,
		Graph:        graph,
		NodesMap:     make(map[string]*flowdsl.Node, len(orderedNodes)),
		EdgesMap:     make(map[string][]flowdsl.Edge),
		PrevNodesMap: make(map[string][]string),
		OutputNodes:  []model.Node{},
	}

	for _, n := range orderedNodes {
		ctx.NodesMap[n.ID] = n
	}

	edges := deduplicateEdges(graph.GetEdges())
	for _, e := range edges {
		ctx.EdgesMap[e.Source] = append(ctx.EdgesMap[e.Source], e)
		ctx.PrevNodesMap[e.Target] = append(ctx.PrevNodesMap[e.Target], e.Source)
	}

	return ctx, nil
}

// deduplicateEdges 对相同 (Source, Target) 的连线进行幂等清洗去重。
// 前端画图（LogicFlow / X6）偶发多次拖拽重叠连线，后端自动净化，优先保留配置了条件表达式等有效属性的连线。
func deduplicateEdges(edges []flowdsl.Edge) []flowdsl.Edge {
	if len(edges) <= 1 {
		return edges
	}

	uniqueEdges := make([]flowdsl.Edge, 0, len(edges))
	seenIndex := make(map[string]int) // "src->dst" -> uniqueEdges 中的索引

	for _, e := range edges {
		key := fmt.Sprintf("%s->%s", e.Source, e.Target)
		if idx, ok := seenIndex[key]; ok {
			// 若重复边中后出现的连线带有条件表达式而先前连线为空，则覆盖更新
			if strings.TrimSpace(uniqueEdges[idx].Expression) == "" && strings.TrimSpace(e.Expression) != "" {
				uniqueEdges[idx] = e
			}
			continue
		}
		seenIndex[key] = len(uniqueEdges)
		uniqueEdges = append(uniqueEdges, e)
	}

	return uniqueEdges
}
