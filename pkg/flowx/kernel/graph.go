package kernel

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// walkAction 遍历命中单个节点时的处置指令。
// Collect 与 Descend 刻意拆成两个独立维度：驳回目标求解需要"命中即停"，
// 自由驳回目标求解需要"命中后继续向上穿透"，二者共用同一套遍历才能避免各写一遍队列与 visited。
type walkAction struct {
	Collect bool // 是否将该节点纳入结果
	Descend bool // 是否继续访问其邻接节点
}

// walk 以 roots 为起点按指定方向广度遍历拓扑，返回按访问顺序收集的节点。
// visited 由原语内部统一维护，因此对含环定义天然安全——发布期 Validate 虽已拦下多节点环路，
// 存量库仍可能留有修复前落库的环定义，若把防环责任留给各调用方，任何一处遗漏都会栈溢出并击穿整个进程。
// 注意本原语只做可达性，环会被 visited 静默吞掉，故不能用它判环（判环须靠入度归零，见 model.Validate）。
func (p *ExecutionPlan) walk(roots []string, forward bool, visit func(model.NodeSpec) walkAction) []model.NodeSpec {
	var collected []model.NodeSpec
	visited := make(map[string]struct{}, len(p.NodesMap))
	queue := make([]string, 0, len(roots))

	// 入队即标记：同一节点最多访问一次，且首次入队顺序就是广度优先的访问顺序
	enqueue := func(ids ...string) {
		for _, id := range ids {
			if _, seen := visited[id]; seen {
				continue
			}
			visited[id] = struct{}{}
			queue = append(queue, id)
		}
	}
	enqueue(roots...)

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		node, ok := p.NodesMap[curr]
		if !ok {
			continue
		}

		action := visit(node)
		if action.Collect {
			collected = append(collected, node)
		}
		if !action.Descend {
			continue
		}
		enqueue(p.neighborIDs(curr, forward)...)
	}

	return collected
}

// adjacent 取指定方向的邻接边：forward 为顺流出边，否则为逆流入了边
func (p *ExecutionPlan) adjacent(nodeID string, forward bool) []Edge {
	if forward {
		return p.OutEdges[nodeID]
	}
	return p.InEdges[nodeID]
}

// neighborIDs 取指定方向的邻接节点 ID，用于以"前驱/后继"而非自身作为遍历起点
func (p *ExecutionPlan) neighborIDs(nodeID string, forward bool) []string {
	return lo.Map(p.adjacent(nodeID, forward), func(e Edge, _ int) string {
		if forward {
			return e.Target
		}
		return e.Source
	})
}

func nodeIDs(nodes []model.NodeSpec) []string {
	return lo.Map(nodes, func(n model.NodeSpec, _ int) string { return n.ID })
}
