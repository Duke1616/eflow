package kernel

import (
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// FindUpstreamRejectNodes 逆向寻找最近一层有效的人工审批节点集合
// 1. 穿透网关、抄送知会、群聊通知与自动化任务等瞬时穿透节点，获取上游直接关联的所有审批节点；
// 2. 结合历史执行任务，自动穿透过滤未走通或被系统跳过 (TaskSkipped) 的分支，仅回退给真实参与审批的人员；
// 3. 若上游无任何审批节点（已追溯至流程起点），返回流程开始节点 (NodeStart) 以便回退给提单人重新编辑发起。
func (p *ExecutionPlan) FindUpstreamRejectNodes(currentNodeID string, allTasks ...[]*model.Task) ([]model.NodeSpec, error) {
	inEdges := p.InEdges[currentNodeID]
	if len(inEdges) == 0 {
		return nil, fmt.Errorf("节点 [%s] 无上游前置节点", currentNodeID)
	}

	var tasks []*model.Task
	if len(allTasks) > 0 {
		tasks = allTasks[0]
	}

	// 1. 逆向 BFS 探索上游前置节点（自动穿透未通过节点与穿透路由节点）
	validApprovals, startCandidates := p.searchUpstreamCandidates(currentNodeID, tasks)
	if len(validApprovals) > 0 {
		immediate := p.filterMostImmediateNodes(lo.UniqBy(validApprovals, func(n model.NodeSpec) string { return n.ID }))
		return immediate, nil
	}

	// 2. 上游无有效前置审批节点，但能追溯到提单开始节点，回退至提单人
	if len(startCandidates) > 0 {
		return lo.UniqBy(startCandidates, func(n model.NodeSpec) string { return n.ID }), nil
	}

	return nil, fmt.Errorf("未找到有效的前置审批节点或提单开始节点")
}

// filterMostImmediateNodes 剔除所有已被其他候选节点作为上游覆盖的更远祖先节点，仅保留最紧邻当前节点的前序审批环节
func (p *ExecutionPlan) filterMostImmediateNodes(nodes []model.NodeSpec) []model.NodeSpec {
	if len(nodes) <= 1 {
		return nodes
	}
	isAncestor := make(map[string]bool)
	for _, n := range nodes {
		queue := []string{n.ID}
		visited := make(map[string]bool)
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			if visited[curr] {
				continue
			}
			visited[curr] = true
			if curr != n.ID && lo.SomeBy(nodes, func(item model.NodeSpec) bool { return item.ID == curr }) {
				isAncestor[n.ID] = true
				break
			}
			for _, edge := range p.OutEdges[curr] {
				queue = append(queue, edge.Target)
			}
		}
	}

	return lo.Filter(nodes, func(n model.NodeSpec, _ int) bool {
		return !isAncestor[n.ID]
	})
}

// FindUpstreamApprovalNode 保持兼容性包装，返回首个有效前置审批节点
func (p *ExecutionPlan) FindUpstreamApprovalNode(currentNodeID string) (model.NodeSpec, error) {
	nodes, err := p.FindUpstreamRejectNodes(currentNodeID)
	if err != nil || len(nodes) == 0 {
		return model.NodeSpec{}, err
	}
	return nodes[0], nil
}

// searchUpstreamCandidates 逆向广度优先搜索，寻找最近一层的审批候选节点与开始节点
// 若分支上的审批节点未实际审批通过（未激活、被跳过或根本未参与），自动穿透跨越继续向上寻找更上游节点
func (p *ExecutionPlan) searchUpstreamCandidates(startNodeID string, tasks []*model.Task) (approvals []model.NodeSpec, starts []model.NodeSpec) {
	queue := lo.Map(p.InEdges[startNodeID], func(e model.EdgeSpec, _ int) string { return e.Source })
	visited := make(map[string]bool)

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		if visited[currID] {
			continue
		}
		visited[currID] = true

		node, ok := p.NodesMap[currID]
		if !ok {
			continue
		}

		switch {
		case node.Type == model.NodeApproval:
			if isNodeActuallyPassed(node.ID, tasks) {
				// 节点在当前流转中实际审批通过，收集为有效驳回目标并阻断该分支继续向上探索
				approvals = append(approvals, node)
			} else {
				// 节点未走通、被跳过或未参与，自动穿透跨越，将上游入边全部入队继续寻找
				for _, edge := range p.InEdges[currID] {
					queue = append(queue, edge.Source)
				}
			}
		case node.Type == model.NodeStart:
			// 遇到流程开始节点，收集为提单起点候选
			starts = append(starts, node)
		case node.IsGateway() || node.Type == model.NodeCarbonCopy ||
			node.Type == model.NodeChatGroup || node.Type == model.NodeTask:
			// 穿透纯路由网关与瞬时穿透型节点（抄送/群聊/自动化任务），将上游前驱全部入队继续探索
			for _, edge := range p.InEdges[currID] {
				queue = append(queue, edge.Source)
			}
		}
	}
	return approvals, starts
}

