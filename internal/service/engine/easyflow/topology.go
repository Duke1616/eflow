package easyflow

import (
	"context"
	"slices"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	easyflow "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/samber/lo"
)

// calculateTraversedEdges 计算 easy-workflow 流程图中已经点亮流转的拓扑边映射
func calculateTraversedEdges(
	ctx context.Context,
	repo repository.IEngineRepository,
	record []model.Task,
	processInstId, processId int,
	status uint8,
) (map[string][]string, error) {
	define, err := repo.GetProcessDefineByVersion(ctx, processId, 0)
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

		if task.Status != 2 && task.Status != 3 && task.PrevNodeID != "" {
			if analyzer.isBatchTainted(task.NodeID, task.BatchCode) {
				continue
			}

			logicalPrevID := topology.resolveLogicalPrev(task.PrevNodeID)
			path := topology.findPath(logicalPrevID, task.NodeID)

			if len(path) > 1 {
				for i := 0; i < len(path)-1; i++ {
					uniqueAppend(litEdges, path[i], path[i+1])
				}
			}
		}

		if analyzer.isBatchEffectivelyPassed(task) {
			if analyzer.hasNewerBatchPending(task.NodeID, task.BatchCode) {
				continue
			}

			nextNodes := topology.resolveNextNodes(task.NodeID)
			for _, nextID := range nextNodes {
				lightUpForward(task.NodeID, nextID, litEdges, nodesMap, topology.effectiveGraph)
			}
		} else if task.Status == 5 {
			nextNodes := topology.resolveNextNodes(task.NodeID)
			for _, nextID := range nextNodes {
				lightUpForward(task.NodeID, nextID, litEdges, nodesMap, topology.effectiveGraph)
			}
		}
	}

	finalEdges := make(map[string][]string)
	if rootID != "" {
		queue := []string{rootID}
		visitedNode := make(map[string]bool)
		visitedNode[rootID] = true

		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]

			targets := litEdges[curr]
			if len(targets) > 0 {
				finalEdges[curr] = targets
				for _, t := range targets {
					if !visitedNode[t] {
						visitedNode[t] = true
						queue = append(queue, t)
					}
				}
			}
		}
	} else {
		finalEdges = litEdges
	}

	return finalEdges, nil
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
		for _, nextID := range effectiveGraph[toID] {
			lightUpForward(toID, nextID, litEdges, nodesMap, effectiveGraph)
		}
	}
}

func buildEffectiveGraph(nodesMap map[string]model.Node) map[string][]string {
	adj := make(map[string][]string)
	for id, node := range nodesMap {
		for _, prevID := range node.PrevNodeIDs {
			uniqueAppend(adj, prevID, id)
		}
	}
	return adj
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
	if batches, ok := a.stats[nodeID]; ok {
		if stats := batches[batchCode]; stats != nil {
			return stats.SystemPass > 0
		}
	}
	return false
}

func (a *nodeStatusAnalyzer) isBatchEffectivelyPassed(task model.Task) bool {
	if task.IsFinished != 1 || task.Status != 1 {
		return false
	}

	isCosigned := false
	if nodeDef, exists := a.nodesMap[task.NodeID]; exists && nodeDef.IsCosigned == 1 {
		isCosigned = true
	} else if task.IsCosigned == 1 {
		isCosigned = true
	}

	if isCosigned {
		if batches, ok := a.stats[task.NodeID]; ok {
			stats := batches[task.BatchCode]
			if stats != nil && stats.Passed < stats.Total {
				return false
			}
		}
	}

	return true
}

func (a *nodeStatusAnalyzer) hasNewerBatchPending(nodeID, currentBatchCode string) bool {
	batches, ok := a.stats[nodeID]
	if !ok {
		return false
	}

	for batchCode, stats := range batches {
		if batchCode != currentBatchCode && stats.Pending > 0 {
			return true
		}
	}
	return false
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
	for _, uid := range node.UserIDs {
		if uid == easyflow.SysAutoUser {
			return true
		}
	}
	return false
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

			if nextID == endID || nextNode.NodeType == model.GateWayNode {
				if !visited[nextID] {
					visited[nextID] = true
					newNodes := make([]string, len(currPath.nodes)+1)
					copy(newNodes, currPath.nodes)
					newNodes[len(currPath.nodes)] = nextID
					queue = append(queue, path{nodes: newNodes})
				}
			}
		}
	}

	return nil
}

func (g *graphTopologyService) resolveNextNodes(nodeID string) []string {
	return g.effectiveGraph[nodeID]
}
