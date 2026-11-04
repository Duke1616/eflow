package kernel

import (
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
)

// ExecutionPlan 流程只读执行拓扑（纯内存索引结构，线程安全）
type ExecutionPlan struct {
	Def       model.ProcessDefinition
	StartNode model.NodeSpec
	NodesMap  map[string]model.NodeSpec
	OutEdges  map[string][]model.EdgeSpec // Source -> Edges
	InEdges   map[string][]model.EdgeSpec // Target -> Edges
}

// ValidateDefinition 静态校验流程定义的完整性与拓扑连通性（向后兼容委托给实体的 Validate 方法）
func ValidateDefinition(def model.ProcessDefinition) error {
	return def.Validate()
}

// NewExecutionPlan 编译流程定义为内存执行拓扑
func NewExecutionPlan(def model.ProcessDefinition) *ExecutionPlan {
	plan := &ExecutionPlan{
		Def:      def,
		NodesMap: make(map[string]model.NodeSpec, len(def.Nodes)),
		OutEdges: make(map[string][]model.EdgeSpec),
		InEdges:  make(map[string][]model.EdgeSpec),
	}

	for _, n := range def.Nodes {
		plan.NodesMap[n.ID] = n
		if n.Type == model.NodeStart {
			plan.StartNode = n
		}
	}
	for _, e := range def.Edges {
		// 编译期归一化：若未挂载强类型 RuleGroup 但存在简易表达式文本，自动解析为标准 ConditionGroup，保证运行时纯粹性
		if e.RuleGroup == nil && e.Condition != "" {
			e.RuleGroup = rule.ParseConditionText(e.Condition)
		}
		plan.OutEdges[e.Source] = append(plan.OutEdges[e.Source], e)
		plan.InEdges[e.Target] = append(plan.InEdges[e.Target], e)
	}
	return plan
}

// FindStartNode 获取流程开始节点
func (p *ExecutionPlan) FindStartNode() (model.NodeSpec, error) {
	if p.StartNode.ID != "" {
		return p.StartNode, nil
	}
	return model.NodeSpec{}, fmt.Errorf("流程未定义开始节点")
}
