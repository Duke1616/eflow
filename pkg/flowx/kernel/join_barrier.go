package kernel

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

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
			states = append(states, resolveBranchState(node, nodeTasks))
		}
	}
	return states
}

// resolveBranchState 根据节点在最新执行批次 (Max BatchCode) 的真实表现推导其分支定态
func resolveBranchState(node model.NodeSpec, nodeTasks []*model.Task) BranchState {
	if len(nodeTasks) == 0 {
		return BranchState{
			NodeID:   node.ID,
			NodeName: node.Name,
			HasTask:  false,
		}
	}

	// 取该节点最新产生的最大批次号任务，避免不同分支因驳回重审导致批次号不相等而死锁
	maxBatchTasks := model.FilterLatestBatchTasks(nodeTasks)

	isPassed := IsNodeSatisfiedToAdvance(node, maxBatchTasks)
	isSkipped := !isPassed && len(maxBatchTasks) > 0 && lo.EveryBy(maxBatchTasks, func(t *model.Task) bool {
		return t.Status == model.TaskSkipped
	})

	return BranchState{
		NodeID:    node.ID,
		NodeName:  node.Name,
		HasTask:   true,
		IsPassed:  isPassed,
		IsSkipped: isSkipped,
	}
}

// FindEffectivePrecedingTaskNodes 逆向溯源指定节点的有效任务前置节点（穿透纯路由性质的网关）
func (p *ExecutionPlan) FindEffectivePrecedingTaskNodes(nodeID string) []model.NodeSpec {
	return p.walk(p.neighborIDs(nodeID, false), false, func(n model.NodeSpec) walkAction {
		switch {
		case n.IsActivity():
			return walkAction{Collect: true}
		case n.IsGateway():
			return walkAction{Descend: true}
		}
		return walkAction{}
	})
}
