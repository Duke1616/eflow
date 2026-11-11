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
	SkippedTargets []model.NodeSpec // 落选分支下需持久化 TaskSkipped 段位的物理活动节点 (排他/包容网关特有)
}

// StepDecision 步骤转移综合决策（包含汇聚屏障裁决与跨网关穿透）
type StepDecision struct {
	ReadyNodes     []model.NodeSpec // 本轮最终就绪、需派发生成待办的目标节点列表
	SkippedNodes   []model.NodeSpec // 条件未满足、需标记为 TaskSkipped 的节点列表
	SkippedTaskIDs []string         // 因排他汇聚竞争放行需更新跳过/作废的未决前置任务 ID 列表
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

	// 多层级联网关递归穿透可能对同一节点重复收集，且真实就绪节点严禁同时被标记跳过，此处统一收敛去重与互斥防卫
	decision.SkippedNodes = lo.UniqBy(decision.SkippedNodes, func(n model.NodeSpec) string { return n.ID })
	decision.SkippedNodes = lo.Filter(decision.SkippedNodes, func(n model.NodeSpec, _ int) bool {
		_, isReady := readyNodeIDs[n.ID]
		return !isReady
	})
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

// CalculateTransition 结合网关分类（或/并/包容）计算流转的目标节点与自动跳过节点 (Value Semantics 优先)
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
		Targets:        make([]model.NodeSpec, 0, len(edges)),
		SkippedTargets: make([]model.NodeSpec, 0),
	}

	// 并行网关 (AND) 无条件并发放行全部出边，不存在落选分支，无需参与跳过语义计算
	if currNode.Type == model.NodeGatewayParallel {
		for _, edge := range edges {
			targetNode, ok := p.NodesMap[edge.Target]
			if !ok {
				return TransitionResult{}, fmt.Errorf("目标节点未定义: %s", edge.Target)
			}
			result.Targets = append(result.Targets, targetNode)
		}
		return result, nil
	}

	// 全量求值所有出边而非命中即短路：排他网关的落选集合必须完整，否则无法为下游汇聚屏障补齐段位
	var matched, unmatched []model.NodeSpec
	for _, edge := range edges {
		targetNode, ok := p.NodesMap[edge.Target]
		if !ok {
			return TransitionResult{}, fmt.Errorf("目标节点未定义: %s", edge.Target)
		}
		if p.evaluateEdge(edge, variables) {
			matched = append(matched, targetNode)
		} else {
			unmatched = append(unmatched, targetNode)
		}
	}

	// 排他网关 (XOR)：顺次求值仅首条命中分支真实放行，其余命中分支同等降级为落选
	if currNode.Type == model.NodeGatewayExclusive && len(matched) > 0 {
		losers := make([]model.NodeSpec, 0, len(matched)-1+len(unmatched))
		losers = append(losers, matched[1:]...)
		losers = append(losers, unmatched...)
		matched, unmatched = matched[:1], losers
	}

	result.Targets = matched

	// 网关落选分支必须持久化 TaskSkipped 段位：落选目标可能是嵌套网关，其下游活动节点同样永不派发待办，
	// 一旦下游存在 AND/OR 汇聚屏障，缺失的段位将使汇聚永久无法满足，流程实例死锁在流转中且不产生任何报错
	if currNode.IsGateway() {
		result.SkippedTargets = p.collectUntakenActivities(unmatched, matched)
	}

	return result, nil
}

// collectUntakenActivities 沿落选分支正向穿透嵌套网关与串行链路，收集全部需要落 TaskSkipped 段位的物理活动节点
// taken 为本次真实放行路径，其可达域内的节点必然会被正常派发，遇到即停止下探，严防污染汇聚点之后的公共主干
func (p *ExecutionPlan) collectUntakenActivities(untaken, taken []model.NodeSpec) []model.NodeSpec {
	if len(untaken) == 0 {
		return nil
	}

	takenReachable := p.reachableFrom(taken)
	return p.walk(nodeIDs(untaken), true, func(n model.NodeSpec) walkAction {
		if _, ok := takenReachable[n.ID]; ok {
			return walkAction{}
		}
		// 网关属于纯拓扑控制节点，自身不落 Task 记录，但必须继续向下穿透以覆盖多层级联落选分支
		return walkAction{Collect: n.IsActivity(), Descend: true}
	})
}

// reachableFrom 沿出边正向遍历，求解给定根节点集合的完整可达域
func (p *ExecutionPlan) reachableFrom(roots []model.NodeSpec) map[string]struct{} {
	reachable := p.walk(nodeIDs(roots), true, func(model.NodeSpec) walkAction {
		return walkAction{Collect: true, Descend: true}
	})
	return lo.SliceToMap(reachable, func(n model.NodeSpec) (string, struct{}) {
		return n.ID, struct{}{}
	})
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