// isNodeActuallyPassed 判断指定节点在当前工单的执行链路中是否真正存在已审批通过的记录 (TaskPassed 或 TaskSystemPassed)
// 1. 若该节点根本没有生成过任务（例如 XOR 条件网关未命中且未走该分支），返回 false，严禁作为驳回目标；
// 2. 若该节点在最新执行批次中全部被跳过 (TaskSkipped) 或取消 (TaskCancelled)，返回 false；
// 3. 必须在最新执行批次中至少存在一个被实际处理人同意通过或系统放行的任务，方可作为有效驳回目标。
func isNodeActuallyPassed(nodeID string, tasks []*model.Task) bool {
	if len(tasks) == 0 {
		return false
	}
	nodeTasks := lo.Filter(tasks, func(t *model.Task, _ int) bool {
		return t.NodeID == nodeID
	})
	if len(nodeTasks) == 0 {
		return false
	}

	latestTasks := model.FilterLatestBatchTasks(nodeTasks)
	if len(latestTasks) == 0 {
		return false
	}

	return lo.SomeBy(latestTasks, func(t *model.Task) bool {
		return t.IsPassed()
	})
}

// FindAllUpstreamRejectTargets 逆向遍历获取当前节点上游所有可作为自由驳回目标的节点集合（包含流程起点与历史已流转审批节点）
// 1. 递归追溯所有上游入边分支，跨越网关、抄送、通知与自动化任务；
// 2. 收集沿途所有有效审批节点与提单开始节点；
// 3. 严格结合历史任务切片，仅放行在流程流转中实际审批通过（TaskPassed）或参与过的审批节点；
// 4. 起点始终保留在候选列表中，提供“驳回至发起人”能力；
// 5. 按照因果流转拓扑顺序（靠近起点在前，靠近当前节点在后）返回，供前端选项呈现。
func (p *ExecutionPlan) FindAllUpstreamRejectTargets(currentNodeID string, allTasks ...[]*model.Task) ([]model.NodeSpec, error) {
	inEdges := p.InEdges[currentNodeID]
	if len(inEdges) == 0 {
		return nil, fmt.Errorf("节点 [%s] 无上游前置节点", currentNodeID)
	}

	var tasks []*model.Task
	if len(allTasks) > 0 {
		tasks = allTasks[0]
	}

	queue := lo.Map(inEdges, func(e model.EdgeSpec, _ int) string { return e.Source })
	visited := make(map[string]bool)
	var approvals []model.NodeSpec
	var startNodes []model.NodeSpec

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]

		if visited[currID] {
			continue
		}
		visited[currID] = true

		node, ok := p.NodesMap[currID]
		if !ok {
			continue
		}

		if node.Type == model.NodeStart {
			startNodes = append(startNodes, node)
			continue
		}

		if node.Type == model.NodeApproval {
			if isNodeActuallyPassed(node.ID, tasks) {
				approvals = append(approvals, node)
			}
		}

		for _, edge := range p.InEdges[currID] {
			queue = append(queue, edge.Source)
		}
	}

	var result []model.NodeSpec
	if len(startNodes) > 0 {
		result = append(result, lo.UniqBy(startNodes, func(n model.NodeSpec) string { return n.ID })...)
	}
	if len(approvals) > 0 {
		uniqApprovals := lo.UniqBy(approvals, func(n model.NodeSpec) string { return n.ID })
		for i := len(uniqApprovals) - 1; i >= 0; i-- {
			result = append(result, uniqApprovals[i])
		}
	}

	return result, nil
}
