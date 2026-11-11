package kernel

import (
	"slices"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// CalculateTraversedEdges 根据流程拓扑与实际执行任务快照投影计算已流转通过的高亮边集合
// 采用自顶向下的清晰流水线设计，消灭深层嵌套与臃肿闭包
func (p *ExecutionPlan) CalculateTraversedEdges(tasks []*model.Task, inst *model.Instance) map[string][]string {
	if len(p.Def.Nodes) == 0 || len(p.Def.Edges) == 0 {
		return make(map[string][]string)
	}

	tracer := newExecutionTracer(p)
	tracer.markTerminalNodes(inst)
	tracer.markTaskNodes(tasks)
	tracer.resolveThroughGateways()
	tracer.resolveWaitingGateways()

	return tracer.collectTraversedEdges()
}

// executionTracer 负责拓扑执行状态推导与有向边投影分析
type executionTracer struct {
	plan      *ExecutionPlan
	activated map[string]bool // 已到达/已激活的节点 ID 集合
	completed map[string]bool // 已完成流转推进的节点 ID 集合
}

func newExecutionTracer(p *ExecutionPlan) *executionTracer {
	t := &executionTracer{
		plan:      p,
		activated: make(map[string]bool),
		completed: make(map[string]bool),
	}
	// 开始节点默认已激活；若扫描到开始节点存在待办任务（驳回至起点修改），将重置为未完成
	if p.StartNode.ID != "" {
		t.activated[p.StartNode.ID] = true
		t.completed[p.StartNode.ID] = true
	}
	return t
}

// markTerminalNodes 实例正常圆满办结时，标记结束节点为已到达
func (t *executionTracer) markTerminalNodes(inst *model.Instance) {
	if inst == nil || inst.Status != model.InstanceFinished {
		return
	}
	for _, n := range t.plan.Def.Nodes {
		if n.Type == model.NodeEnd {
			t.activated[n.ID] = true
		}
	}
}

// markTaskNodes 遍历物理任务切片，依据每个节点最新执行批次 (Max BatchCode) 的真实定态标记到达与完成
func (t *executionTracer) markTaskNodes(tasks []*model.Task) {
	tasksByNode := lo.GroupBy(tasks, func(tk *model.Task) string { return tk.NodeID })

	// 若开始节点存在最新批次的待办任务（被驳回到起点重新填报），则开始节点尚未完成
	if startTasks, ok := tasksByNode[t.plan.StartNode.ID]; ok {
		latestStartTasks := model.FilterLatestBatchTasks(startTasks)
		if lo.SomeBy(latestStartTasks, func(tk *model.Task) bool { return tk.IsPending() }) {
			t.completed[t.plan.StartNode.ID] = false
		}
	}

	for nodeID, nodeTasks := range tasksByNode {
		if nodeID == t.plan.StartNode.ID {
			continue
		}
		latestTasks := model.FilterLatestBatchTasks(nodeTasks)

		// 若最新批次全为系统跳过任务，标记为已到达且已办结（供跳过分支虚线投影感知）
		isAllSkipped := len(latestTasks) > 0 && lo.EveryBy(latestTasks, func(tk *model.Task) bool { return tk.Status == model.TaskSkipped })
		if isAllSkipped {
			t.activated[nodeID] = true
			t.completed[nodeID] = true
			continue
		}

		t.activated[nodeID] = true
		if lo.SomeBy(latestTasks, func(tk *model.Task) bool { return tk.IsPassed() }) {
			t.completed[nodeID] = true
		}
	}
}

// resolveThroughGateways 识别已完整穿透流转的网关 (上游存在已完成分支 且 下游已激活推进)
func (t *executionTracer) resolveThroughGateways() {
	for _, n := range t.plan.Def.Nodes {
		if !n.IsGateway() {
			continue
		}
		upCompleted := t.canReach(n.ID, false, func(id string) bool { return t.completed[id] }, make(map[string]bool))
		downActivated := t.canReach(n.ID, true, func(id string) bool { return t.activated[id] }, make(map[string]bool))
		if upCompleted && downActivated {
			t.completed[n.ID] = true
			t.activated[n.ID] = true
		}
	}
}

// resolveWaitingGateways 识别处于汇聚等待态的网关 (入边上游分支已到达，但下游因多分支未完全齐备仍在等待)
// NOTE: 保证单分支审批通过时，流向汇聚网关的入边能够准确点亮至网关入口，而网关出边保持严格阻断
func (t *executionTracer) resolveWaitingGateways() {
	for _, n := range t.plan.Def.Nodes {
		if !n.IsGateway() || t.activated[n.ID] {
			continue
		}

		// 若存在已完成的前驱分支（排除排他网关的未选分支），说明流转确已流经到达该网关
		hasCompletedInflow := lo.SomeBy(t.plan.InEdges[n.ID], func(edge model.EdgeSpec) bool {
			if !t.completed[edge.Source] {
				return false
			}
			srcNode, ok := t.plan.NodesMap[edge.Source]
			return ok && (!srcNode.IsGateway() || srcNode.Type != model.NodeGatewayExclusive)
		})

		if hasCompletedInflow {
			t.activated[n.ID] = true
		}
	}
}

// canReach 沿指定方向（isForward: true 为顺流向，false 为逆流向）穿透网关寻找命中 matcher 判定条件的业务节点
func (t *executionTracer) canReach(currID string, isForward bool, matcher func(string) bool, visited map[string]bool) bool {
	edges := lo.Ternary(isForward, t.plan.OutEdges[currID], t.plan.InEdges[currID])
	for _, edge := range edges {
		nextID := lo.Ternary(isForward, edge.Target, edge.Source)
		if visited[nextID] {
			continue
		}
		visited[nextID] = true

		nextNode, exists := t.plan.NodesMap[nextID]
		if !exists {
			continue
		}

		if !nextNode.IsGateway() {
			if matcher(nextID) {
				return true
			}
		} else if t.canReach(nextID, isForward, matcher, visited) {
			return true
		}
	}
	return false
}

// collectTraversedEdges 从开始节点顺流推进，严格按拓扑连通性收集当前生命周期的已流转有向边
func (t *executionTracer) collectTraversedEdges() map[string][]string {
	traversed := make(map[string][]string)
	startID := t.plan.StartNode.ID
	if startID == "" || !t.completed[startID] {
		// 若开始节点未就绪或当前正处于驳回重报待办状态，后续流程未顺流流转，返回空边集
		return traversed
	}

	queue := []string{startID}
	visited := map[string]bool{startID: true}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, edge := range t.plan.OutEdges[curr] {
			target := edge.Target
			// 仅当目标节点确实已被当前流转激活才纳入
			if !t.activated[target] {
				continue
			}

			if !slices.Contains(traversed[curr], target) {
				traversed[curr] = append(traversed[curr], target)
			}

			// 目标节点已完成流转且未访问，继续向下推导；若为待办或驳回终止，出边严格阻断
			if t.completed[target] && !visited[target] {
				visited[target] = true
				queue = append(queue, target)
			}
		}
	}

	return traversed
}
