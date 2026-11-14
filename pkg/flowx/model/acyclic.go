package model

import (
	"fmt"
	"slices"
	"strings"

	"github.com/samber/lo"
)

// validateAcyclic 用 Kahn 拓扑排序拦截多节点有向环路，就地消费 Validate 已算好的 inDegree。
// 端点存在性与自环已由 Validate 前置校验，故入度不会被悬空边虚增。
func (def ProcessDefinition) validateAcyclic(inDegree map[string]int) error {
	outEdges := lo.GroupBy(def.Edges, func(e EdgeSpec) string { return e.Source })

	// 按定义顺序入队：零入度节点可能不在 inDegree 键集中，且 map 遍历会让报错路径不确定
	queue := lo.FilterMap(def.Nodes, func(n NodeSpec, _ int) (string, bool) {
		return n.ID, inDegree[n.ID] == 0
	})
	visited := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited++
		for _, e := range outEdges[curr] {
			inDegree[e.Target]--
			if inDegree[e.Target] == 0 {
				queue = append(queue, e.Target)
			}
		}
	}
	if visited == len(def.Nodes) {
		return nil
	}

	return fmt.Errorf("流程图中存在死循环环路（路径: %s），工作流必须是有向无环图（DAG），请移除回路连线",
		def.cyclePath(inDegree))
}

// cyclePath 在拓扑残留子图中提取一条具体环路，供画布定位该删哪条回路连线。
// 残留节点入度仍 > 0，意味着它至少有一条来自残留节点的前驱边，故沿前驱反向游走
// 永不断头，只可能在重复访问处收口成环——无需递归，深图也不会栈溢出。
func (def ProcessDefinition) cyclePath(inDegree map[string]int) string {
	residualEdges := lo.Filter(def.Edges, func(e EdgeSpec, _ int) bool {
		return inDegree[e.Source] > 0 && inDegree[e.Target] > 0
	})
	inEdges := lo.GroupBy(residualEdges, func(e EdgeSpec) string { return e.Target })

	first, _ := lo.Find(def.Nodes, func(n NodeSpec) bool { return inDegree[n.ID] > 0 })
	seen := make(map[string]int)
	var walk []string
	for curr := first.ID; ; curr = inEdges[curr][0].Source {
		if idx, ok := seen[curr]; ok {
			// walk 沿前驱逆向记录，就地倒序才是正向环路，再补上收口节点做到首尾同名
			cycle := walk[idx:]
			slices.Reverse(cycle)
			return def.renderNodePath(append([]string{curr}, cycle...))
		}
		seen[curr] = len(walk)
		walk = append(walk, curr)
	}
}

// renderNodePath 把节点 ID 序列渲染成画布可读的展示名路径，缺展示名时回落 ID
func (def ProcessDefinition) renderNodePath(ids []string) string {
	names := lo.SliceToMap(def.Nodes, func(n NodeSpec) (string, string) {
		return n.ID, lo.Ternary(n.Name != "", n.Name, n.ID)
	})
	return strings.Join(lo.Map(ids, func(id string, _ int) string { return names[id] }), " → ")
}
