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
	ReadyNodes   []model.NodeSpec // 本轮最终就绪、需派发生成待办的目标节点列表
	SkippedNodes []model.NodeSpec // 条件未满足、需标记为 TaskSkipped 的节点列表
	IsBlocked    bool             // 是否存在汇聚网关屏障未全部就绪而需保持等待
}

// ResolveNextStep 综合求解下一步流转决策：网关分流 + 自动Skip识别 + 汇聚屏障过滤 + 跨汇聚网关下游递归穿透 (支持多层级联网关)
func (p *ExecutionPlan) ResolveNextStep(currentNodeID string, variables model.Variables, allTasks []*model.Task, batchCode int) (StepDecision, error) {
	transRes, err := p.CalculateTransition(currentNodeID, variables)
	if err != nil {
		return StepDecision{}, err
	}

	decision := StepDecision{
		ReadyNodes:   make([]model.NodeSpec, 0, len(transRes.Targets)),
		SkippedNodes: transRes.SkippedTargets,
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

		// 3. 防环检查，避免网关循环互连导致死循环
		if _, visited := visitedGateways[nextNode.ID]; visited {
			continue
		}
		visitedGateways[nextNode.ID] = struct{}{}

		// 4. 网关满足流转条件，递归穿透获取下游目标
		subRes, err := p.CalculateTransition(nextNode.ID, variables)
		if err != nil {
			return StepDecision{}, err
		}
		decision.SkippedNodes = append(decision.SkippedNodes, subRes.SkippedTargets...)
		queue = append(queue, subRes.Targets...)
	}

	return decision, nil
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

// FindEffectivePrecedingTaskNodes 逆向递归溯源指定节点的有效任务前置节点（穿透纯路由性质的网关）
func (p *ExecutionPlan) FindEffectivePrecedingTaskNodes(nodeID string) []model.NodeSpec {
	inEdges := p.InEdges[nodeID]
	if len(inEdges) == 0 {
		return nil
	}

	res := lo.FlatMap(inEdges, func(edge model.EdgeSpec, _ int) []model.NodeSpec {
		src, ok := p.NodesMap[edge.Source]
		if !ok {
			return nil
		}
		if src.IsActivity() {
			return []model.NodeSpec{src}
		}
		if src.IsGateway() {
			return p.FindEffectivePrecedingTaskNodes(src.ID)
		}
		return nil
	})

	return lo.UniqBy(res, func(n model.NodeSpec) string { return n.ID })
}

// extractBranchStates 逆向萃取汇聚网关所有入边分支的物理任务流转定态快照
func (p *ExecutionPlan) extractBranchStates(joinNode model.NodeSpec, allTasks []*model.Task) []BranchState {
	inEdges := p.InEdges[joinNode.ID]
	if len(inEdges) == 0 {
		return nil
	}

	var states []BranchState
	for _, edge := range inEdges {
		srcNode, ok := p.NodesMap[edge.Source]
		if !ok {
			continue
		}

		var checkNodes []model.NodeSpec
		if srcNode.IsActivity() {
			checkNodes = []model.NodeSpec{srcNode}
		} else if srcNode.IsGateway() {
			checkNodes = p.FindEffectivePrecedingTaskNodes(srcNode.ID)
		}

		for _, node := range checkNodes {
			nodeTasks := lo.Filter(allTasks, func(t *model.Task, _ int) bool {
				return t.NodeID == node.ID
			})
			if len(nodeTasks) == 0 {
				states = append(states, BranchState{
					NodeID:   node.ID,
					NodeName: node.Name,
					HasTask:  false,
				})
				continue
			}

			// 核心防卫：取该节点最新产生的最大批次号任务进行流转状态裁决，避免不同分支因驳回重审导致批次号不相等而死锁
			maxBatch := lo.MaxBy(nodeTasks, func(a, b *model.Task) bool {
				return a.BatchCode > b.BatchCode
			}).BatchCode

			srcTasks := lo.Filter(nodeTasks, func(t *model.Task, _ int) bool {
				return t.BatchCode == maxBatch
			})

			isPassed := IsNodeSatisfiedToAdvance(node, srcTasks)
			isSkipped := lo.SomeBy(srcTasks, func(t *model.Task) bool {
				return t.Status == model.TaskSkipped
			})

			states = append(states, BranchState{
				NodeID:    node.ID,
				NodeName:  node.Name,
				HasTask:   true,
				IsPassed:  isPassed,
				IsSkipped: isSkipped,
			})
		}
	}
	return states
}

// IsJoinSatisfied 裁决汇聚网关（Join）的上游前置分支是否全部就绪
// 支持递归穿透非任务型网关，并委托给强内聚的 IJoinStrategy 策略进行多路汇聚放行判定
func (p *ExecutionPlan) IsJoinSatisfied(joinNode model.NodeSpec, allTasks []*model.Task) bool {
	// 只有显式声明为并行 (PARALLEL) 或包容 (INCLUSIVE) 的网关，才执行多分支同步汇聚屏障
	// 排他网关 (EXCLUSIVE) 或普通节点直接放行 (首到即走)
	if joinNode.Type != model.NodeGatewayParallel && joinNode.Type != model.NodeGatewayInclusive {
		return true
	}

	inEdges := p.InEdges[joinNode.ID]
	if len(inEdges) <= 1 {
		return true // 非多路汇聚点，直接放行
	}

	strategy := GetJoinStrategy(joinNode.Type)
	branchStates := p.extractBranchStates(joinNode, allTasks)
	return strategy.IsSatisfied(branchStates)
}

// FindUpstreamApprovalNode 逆向查找上一个可驳回的人工审批节点
func (p *ExecutionPlan) FindUpstreamApprovalNode(currentNodeID string) (model.NodeSpec, error) {
	inEdges := p.InEdges[currentNodeID]
	if len(inEdges) == 0 {
		return model.NodeSpec{}, fmt.Errorf("节点 [%s] 无上游前置节点", currentNodeID)
	}

	// 向上回溯
	for _, edge := range inEdges {
		prevNode, ok := p.NodesMap[edge.Source]
		if !ok {
			continue
		}
		if prevNode.Type == model.NodeApproval {
			return prevNode, nil
		}
		if prevNode.IsGateway() {
			return p.FindUpstreamApprovalNode(prevNode.ID)
		}
	}
	return model.NodeSpec{}, fmt.Errorf("未找到有效的前置审批节点")
}
