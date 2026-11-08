package kernel

import (
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
	"github.com/samber/lo"
)

// TransitionResult 流程转移推导计算结果
type TransitionResult struct {
	Targets        []model.NodeSpec // 本次流转应激活的目标节点
	SkippedTargets []model.NodeSpec // 条件未命中被自动跳过的目标节点 (包容网关特有)
}

// StepDecision 步骤转移综合决策（包含汇聚屏障裁决与跨网关穿透）
type StepDecision struct {
	ReadyNodes     []model.NodeSpec // 本轮最终就绪、需派发生成待办的目标节点列表
	SkippedNodes   []model.NodeSpec // 条件未满足、需标记为 TaskSkipped 的节点列表
	SkippedTaskIDs []string         // 因排他汇聚竞争放行需联动跳过/作废的未决前置任务 ID 列表
	IsBlocked      bool             // 是否存在汇聚网关屏障未全部就绪而需保持等待
}

// ResolveNextStep 综合求解下一步流转决策：网关分流 + 自动Skip识别 + 汇聚屏障过滤 + 跨汇聚网关下游递归穿透 (支持多层级联网关)
func (p *ExecutionPlan) ResolveNextStep(currentNodeID string, variables model.Variables, allTasks []*model.Task, batchCode int) (StepDecision, error) {
	transRes, err := p.CalculateTransition(currentNodeID, variables)
	if err != nil {
		return StepDecision{}, err
	}

	decision := StepDecision{
		ReadyNodes:     make([]model.NodeSpec, 0, len(transRes.Targets)),
		SkippedNodes:   transRes.SkippedTargets,
		SkippedTaskIDs: make([]string, 0),
	}

	if len(transRes.Targets) == 0 {
		return decision, nil
	}

	queue := append([]model.NodeSpec{}, transRes.Targets...)
	visitedGateways := make(map[string]struct{})
	readyNodeIDs := make(map[string]struct{})

	for len(queue) > 0 {
		nextNode := queue[0]
		queue = queue[1:]

		// 1. 若目标是非网关节点（审批、自动化任务、结束节点），加入就绪列表（防重）
		if !nextNode.IsGateway() {
			if _, exists := readyNodeIDs[nextNode.ID]; !exists {
				readyNodeIDs[nextNode.ID] = struct{}{}
				decision.ReadyNodes = append(decision.ReadyNodes, nextNode)
			}
			continue
		}

		// 2. 若目标是网关节点，先检查是否存在多路汇聚屏障阻断
		if !p.IsJoinSatisfied(nextNode, allTasks) {
			decision.IsBlocked = true
			continue // 该网关前置尚未全部就绪，保持阻断等待
		}

		// 3. 排他汇聚网关 (NodeGatewayExclusive 且入边 > 1)：首到即放行，自动收集其他竞争失败分支上的未决待办任务
		if nextNode.Type == model.NodeGatewayExclusive && len(p.InEdges[nextNode.ID]) > 1 {
			decision.SkippedTaskIDs = append(decision.SkippedTaskIDs, p.collectExclusiveJoinSkippedTasks(nextNode, allTasks)...)
		}

		// 4. 防环检查，避免网关循环互连导致死循环
		if _, visited := visitedGateways[nextNode.ID]; visited {
			continue
		}
		visitedGateways[nextNode.ID] = struct{}{}

		// 5. 网关满足流转条件，递归穿透获取下游目标
		subRes, err := p.CalculateTransition(nextNode.ID, variables)
		if err != nil {
			return StepDecision{}, err
		}
		decision.SkippedNodes = append(decision.SkippedNodes, subRes.SkippedTargets...)
		queue = append(queue, subRes.Targets...)
	}

	decision.SkippedTaskIDs = lo.Uniq(decision.SkippedTaskIDs)
	return decision, nil
}

// collectExclusiveJoinSkippedTasks 排他网关竞争放行后，收集竞争失败分支上待取消的未决任务
func (p *ExecutionPlan) collectExclusiveJoinSkippedTasks(joinNode model.NodeSpec, allTasks []*model.Task) []string {
	branchStates := p.extractBranchStates(joinNode, allTasks)
	hasPassedBranch := lo.SomeBy(branchStates, func(b BranchState) bool { return b.IsPassed })
	if !hasPassedBranch {
		return nil
	}

	unpassedNodeIDs := lo.FilterMap(branchStates, func(b BranchState, _ int) (string, bool) {
		return b.NodeID, !b.IsPassed && !b.IsSkipped && b.HasTask
	})
	if len(unpassedNodeIDs) == 0 {
		return nil
	}

	unpassedSet := lo.SliceToMap(unpassedNodeIDs, func(id string) (string, struct{}) {
		return id, struct{}{}
	})

	var skippedIDs []string
	for _, t := range allTasks {
		if _, ok := unpassedSet[t.NodeID]; ok && t.IsPending() {
			skippedIDs = append(skippedIDs, t.ID)
		}
	}
	return skippedIDs
}

// CalculateTransition 结合网关分类（或/并/包容）计算下一阶段的目标节点与自动跳过节点 (Value Semantics 优先)
func (p *ExecutionPlan) CalculateTransition(currentNodeID string, variables model.Variables) (TransitionResult, error) {
	edges := p.OutEdges[currentNodeID]
	if len(edges) == 0 {
		return TransitionResult{}, nil
	}

	currNode, exists := p.NodesMap[currentNodeID]
	if !exists {
		return TransitionResult{}, fmt.Errorf("当前节点未定义: %s", currentNodeID)
	}

	result := TransitionResult{
		Targets:        make([]model.NodeSpec, 0),
		SkippedTargets: make([]model.NodeSpec, 0),
	}

	for _, edge := range edges {
		targetNode, ok := p.NodesMap[edge.Target]
		if !ok {
			return TransitionResult{}, fmt.Errorf("目标节点未定义: %s", edge.Target)
		}

		// 并行网关无条件并发放行；其余网关或普通节点按规则条件求值
		matched := currNode.Type == model.NodeGatewayParallel || p.evaluateEdge(edge, variables)

		if matched {
			result.Targets = append(result.Targets, targetNode)

			// 排他网关 (XOR / 或网关)：命中首个满足条件的分支后立即短路退出！
			if currNode.Type == model.NodeGatewayExclusive {
				break
			}
		} else if currNode.Type == model.NodeGatewayInclusive {
			// 包容网关 (OR / 条件并行)：未满足条件的分支自动标记为跳过 (Skip)
			result.SkippedTargets = append(result.SkippedTargets, targetNode)
		}
	}

	return result, nil
}

// evaluateEdge 优先面向标准 RuleGroup 规则组进行求值；若无 RuleGroup 但配置了文本表达式，则委托表达式引擎求值
func (p *ExecutionPlan) evaluateEdge(edge model.EdgeSpec, variables model.Variables) bool {
	if edge.RuleGroup != nil {
		return rule.EvaluateRuleGroup(edge.RuleGroup, variables)
	}
	if edge.Condition != "" {
		res, err := rule.GetDefaultEngine().Evaluate(edge.Condition, variables)
		if err != nil {
			return false
		}
		return res
	}
	return true // 无任何规则约束，默认放行
}
