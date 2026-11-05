package easyflow

import (
	"fmt"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
)

// IConverter easy-workflow 流程画布编译器通用契约
type IConverter interface {
	// Convert 将标准领域拓扑图编译为 easy-workflow 流程定义模型
	Convert(procName string, graph flowdsl.IGraph) (*model.Process, error)
}

// Converter 兼容旧类型别名
type Converter = IConverter

// INodeHandler 节点解析器策略接口
type INodeHandler interface {
	// Type 声明支持处理的标准节点类型
	Type() flowdsl.NodeType
	// Handle 将单个领域节点转为 easy-workflow 引擎节点规格
	Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error)
}

// Context 转换过程的全局拓扑上下文
type Context struct {
	ProcName     string
	Graph        flowdsl.IGraph
	NodesMap     map[string]*flowdsl.Node
	EdgesMap     map[string][]flowdsl.Edge // Source -> []Edge
	PrevNodesMap map[string][]string       // Target -> []Source
	OutputNodes  []model.Node              // 转换生成的最终节点集
	ProxyNodes   map[string]struct{}       // 已创建的代理节点 ID 集合，防止重复
}

// GetPrevNodeIDs 获取目标节点的上级节点 ID
func (ctx *Context) GetPrevNodeIDs(nodeID string) []string {
	return ctx.PrevNodesMap[nodeID]
}

// GetTargetEdges 获取指定节点的后置出边
func (ctx *Context) GetTargetEdges(nodeID string) []flowdsl.Edge {
	return ctx.EdgesMap[nodeID]
}

// GetNodeInfo 获取原始领域节点信息
func (ctx *Context) GetNodeInfo(nodeID string) *flowdsl.Node {
	return ctx.NodesMap[nodeID]
}

// AddOutputNode 添加生成后的 Engine 节点
func (ctx *Context) AddOutputNode(n ...model.Node) {
	ctx.OutputNodes = append(ctx.OutputNodes, n...)
}

// GetOrGenerateProxyID 获取或生成代理节点 ID
// 当目标节点是网关类型时，创建并返回代理节点 ID，否则返回目标节点 ID
func (ctx *Context) GetOrGenerateProxyID(srcID, targetID string) string {
	targetNode := ctx.GetNodeInfo(targetID)
	if targetNode == nil {
		return targetID
	}

	// 只有目标是特定网关类型时才需要代理
	if targetNode.Type == NodeTypeParallel || targetNode.Type == NodeTypeInclusion || targetNode.Type == NodeTypeSelective {
		proxyID := fmt.Sprintf("proxy_%s_%s", srcID, targetID)
		if ctx.ProxyNodes == nil {
			ctx.ProxyNodes = make(map[string]struct{})
		}
		if _, ok := ctx.ProxyNodes[proxyID]; !ok {
			ctx.createProxyNode(proxyID, srcID, targetID)
		}
		return proxyID
	}
	return targetID
}

// GetOrGenerateProxyForGateway 当源节点是条件网关连接到并行/包容/条件并行网关时，获取或生成代理节点 ID
func (ctx *Context) GetOrGenerateProxyForGateway(conditionGatewayID, targetID string) string {
	conditionNode := ctx.GetNodeInfo(conditionGatewayID)
	if conditionNode == nil {
		return conditionGatewayID
	}

	// 只有源节点是条件网关时才需要代理
	if conditionNode.Type == NodeTypeCondition {
		proxyID := fmt.Sprintf("proxy_%s_%s", conditionGatewayID, targetID)
		if ctx.ProxyNodes == nil {
			ctx.ProxyNodes = make(map[string]struct{})
		}
		if _, ok := ctx.ProxyNodes[proxyID]; !ok {
			ctx.createProxyNode(proxyID, conditionGatewayID, targetID)
		}
		return proxyID
	}
	return conditionGatewayID
}

func (ctx *Context) createProxyNode(proxyID, srcID, targetID string) {
	if ctx.ProxyNodes == nil {
		ctx.ProxyNodes = make(map[string]struct{})
	}
	ctx.ProxyNodes[proxyID] = struct{}{}

	targetNode := ctx.GetNodeInfo(targetID)
	var passEvents []string
	if targetNode != nil {
		switch targetNode.Type {
		case NodeTypeParallel:
			passEvents = []string{EventTaskParallelNodePass}
		case NodeTypeInclusion:
			passEvents = []string{EventTaskInclusionNodePass}
		case NodeTypeSelective:
			passEvents = []string{EventTaskSelectiveNodePass}
		}
	}

	n := model.Node{
		NodeID:           proxyID,
		NodeName:         SysProxyNodeName,
		NodeType:         model.TaskNode,
		PrevNodeIDs:      []string{srcID},
		UserIDs:          []string{SysAutoUser},
		NodeStartEvents:  []string{EventNotify},
		NodeEndEvents:    []string{},
		TaskFinishEvents: passEvents,
	}
	ctx.AddOutputNode(n)
}
