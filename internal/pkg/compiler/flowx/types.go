package flowx

import (
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// 节点默认名称
const (
	DefaultNameStart     = "开始"
	DefaultNameEnd       = "结束"
	DefaultNameUser      = "人工审批"
	DefaultNameParallel  = "并行网关"
	DefaultNameInclusion = "包容网关"
	DefaultNameSelective = "排他网关"
)

// IConverter 流程画布编译器通用契约
type IConverter interface {
	// Convert 将标准领域拓扑图编译为 flowx 强类型流程定义（版本默认置零，交由部署期分配）
	Convert(procKey, procName string, graph flowdsl.IGraph) (model.ProcessDefinition, error)
}

// INodeConverter 单个标准节点类型转换器策略接口
type INodeConverter interface {
	// Type 声明支持处理的标准节点类型
	Type() flowdsl.NodeType
	// Convert 将单个标准节点转为 flowx 的 NodeSpec 规格定义
	Convert(ctx *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error)
}

// ConvertContext 画布编译全局上下文
type ConvertContext struct {
	ProcKey  string
	ProcName string
	Graph    flowdsl.IGraph
}
