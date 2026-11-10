package flowx

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// DefaultConverter flowx 流程图编译器默认实现
type DefaultConverter struct {
	handlers map[flowdsl.NodeType]INodeConverter
}

// NewConverter 创建并初始化包含所有标准节点处理器的转换器
func NewConverter() IConverter {
	c := &DefaultConverter{
		handlers: make(map[flowdsl.NodeType]INodeConverter),
	}
	c.Register(&StartNodeConverter{})
	c.Register(&EndNodeConverter{})
	c.Register(&UserNodeConverter{})
	c.Register(&ParallelGatewayConverter{})
	c.Register(&InclusionGatewayConverter{})
	c.Register(&SelectiveGatewayConverter{})
	c.Register(&ConditionGatewayConverter{})
	c.Register(&AutomationNodeConverter{})
	c.Register(&ChatGroupNodeConverter{})
	return c
}

// Register 注册或覆盖特定节点类型的转换器
func (c *DefaultConverter) Register(handler INodeConverter) {
	c.handlers[handler.Type()] = handler
}

// Convert 将标准领域拓扑图编译为 flowx 强类型流程定义快照（Version 默认为 0，交由引擎发布时分配）
func (c *DefaultConverter) Convert(procKey, procName string, graph flowdsl.IGraph) (model.ProcessDefinition, error) {
	if procKey == "" {
		return model.ProcessDefinition{}, fmt.Errorf("流程定义唯一标识 (procKey) 不能为空")
	}
	if graph == nil {
		return model.ProcessDefinition{}, fmt.Errorf("流程拓扑图 (graph) 不能为空")
	}

	// 1. 组装全局编译上下文
	ctx := &ConvertContext{
		ProcKey:  procKey,
		ProcName: procName,
		Graph:    graph,
	}

	// 3. 逐个转换节点规格（采用保持原始顺序的节点流，确保编译幂等）
	orderedNodes := graph.GetOrderedNodes()
	nodeSpecs := make([]model.NodeSpec, 0, len(orderedNodes))
	for _, node := range orderedNodes {
		handler, ok := c.handlers[node.Type]
		if !ok && node.Type == flowdsl.NodeGateway {
			// 若为未细分具体类型的网关，默认对齐条件分支网关
			handler = c.handlers[flowdsl.NodeGatewayExclusive]
			ok = handler != nil
		}
		if !ok {
			return model.ProcessDefinition{}, fmt.Errorf("不支持的画布节点类型: [%s] (node_id=%s)", node.Type, node.ID)
		}

		spec, err := handler.Convert(ctx, node)
		if err != nil {
			return model.ProcessDefinition{}, fmt.Errorf("转换节点规格失败 [%s]: %w", node.ID, err)
		}
		nodeSpecs = append(nodeSpecs, spec)
	}

	// 4. 转换连线拓扑
	edgeSpecs := ConvertEdges(graph.GetEdges())

	// 5. 产出强类型流程定义（版本默认为 0，交由引擎发布期分配）
	def := model.ProcessDefinition{
		Key:     procKey,
		Name:    procName,
		Version: 0,
		Nodes:   nodeSpecs,
		Edges:   edgeSpecs,
	}

	// 6. 拓扑合法性自卫校验 (Fail-Fast: 检查连通性、环路自环、孤立网关等)
	if err := def.Validate(); err != nil {
		return model.ProcessDefinition{}, fmt.Errorf("%w: %s", flowx.ErrInvalidDefinition, err.Error())
	}

	return def, nil
}
