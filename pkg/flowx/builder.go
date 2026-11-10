package flowx

import (
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
)

// ProcessBuilder 流式构造器，便于以纯 Go 代码直观定义流程
type ProcessBuilder struct {
	def       model.ProcessDefinition
	nodeIndex map[string]int // nodeID -> slice index，加速配置查找
	err       error          // 链式构建期捕获的首个语法异常 (Fail-Fast 防卫)
}

// NewProcess 创建一个流程构建器
func NewProcess(key, name string) *ProcessBuilder {
	return &ProcessBuilder{
		def: model.ProcessDefinition{
			Key:     key,
			Name:    name,
			Version: 1,
			Nodes:   make([]model.NodeSpec, 0),
			Edges:   make([]model.EdgeSpec, 0),
		},
		nodeIndex: make(map[string]int),
	}
}

// Version 设置流程定义版本号（默认版本为 1）
func (b *ProcessBuilder) Version(v int) *ProcessBuilder {
	b.def.Version = v
	return b
}

func (b *ProcessBuilder) addNode(spec model.NodeSpec) *ProcessBuilder {
	idx := len(b.def.Nodes)
	b.def.Nodes = append(b.def.Nodes, spec)
	b.nodeIndex[spec.ID] = idx
	return b
}

// Start 添加发起开始节点
func (b *ProcessBuilder) Start(id, name string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:   id,
		Name: name,
		Type: model.NodeStart,
	})
}

// Approval 添加人工审批节点
func (b *ProcessBuilder) Approval(id, name string, signRule model.SignRule, assignees ...string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:        id,
		Name:      name,
		Type:      model.NodeApproval,
		SignRule:  signRule,
		Assignees: assignees,
	})
}

// CarbonCopy 添加抄送节点 (只读知会相关人，生成抄送记录后瞬时穿透，不阻塞流程推进)
func (b *ProcessBuilder) CarbonCopy(id, name string, assignees ...string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:        id,
		Name:      name,
		Type:      model.NodeCarbonCopy,
		Assignees: assignees,
	})
}

// ChatGroup 添加群聊通知节点 (IM 群聊广播通知，瞬时穿透，不阻塞流程推进)
func (b *ProcessBuilder) ChatGroup(id, name string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:   id,
		Name: name,
		Type: model.NodeChatGroup,
	})
}

// Task 添加自动化执行节点 (如自动调用接口、执行系统任务)
func (b *ProcessBuilder) Task(id, name, workerType string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:         id,
		Name:       name,
		Type:       model.NodeTask,
		WorkerType: workerType,
	})
}

// TaskWithRetry 添加具备失败重试退避特性的自动化执行节点
func (b *ProcessBuilder) TaskWithRetry(id, name, workerType string, retry model.RetryPolicy) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:          id,
		Name:        name,
		Type:        model.NodeTask,
		WorkerType:  workerType,
		RetryPolicy: &retry,
	})
}

// ExclusiveGateway 添加排他网关节点 (XOR / 或网关：首个满足短路，仅走单条分支)
func (b *ProcessBuilder) ExclusiveGateway(id, name string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:   id,
		Name: name,
		Type: model.NodeGatewayExclusive,
	})
}

// ParallelGateway 添加并行网关节点 (AND / 并网关：分支全部并发激活，汇聚等待所有前置完成)
func (b *ProcessBuilder) ParallelGateway(id, name string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:   id,
		Name: name,
		Type: model.NodeGatewayParallel,
	})
}

// InclusiveGateway 添加包容网关节点 (OR / 条件并行网关：满足条件的全走，未满足自动 Skip)
func (b *ProcessBuilder) InclusiveGateway(id, name string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:   id,
		Name: name,
		Type: model.NodeGatewayInclusive,
	})
}

// End 添加结束办结节点
func (b *ProcessBuilder) End(id, name string) *ProcessBuilder {
	return b.addNode(model.NodeSpec{
		ID:   id,
		Name: name,
		Type: model.NodeEnd,
	})
}

// Hook 为指定节点配置前置同步守卫与动态算人 Hook
func (b *ProcessBuilder) Hook(nodeID string, hookNames ...string) *ProcessBuilder {
	if idx, ok := b.nodeIndex[nodeID]; ok {
		b.def.Nodes[idx].Hooks = append(b.def.Nodes[idx].Hooks, hookNames...)
	} else if b.err == nil {
		b.err = fmt.Errorf("节点 [%s] 未定义，无法为其绑定 Hook", nodeID)
	}
	return b
}

// NodeProps 为指定节点注入自定义扩展配置属性
func (b *ProcessBuilder) NodeProps(nodeID string, props map[string]any) *ProcessBuilder {
	if idx, ok := b.nodeIndex[nodeID]; ok {
		if b.def.Nodes[idx].Props == nil {
			b.def.Nodes[idx].Props = make(map[string]any, len(props))
		}
		for k, v := range props {
			b.def.Nodes[idx].Props[k] = v
		}
	} else if b.err == nil {
		b.err = fmt.Errorf("节点 [%s] 未定义，无法注入扩展属性", nodeID)
	}
	return b
}

// Edge 添加无条件连线
func (b *ProcessBuilder) Edge(source, target string) *ProcessBuilder {
	b.def.Edges = append(b.def.Edges, model.EdgeSpec{
		Source: source,
		Target: target,
	})
	return b
}

// ConditionEdge 添加带条件的连线（语法糖：在构建期自动将简易表达式编译为强类型 RuleGroup）
func (b *ProcessBuilder) ConditionEdge(source, target, condition string) *ProcessBuilder {
	b.def.Edges = append(b.def.Edges, model.EdgeSpec{
		Source:    source,
		Target:    target,
		Condition: condition,
		RuleGroup: rule.ParseConditionText(condition),
	})
	return b
}

// RuleEdge 添加基于强类型规则引擎的条件连线 (对齐 ealert 规则规范)
func (b *ProcessBuilder) RuleEdge(source, target string, group model.ConditionGroup) *ProcessBuilder {
	b.def.Edges = append(b.def.Edges, model.EdgeSpec{
		Source:    source,
		Target:    target,
		RuleGroup: &group,
	})
	return b
}

// Validate 静态校验当前构建中的流程定义合法性 (优先抛出链式构建错误)
func (b *ProcessBuilder) Validate() error {
	if b.err != nil {
		return b.err
	}
	return b.def.Validate()
}

// Build 产出不可变流程定义 (Fail-Fast: 若构建期存在语法或引用异常则立即中断)
func (b *ProcessBuilder) Build() model.ProcessDefinition {
	if b.err != nil {
		panic(fmt.Sprintf("流程定义构建失败: %v", b.err))
	}
	return b.def
}
