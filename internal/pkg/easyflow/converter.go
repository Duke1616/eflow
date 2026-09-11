package easyflow

import (
	"fmt"
	"strings"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/mitchellh/mapstructure"
)

// DefaultConverter 默认转换器实现
type DefaultConverter struct {
	handlers map[string]INodeHandler
}

func NewDefaultConverter() *DefaultConverter {
	return &DefaultConverter{
		handlers: make(map[string]INodeHandler),
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
func (c *DefaultConverter) Convert(wf Workflow) (*model.Process, error) {
	// 注入 context
	ctx, err := c.initContext(wf)
	if err != nil {
		return nil, err
	}

	// 拓扑合法性校验：在节点转换前先验证流程图结构，
	// 将不合理的设计在发布阶段拦截，避免运行时死锁或异常
	if err = ValidateAll(ctx); err != nil {
		return nil, fmt.Errorf("流程图校验失败: %w", err)
	}

	// 数据转换
	nodes, err := ParseNodes(wf.FlowData.Nodes)
	if err != nil {
		return nil, fmt.Errorf("parse nodes failed: %w", err)
	}

	for _, node := range nodes {
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

	// 此处可以扩展如 GraphRewriter, EventInjector 等
	// 目前逻辑简单，直接组装结果

	process := &model.Process{
		ProcessName:  wf.Name,
		Source:       "工单系统",
		RevokeEvents: []string{EventRevoke},
		Nodes:        ctx.OutputNodes,
	}

	return process, nil
}

func (c *DefaultConverter) initContext(wf Workflow) (*Context, error) {
	ctx := &Context{
		Workflow:     wf,
		NodesMap:     make(map[string]Node),
		EdgesMap:     make(map[string][]Edge),
		PrevNodesMap: make(map[string][]string),
		OutputNodes:  []model.Node{},
	}

	edges, err := parseEdges(wf.FlowData.Edges)
	if err != nil {
		return nil, fmt.Errorf("parse edges failed: %w", err)
	}

	// 针对前端 LogicFlow 连线拖拽偶发的完全重叠连线（同源、同目标）做幂等清洗去重。
	// 若存在属性差异（如某一连线配置了条件表达式），优先保留含有有效配置的连线，提升容错性。
	edges = deduplicateEdges(edges)

	nodes, err := ParseNodes(wf.FlowData.Nodes)
	if err != nil {
		return nil, fmt.Errorf("parse nodes failed: %w", err)
	}

	for _, n := range nodes {
		ctx.NodesMap[n.ID] = n
	}

	for _, e := range edges {
		ctx.EdgesMap[e.SourceNodeId] = append(ctx.EdgesMap[e.SourceNodeId], e)
		ctx.PrevNodesMap[e.TargetNodeId] = append(ctx.PrevNodesMap[e.TargetNodeId], e.SourceNodeId)
	}

	return ctx, nil
}

// deduplicateEdges 对相同 (SourceNodeId, TargetNodeId) 的连线进行幂等清洗去重。
// 前端画图（LogicFlow）偶发多次拖拽重叠连线，后端自动净化，优先保留配置了条件表达式等有效属性的连线。
func deduplicateEdges(edges []Edge) []Edge {
	if len(edges) <= 1 {
		return edges
	}

	uniqueEdges := make([]Edge, 0, len(edges))
	seenIndex := make(map[string]int) // "src->dst" -> uniqueEdges 中的索引

	for _, e := range edges {
		key := fmt.Sprintf("%s->%s", e.SourceNodeId, e.TargetNodeId)
		if idx, ok := seenIndex[key]; ok {
			// 若重复边中后出现的连线带有条件表达式而先前连线为空，则覆盖更新
			oldProp, _ := ToEdgeProperty(uniqueEdges[idx])
			newProp, _ := ToEdgeProperty(e)
			if strings.TrimSpace(oldProp.Expression) == "" && strings.TrimSpace(newProp.Expression) != "" {
				uniqueEdges[idx] = e
			}
			continue
		}
		seenIndex[key] = len(uniqueEdges)
		uniqueEdges = append(uniqueEdges, e)
	}

	return uniqueEdges
}

// parseEdges 定义线字段 (搬运自 convert.go)
func parseEdges(raw any) ([]Edge, error) {
	var edges []Edge
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:  &edges,
		TagName: "json",
	})
	if err != nil {
		return nil, err
	}

	if err = decoder.Decode(raw); err != nil {
		return nil, err
	}

	return edges, nil
}
