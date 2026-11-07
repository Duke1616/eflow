package easyflow

import (
	"context"
	"slices"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	easyflow "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/samber/lo"
)

const (
	taskStatusPending    = 0 // 待审批
	taskStatusPassed     = 1 // 审批通过
	taskStatusRejected   = 2 // 审批驳回
	taskStatusSystemPass = 3 // 系统自动通过/取消
	taskStatusSkipped    = 5 // 分支条件不满足跳过
)

// calculateTraversedEdges 计算 easy-workflow 流程图中已经点亮流转的拓扑边映射
func calculateTraversedEdges(
	ctx context.Context,
	repo repository.IEngineRepository,
	record []model.Task,
	processInstId, processId, version int,
	status uint8,
) (map[string][]string, error) {
	define, err := repo.GetProcessDefineByVersion(ctx, processId, version)
	if err != nil {
		return nil, err
	}

	rootID := ""
	nodesMap := lo.KeyBy(define.Nodes, func(element model.Node) string {
		if element.NodeType == model.RootNode {
			rootID = element.NodeID
		}
		return element.NodeID
	})
	if rootID == "" && len(record) > 0 {
		rootID = record[0].NodeID
	}

	analyzer := newNodeStatusAnalyzer(record, nodesMap)
	topology := newGraphTopologyService(nodesMap)

	litEdges := make(map[string][]string)

	for _, task := range record {
		recursiveReset(task.NodeID, litEdges, nodesMap)

		// 1. 若当前任务不是终结/废弃状态，根据其前置节点寻路回溯，点亮到达当前节点的边
		if task.Status != taskStatusRejected && task.Status != taskStatusSystemPass && task.PrevNodeID != "" {
			if !analyzer.isBatchTainted(task.NodeID, task.BatchCode) {
				logicalPrevID := topology.resolveLogicalPrev(task.PrevNodeID)
				path := topology.findPath(logicalPrevID, task.NodeID)
				appendPathEdges(litEdges, path)
			}
		}

		// 2. 若当前任务已跳过或有效通过，向前尝试点亮后续节点（受网关汇聚阻断约束）
		shouldLightUp := task.Status == taskStatusSkipped ||
			(analyzer.isBatchEffectivelyPassed(task) && !analyzer.hasNewerBatchPending(task.NodeID, task.BatchCode))
		if shouldLightUp {
			for _, nextID := range topology.resolveNextNodes(task.NodeID) {
				lightUpForward(task.NodeID, nextID, litEdges, nodesMap, topology.effectiveGraph)
			}
		}
	}

	return filterReachableEdgesFromRoot(rootID, litEdges), nil
}

// appendPathEdges 将路径上的相邻节点转换为有向边加入边集合
func appendPathEdges(litEdges map[string][]string, path []string) {
	if len(path) <= 1 {
		return
	}
	for i := 0; i < len(path)-1; i++ {
		uniqueAppend(litEdges, path[i], path[i+1])
	}
}

// filterReachableEdgesFromRoot 从根节点进行广度优先搜索，只保留从 Root 真正可达的高亮边，过滤游离脏边
func filterReachableEdgesFromRoot(rootID string, litEdges map[string][]string) map[string][]string {
	if rootID == "" {
		return litEdges
	}

	finalEdges := make(map[string][]string)
	queue := []string{rootID}
	visitedNode := map[string]bool{rootID: true}

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		targets := litEdges[curr]
		if len(targets) == 0 {
			continue
		}

		finalEdges[curr] = targets
		for _, t := range targets {
			if !visitedNode[t] {
				visitedNode[t] = true
				queue = append(queue, t)
			}
		}
	}

	return finalEdges
}

func uniqueAppend(m map[string][]string, key, val string) {
	if !slices.Contains(m[key], val) {
		m[key] = append(m[key], val)
	}
}

func recursiveReset(nodeID string, litEdges map[string][]string, nodesMap map[string]model.Node) {
	targets := litEdges[nodeID]
	delete(litEdges, nodeID)

	for _, nextID := range targets {
		node, ok := nodesMap[nextID]
		if ok && node.NodeType == model.GateWayNode {
			recursiveReset(nextID, litEdges, nodesMap)
		}
	}
}

func lightUpForward(fromID, toID string, litEdges map[string][]string,
	nodesMap map[string]model.Node, effectiveGraph map[string][]string) {

	uniqueAppend(litEdges, fromID, toID)

	toNode, exists := nodesMap[toID]
	if !exists {
		return
	}

	if toNode.NodeType == model.GateWayNode {
		nextIDs := effectiveGraph[toID]

		inDegree := len(toNode.PrevNodeIDs)
		outDegree := len(nextIDs)
		waitType := toNode.GWConfig.WaitForAllPrevNode

		// 1. 并行汇聚网关 (waitType == 1 && inDegree > 1)：汇聚节点必须等待所有前驱分支全部完成，单分支流转不能提前穿透点亮下游
		if waitType == 1 && inDegree > 1 {
			return
		}

		// 2. 包容网关 (waitType == 0)：单分支流转不得穿透点亮
		if waitType == 0 {
			return
		}

		// 3. 条件分流网关 (waitType == 3 && outDegree > 1)：多出度条件网关必须等待下游真实命中任务通过 findPath 点亮，不能盲目向下广播
		if waitType == 3 && outDegree > 1 {
			return
		}

		for _, nextID := range nextIDs {
			lightUpForward(toID, nextID, litEdges, nodesMap, effectiveGraph)
		}
	}
}

