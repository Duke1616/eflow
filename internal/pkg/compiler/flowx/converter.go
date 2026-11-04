package flowx

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// DefaultConverter flowx 流程图编译器默认实现
type DefaultConverter struct {
	handlers map[string]INodeConverter
}

// NewConverter 创建并初始化包含所有标准节点处理器的转换器
func NewConverter() IConverter {
	c := &DefaultConverter{
		handlers: make(map[string]INodeConverter),
	}
	c.Register(&StartNodeConverter{})
	c.Register(&EndNodeConverter{})
	c.Register(&UserNodeConverter{})
	c.Register(&ParallelGatewayConverter{})
	c.Register(&InclusionGatewayConverter{})
	c.Register(&SelectiveGatewayConverter{})
	c.Register(&ConditionGatewayConverter{})
	c.Register(&AutomationNodeConverter{})
	return c
}

// Register 注册或覆盖特定节点类型的转换器
func (c *DefaultConverter) Register(handler INodeConverter) {
	c.handlers[handler.Type()] = handler
}

// Convert 将 LogicFlow 画布拓扑数据编译为 flowx 强类型流程定义快照
func (c *DefaultConverter) Convert(procKey, procName string, version int, flow domain.LogicFlow) (model.ProcessDefinition, error) {
	if procKey == "" {
		return model.ProcessDefinition{}, fmt.Errorf("流程定义唯一标识 (procKey) 不能为空")
	}
	if version <= 0 {
		version = 1
	}

	// 1. 反序列化原始节点与连线
	rawNodes, err := ParseNodes(flow.Nodes)
	if err != nil {
		return model.ProcessDefinition{}, fmt.Errorf("解析画布节点失败: %w", err)
	}
	rawEdges, err := ParseEdges(flow.Edges)
	if err != nil {
		return model.ProcessDefinition{}, fmt.Errorf("解析画布连线失败: %w", err)
	}

	// 2. 组装全局编译上下文
	ctx := &ConvertContext{
		ProcKey:  procKey,
		ProcName: procName,
		Version:  version,
		NodesMap: make(map[string]Node, len(rawNodes)),
		OutEdges: make(map[string][]Edge),
		InEdges:  make(map[string][]Edge),
		AllNodes: rawNodes,
		AllEdges: rawEdges,
	}
	for _, n := range rawNodes {
		ctx.NodesMap[n.ID] = n
	}
	for _, e := range rawEdges {
		ctx.OutEdges[e.SourceNodeId] = append(ctx.OutEdges[e.SourceNodeId], e)
		ctx.InEdges[e.TargetNodeId] = append(ctx.InEdges[e.TargetNodeId], e)
	}

	// 3. 逐个转换节点规格
	nodeSpecs := make([]model.NodeSpec, 0, len(rawNodes))
	for _, rawNode := range rawNodes {
		handler, ok := c.handlers[rawNode.Type]
		if !ok {
			return model.ProcessDefinition{}, fmt.Errorf("不支持的画布节点类型: [%s] (node_id=%s)", rawNode.Type, rawNode.ID)
		}

		spec, err := handler.Convert(ctx, rawNode)
		if err != nil {
			return model.ProcessDefinition{}, fmt.Errorf("转换节点规格失败 [%s]: %w", rawNode.ID, err)
		}
		nodeSpecs = append(nodeSpecs, spec)
	}

	// 4. 转换连线拓扑
	edgeSpecs := ConvertEdges(rawEdges)

	// 5. 产出强类型流程定义
	def := model.ProcessDefinition{
		Key:     procKey,
		Name:    procName,
		Version: version,
		Nodes:   nodeSpecs,
		Edges:   edgeSpecs,
	}

	// 6. 拓扑合法性自卫校验 (Fail-Fast: 检查连通性、环路自环、孤立网关等)
	if err := def.Validate(); err != nil {
		return model.ProcessDefinition{}, fmt.Errorf("转换生成的流程定义拓扑校验未通过: %w", err)
	}

	return def, nil
}
