package kernel

import (
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
)

// Edge 运行期边：条件在 plan 构建时一次性编译成树，求值不再触碰持久化 DTO。
// Rule 为 nil 表示无条件放行。
type Edge struct {
	Source string
	Target string
	Rule   rule.Condition
}

// ExecutionPlan 流程只读执行拓扑（纯内存索引结构，线程安全）
type ExecutionPlan struct {
	Def       model.ProcessDefinition
	StartNode model.NodeSpec
	NodesMap  map[string]model.NodeSpec
	OutEdges  map[string][]Edge // Source -> Edges
	InEdges   map[string][]Edge // Target -> Edges
}

// NewExecutionPlan 编译流程定义为内存执行拓扑
func NewExecutionPlan(def model.ProcessDefinition) *ExecutionPlan {
	plan := &ExecutionPlan{
		Def:      def,
		NodesMap: make(map[string]model.NodeSpec, len(def.Nodes)),
		OutEdges: make(map[string][]Edge),
		InEdges:  make(map[string][]Edge),
	}

	for _, n := range def.Nodes {
		plan.NodesMap[n.ID] = n
		if n.Type == model.NodeStart {
			plan.StartNode = n
		}
	}
	for _, e := range def.Edges {
		edge := Edge{Source: e.Source, Target: e.Target, Rule: rule.FromGroup(e.RuleGroup)}
		if edge.Rule == nil {
			// 存量兼容：早期编译产物只落了 condition 没有 rule_group，用原文补编译一次。
			// 标准模式必须原文直送 expr：走 Parse 会重新套上方言适配与宽松比较，落库往返后语义就翻了。
			if e.Standard {
				edge.Rule = rule.ParseStandard(e.Condition)
			} else {
				edge.Rule = rule.Parse(e.Condition)
			}
		}
		plan.OutEdges[e.Source] = append(plan.OutEdges[e.Source], edge)
		plan.InEdges[e.Target] = append(plan.InEdges[e.Target], edge)
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
