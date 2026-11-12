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

	tracer := newExecutionTracer(p, tasks)
	tracer.markTerminalNodes(inst)
	tracer.markTaskNodes(tasks)
	tracer.resolveThroughGateways()
	tracer.resolveWaitingGateways()

	return tracer.collectTraversedEdges()
}

// executionTracer 负责拓扑执行状态推导与有向边投影分析
type executionTracer struct {
	plan      *ExecutionPlan
	tasks     []*model.Task   // 全量历史任务快照，供汇聚屏障按各节点最新批次重新裁决
	activated map[string]bool // 已到达/已激活的节点 ID 集合
	completed map[string]bool // 已完成流转推进的节点 ID 集合
}

func newExecutionTracer(p *ExecutionPlan, tasks []*model.Task) *executionTracer {
	t := &executionTracer{
		plan:      p,
		tasks:     tasks,
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

// resolveThroughGateways 识别已完整穿透流转的网关 (上游存在已完成分支 且 下游已激活推进 且 汇聚屏障真实开闸)
// 前后两个存在性判定跑在历史全量并集上，驳回回退后会被"从未执行的跳过分支"或"停留在旧批次的兄弟通过"喂通，
// 使已退出的下游路径继续点亮，故必须叠加按各节点最新批次裁决的真实屏障
func (t *executionTracer) resolveThroughGateways() {
	for _, n := range t.plan.Def.Nodes {
		if !n.IsGateway() {
			continue
		}
		upCompleted := t.canReach(n.ID, false, func(id string) bool { return t.completed[id] })
		downActivated := t.canReach(n.ID, true, func(id string) bool { return t.activated[id] })
		if upCompleted && downActivated && t.plan.IsJoinSatisfied(n, t.tasks) {
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
		hasCompletedInflow := lo.SomeBy(t.plan.InEdges[n.ID], func(edge Edge) bool {
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
func (t *executionTracer) canReach(currID string, isForward bool, matcher func(string) bool) bool {
	found := t.plan.walk(t.plan.neighborIDs(currID, isForward), isForward, func(n model.NodeSpec) walkAction {
		if n.IsGateway() {
			return walkAction{Descend: true}
		}
		return walkAction{Collect: matcher(n.ID)}
	})
	return len(found) > 0
}

// collectTraversedEdges 严格按拓扑连通性收集当前生命周期的已流转有向边
func (t *executionTracer) collectTraversedEdges() map[string][]string {
	traversed := make(map[string][]string)
	startID := t.plan.StartNode.ID
	if startID == "" || !t.completed[startID] {
		// 若开始节点未就绪或当前正处于驳回重报待办状态，后续流程未顺流流转，返回空边集
		return traversed
	}

	// 先求出自起点顺流可达且已完成流转的节点域：只有这些节点的出边才可能真实流转过。
	// 待办或驳回终止的节点不再向下推导，其出边严格阻断
	advanced := t.plan.walk([]string{startID}, true, func(n model.NodeSpec) walkAction {
		done := t.activated[n.ID] && t.completed[n.ID]
		return walkAction{Collect: done, Descend: done}
	})

	for _, n := range advanced {
		for _, edge := range t.plan.OutEdges[n.ID] {
			// 仅当目标节点确实已被当前流转激活才纳入
			if !t.activated[edge.Target] {
				continue
			}
			if !slices.Contains(traversed[n.ID], edge.Target) {
				traversed[n.ID] = append(traversed[n.ID], edge.Target)
			}
		}
	}

	return traversed
}
