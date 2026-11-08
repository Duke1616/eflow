package kernel

import (
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// FindUpstreamRejectNodes 逆向寻找最近一层有效的人工审批节点集合
// 1. 穿透网关与抄送知会节点，获取上游直接关联的所有审批节点；
// 2. 结合历史执行任务，自动过滤因条件未命中被跳过 (TaskSkipped) 的分支，仅回退给真实参与审批的人员；
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

	// 1. 逆向 BFS 探索上游前置节点
	approvalCandidates, startCandidates := p.searchUpstreamCandidates(currentNodeID)

	// 2. 过滤掉最新批次中被系统自动跳过 (TaskSkipped) 的非激活分支
	validApprovals := filterValidApprovalNodes(approvalCandidates, tasks)
	if len(validApprovals) > 0 {
		return validApprovals, nil
	}

	// 3. 上游无有效前置审批节点，但能追溯到提单开始节点，回退至提单人
	if len(startCandidates) > 0 {
		return lo.UniqBy(startCandidates, func(n model.NodeSpec) string { return n.ID }), nil
	}

	return nil, fmt.Errorf("未找到有效的前置审批节点或提单开始节点")
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
func (p *ExecutionPlan) searchUpstreamCandidates(startNodeID string) (approvals []model.NodeSpec, starts []model.NodeSpec) {
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
			// 遇到人工审批节点，收集为候选目标，阻断该分支继续向上探索
			approvals = append(approvals, node)
		case node.Type == model.NodeStart:
			// 遇到流程开始节点，收集为提单起点候选
			starts = append(starts, node)
		case node.IsGateway() || node.Type == model.NodeCarbonCopy:
			// 穿透纯路由网关与抄送知会节点，将上游前驱全部入队继续探索
			for _, edge := range p.InEdges[currID] {
				queue = append(queue, edge.Source)
			}
		}
	}
	return approvals, starts
}

// filterValidApprovalNodes 过滤掉在本次实例流转中被明确跳过 (TaskSkipped) 的无效审批节点
func filterValidApprovalNodes(candidates []model.NodeSpec, tasks []*model.Task) []model.NodeSpec {
	valid := lo.Filter(candidates, func(node model.NodeSpec, _ int) bool {
		return !isNodeSkipped(node.ID, tasks)
	})
	return lo.UniqBy(valid, func(n model.NodeSpec) string { return n.ID })
}

// isNodeSkipped 判断指定节点在当前工单的最新执行批次中是否全部被标记为跳过 (TaskSkipped)
func isNodeSkipped(nodeID string, tasks []*model.Task) bool {
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
	return len(latestTasks) > 0 && lo.EveryBy(latestTasks, func(t *model.Task) bool {
		return t.Status == model.TaskSkipped
	})
}