func buildEffectiveGraph(nodesMap map[string]model.Node) map[string][]string {
	graph := make(map[string][]string)

	for id, node := range nodesMap {
		if isProxyNode(node) {
			continue
		}

		realPrevs := resolveRealPrevs(node, nodesMap)
		for _, prevID := range realPrevs {
			uniqueAppend(graph, prevID, id)
		}
	}
	return graph
}

func resolveRealPrevs(node model.Node, nodesMap map[string]model.Node) []string {
	var result []string
	for _, prevID := range node.PrevNodeIDs {
		prevNode, exists := nodesMap[prevID]
		if !exists {
			continue
		}

		if isProxyNode(prevNode) {
			result = append(result, resolveRealPrevs(prevNode, nodesMap)...)
		} else {
			result = append(result, prevID)
		}
	}
	return result
}

func isProxyNode(node model.Node) bool {
	return slices.Contains(node.UserIDs, easyflow.SysAutoUser)
}

type batchStats struct {
	Total      int
	Pending    int
	Passed     int
	SystemPass int
}

type nodeStatusAnalyzer struct {
	stats    map[string]map[string]*batchStats
	nodesMap map[string]model.Node
}

func newNodeStatusAnalyzer(record []model.Task, nodesMap map[string]model.Node) *nodeStatusAnalyzer {
	stats := make(map[string]map[string]*batchStats)
	for _, t := range record {
		if _, ok := stats[t.NodeID]; !ok {
			stats[t.NodeID] = make(map[string]*batchStats)
		}

		bStats, ok := stats[t.NodeID][t.BatchCode]
		if !ok {
			bStats = &batchStats{}
			stats[t.NodeID][t.BatchCode] = bStats
		}

		bStats.Total++
		if t.IsFinished == 0 {
			bStats.Pending++
		} else if t.Status == 1 {
			bStats.Passed++
		} else if t.Status == 3 {
			bStats.SystemPass++
		}
	}

	return &nodeStatusAnalyzer{
		stats:    stats,
		nodesMap: nodesMap,
	}
}

func (a *nodeStatusAnalyzer) isBatchTainted(nodeID, batchCode string) bool {
	if stats := a.stats[nodeID][batchCode]; stats != nil {
		return stats.SystemPass > 0
	}
	return false
}

func (a *nodeStatusAnalyzer) isBatchEffectivelyPassed(task model.Task) bool {
	if task.IsFinished != 1 || task.Status != 1 {
		return false
	}

	isCosigned := task.IsCosigned == 1 || (a.nodesMap[task.NodeID].IsCosigned == 1)
	if isCosigned {
		if stats := a.stats[task.NodeID][task.BatchCode]; stats != nil && stats.Passed < stats.Total {
			return false
		}
	}

	return true
}

func (a *nodeStatusAnalyzer) hasNewerBatchPending(nodeID, currentBatchCode string) bool {
	batches, ok := a.stats[nodeID]
	if !ok {
		return false
	}

	return lo.SomeBy(lo.Entries(batches), func(entry lo.Entry[string, *batchStats]) bool {
		return entry.Key != currentBatchCode && entry.Value != nil && entry.Value.Pending > 0
	})
}

type graphTopologyService struct {
	nodesMap       map[string]model.Node
	effectiveGraph map[string][]string
}

func newGraphTopologyService(nodesMap map[string]model.Node) *graphTopologyService {
	return &graphTopologyService{
		nodesMap:       nodesMap,
		effectiveGraph: buildEffectiveGraph(nodesMap),
	}
}

func (g *graphTopologyService) resolveLogicalPrev(rawPrevID string) string {
	node, ok := g.nodesMap[rawPrevID]
	if !ok {
		return rawPrevID
	}

	if g.isProxyNode(node) {
		if len(node.PrevNodeIDs) > 0 {
			return g.resolveLogicalPrev(node.PrevNodeIDs[0])
		}
	}
	return rawPrevID
}

func (g *graphTopologyService) isProxyNode(node model.Node) bool {
	return isProxyNode(node)
}

func (g *graphTopologyService) findPath(startID, endID string) []string {
	type path struct {
		nodes []string
	}

	queue := []path{{nodes: []string{startID}}}
	visited := map[string]bool{startID: true}

	for len(queue) > 0 {
		currPath := queue[0]
		queue = queue[1:]

		currNodeID := currPath.nodes[len(currPath.nodes)-1]
		if currNodeID == endID {
			return currPath.nodes
		}

		if len(currPath.nodes) > 20 {
			continue
		}

		for _, nextID := range g.effectiveGraph[currNodeID] {
			nextNode, exists := g.nodesMap[nextID]
			if !exists {
				continue
			}

			isTarget := nextID == endID
			isGateway := nextNode.NodeType == model.GateWayNode

			if (isTarget || isGateway) && !visited[nextID] {
				visited[nextID] = true
				newNodes := make([]string, len(currPath.nodes)+1)
				copy(newNodes, currPath.nodes)
				newNodes[len(currPath.nodes)] = nextID

				if isTarget {
					return newNodes
				}
				queue = append(queue, path{nodes: newNodes})
			}
		}
	}

	return nil
}

func (g *graphTopologyService) resolveNextNodes(nodeID string) []string {
	return g.effectiveGraph[nodeID]
}
