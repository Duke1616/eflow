package easyflow

import (
	"fmt"
	"strings"

	"github.com/samber/lo"
)

// ValidateError 流程图拓扑校验失败的哨兵错误类型。
// 使用单独类型而非普通 error，是为了让上层（Web Handler）通过 errors.As 精确识别「用户设计错误」vs「系统内部异常」，从而为前端返回可读的提示信息。
type ValidateError struct {
	msg string
}

func (e *ValidateError) Error() string { return e.msg }

func newValidateError(format string, args ...any) *ValidateError {
	return &ValidateError{msg: fmt.Sprintf(format, args...)}
}

// IValidator 流程图拓扑合法性校验器接口
type IValidator interface {
	Validate(ctx *Context) error
}

// ValidateAll 串行执行所有内置校验器，按「元素合法性 → 边界 → 图结构 → 连通性 → 网关与配置」层层递进校验，任意一个失败即返回错误
func ValidateAll(ctx *Context) error {
	validators := []IValidator{
		// 1. 基础元素合法性
		&validateSupportedNodeTypes{},
		&validateEdgeEndpoints{},
		&validateNoSelfLoops{},
		// 2. 开始与结束节点边界
		&validateStartEnd{},
		// 3. 图拓扑结构（无环 DAG 检测）
		&validateAcyclic{},
		// 4. 双向连通性与死胡同校验（全图可达性）
		&validateReachability{},
		// 5. 网关拓扑与死锁防护
		&validateConditionParallelDeadlock{},
		&validateParallelForkJoinPairing{},
		&validateSelectiveTargetsMustBeCondition{},
		&validateSelectiveMinBranches{},
		// 6. 节点与连接线配置完整性
		&validateConditionEdgeExpression{},
		&validateNoDuplicateConditionExpressions{},
	}

	for _, v := range validators {
		if err := v.Validate(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 通用图拓扑辅助工具函数
// ─────────────────────────────────────────────────────────────────────────────

// getNodeDisplayName 获取节点的用户可读名称，格式为 "节点名 (节点ID)" 或仅 "节点ID"
func getNodeDisplayName(node Node) string {
	if p, _ := ToNodeProperty[struct{ Name string }](node); p.Name != "" {
		return fmt.Sprintf("%s (%s)", p.Name, node.ID)
	}
	return node.ID
}

// getAllEdges 扁平化获取流程图中所有的连线
func getAllEdges(ctx *Context) []Edge {
	return lo.Flatten(lo.Values(ctx.EdgesMap))
}

// isMergeGateway 判断节点是否为等待并发前置的聚合网关（parallel 或 inclusion）
func isMergeGateway(nodeType string) bool {
	return nodeType == NodeTypeParallel || nodeType == NodeTypeInclusion
}

// intersectMany 计算多个切片的交集，并保证结果去重
func intersectMany(slices ...[]string) []string {
	if len(slices) == 0 {
		return nil
	}
	res := lo.Uniq(slices[0])
	for i := 1; i < len(slices); i++ {
		res = lo.Intersect(res, slices[i])
	}
	return res
}

// branchGatewayScan 从指定分支起始节点出发，沿有向边深度搜索遇到的首层聚合网关及是否直达 End 节点
type branchGatewayScan struct {
	gateways   []string // 遇到的首层聚合网关 ID 集合（到达聚合网关后停止向下扩散）
	reachedEnd bool     // 是否存在未经过任何聚合网关而直接流向 End 节点的路径
	endNodeID  string   // 若直达 End，记录该 End 节点 ID
}

// scanBranchDownstream 从分支起始节点出发做 BFS 遍历，分析该分支下游的汇聚走向。
// stopAtExclusiveJoin: 若为 true，当遇到排他汇聚节点（多入边的 condition 节点）时停止扩散，
// 避免将已在排他汇聚节点闭合的单线流转误判为并发死锁。
func scanBranchDownstream(ctx *Context, startNodeID string, stopAtExclusiveJoin bool) branchGatewayScan {
	var joins []string
	var endID string
	reachedEnd := false

	visited := make(map[string]bool)
	queue := []string{startNodeID}
	visited[startNodeID] = true

	for len(queue) > 0 {
		currID := queue[0]
		queue = queue[1:]
		currNode := ctx.GetNodeInfo(currID)

		// 若到达聚合网关，记录该网关 ID，不再向该网关下游继续深入（已达汇聚边界）
		if isMergeGateway(currNode.Type) {
			joins = append(joins, currID)
			continue
		}

		// 若为排他汇聚点（多入边的 condition 节点），说明多条互斥分支在此处已先合并为单线流转
		if stopAtExclusiveJoin && currNode.Type == NodeTypeCondition && len(ctx.PrevNodesMap[currID]) > 1 {
			continue
		}

		// 若未经聚合网关直接流向了结束节点，标记并记录
		if currNode.Type == NodeTypeEnd {
			reachedEnd = true
			endID = currID
			continue
		}

		for _, edge := range ctx.GetTargetEdges(currID) {
			if !visited[edge.TargetNodeId] {
				visited[edge.TargetNodeId] = true
				queue = append(queue, edge.TargetNodeId)
			}
		}
	}

	return branchGatewayScan{
		gateways:   lo.Uniq(joins),
		reachedEnd: reachedEnd,
		endNodeID:  endID,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. 基础元素合法性校验
// ─────────────────────────────────────────────────────────────────────────────

// validateSupportedNodeTypes 检测所有节点类型是否受系统引擎支持
type validateSupportedNodeTypes struct{}

func (v *validateSupportedNodeTypes) Validate(ctx *Context) error {
	supportedTypes := map[string]bool{
		NodeTypeStart:     true,
		NodeTypeEnd:       true,
		NodeTypeUser:      true,
		NodeTypeCondition: true,
		NodeTypeParallel:  true,
		NodeTypeInclusion: true,
		NodeTypeSelective: true,
		NodeTypeAuto:      true,
		NodeTypeChat:      true,
	}

	badNode, found := lo.Find(lo.Values(ctx.NodesMap), func(n Node) bool {
		return !supportedTypes[n.Type]
	})
	if found {
		return newValidateError("节点 [%s] 的类型 [%s] 不受系统支持，请检查节点配置", getNodeDisplayName(badNode), badNode.Type)
	}
	return nil
}

// validateEdgeEndpoints 检测连线的起点与终点节点在节点列表中是否存在（清理残留脏连线）
type validateEdgeEndpoints struct{}

func (v *validateEdgeEndpoints) Validate(ctx *Context) error {
	badEdge, found := lo.Find(getAllEdges(ctx), func(e Edge) bool {
		_, srcOk := ctx.NodesMap[e.SourceNodeId]
		_, dstOk := ctx.NodesMap[e.TargetNodeId]
		return !srcOk || !dstOk
	})
	if !found {
		return nil
	}

	if _, ok := ctx.NodesMap[badEdge.SourceNodeId]; !ok {
		return newValidateError("连线 [%s] 的起点节点 [%s] 不存在，请清理画布上的残留连线", badEdge.ID, badEdge.SourceNodeId)
	}
	return newValidateError("连线 [%s] 的目标节点 [%s] 不存在，请清理画布上的残留连线", badEdge.ID, badEdge.TargetNodeId)
}

// validateNoSelfLoops 检测禁止节点指向自身（自环）
type validateNoSelfLoops struct{}

func (v *validateNoSelfLoops) Validate(ctx *Context) error {
	loopEdge, found := lo.Find(getAllEdges(ctx), func(e Edge) bool {
		return e.SourceNodeId == e.TargetNodeId
	})
	if found {
		node := ctx.GetNodeInfo(loopEdge.SourceNodeId)
		return newValidateError("节点 [%s] 存在指向自身的连线（自环），请删除该连线", getNodeDisplayName(node))
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. 开始与结束节点边界校验
// ─────────────────────────────────────────────────────────────────────────────

// validateStartEnd 检测开始节点和结束节点的数量及出入边限制：
// - 必须且仅有 1 个开始节点，且开始节点不能有入边
// - 至少有 1 个结束节点，且结束节点不能有出边
type validateStartEnd struct{}

func (v *validateStartEnd) Validate(ctx *Context) error {
	allNodes := lo.Values(ctx.NodesMap)
	startNodes := lo.Filter(allNodes, func(n Node, _ int) bool { return n.Type == NodeTypeStart })
	endNodes := lo.Filter(allNodes, func(n Node, _ int) bool { return n.Type == NodeTypeEnd })

	if len(startNodes) == 0 {
		return newValidateError("流程缺少开始节点，请添加一个 [start] 节点")
	}
	if len(startNodes) > 1 {
		return newValidateError("流程存在多个开始节点（共 %d 个），只允许有一个开始节点", len(startNodes))
	}
	startNode := startNodes[0]
	if len(ctx.PrevNodesMap[startNode.ID]) > 0 {
		return newValidateError("开始节点 [%s] 不能有前置连线（入边），请删除指向开始节点的连线", getNodeDisplayName(startNode))
	}

	if len(endNodes) == 0 {
		return newValidateError("流程缺少结束节点，请添加至少一个 [end] 节点")
	}
	badEnd, found := lo.Find(endNodes, func(end Node) bool {
		return len(ctx.EdgesMap[end.ID]) > 0
	})
	if found {
		return newValidateError("结束节点 [%s] 不能有后续连线（出边），请删除从结束节点指出的连线", getNodeDisplayName(badEnd))
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. 图结构校验（无环 DAG 检测）
// ─────────────────────────────────────────────────────────────────────────────

// validateAcyclic 使用拓扑排序（Kahn 算法）检测流程图是否存在有向环路。
// 工作流引擎推进依赖有向无环图（DAG），若存在循环回路会导致流程陷入无限死循环或网关死锁。
type validateAcyclic struct{}

func (v *validateAcyclic) Validate(ctx *Context) error {
	inDegree := make(map[string]int, len(ctx.NodesMap))
	for id := range ctx.NodesMap {
		inDegree[id] = len(ctx.PrevNodesMap[id])
	}

	queue := lo.Filter(lo.Keys(inDegree), func(id string, _ int) bool {
		return inDegree[id] == 0
	})

	visitedCount := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visitedCount++

		for _, edge := range ctx.EdgesMap[curr] {
			inDegree[edge.TargetNodeId]--
			if inDegree[edge.TargetNodeId] == 0 {
				queue = append(queue, edge.TargetNodeId)
			}
		}
	}

	if visitedCount < len(ctx.NodesMap) {
		cyclePath := findCyclePath(ctx, inDegree)
		if cyclePath != "" {
			return newValidateError("流程图中存在死循环环路（路径: %s），工作流必须是有向无环图（DAG），请移除回路连线", cyclePath)
		}
		return newValidateError("流程图中存在死循环环路，工作流必须是有向无环图（DAG），请检查并移除回路连线")
	}

	return nil
}

// findCyclePath 在入度仍大于 0 的节点子图中通过 DFS 追溯一条环路路径用于友好报错
func findCyclePath(ctx *Context, inDegree map[string]int) string {
	cycleNodes := lo.PickBy(inDegree, func(_ string, deg int) bool { return deg > 0 })

	visited := make(map[string]int) // 0: 未访问, 1: 访问中, 2: 已完成
	var path, cycle []string

	var dfs func(u string) bool
	dfs = func(u string) bool {
		visited[u] = 1
		path = append(path, u)

		for _, edge := range ctx.EdgesMap[u] {
			v := edge.TargetNodeId
			if _, ok := cycleNodes[v]; !ok {
				continue
			}
			if visited[v] == 1 {
				idx := lo.IndexOf(path, v)
				if idx != -1 {
					cycle = append(path[idx:], v)
					return true
				}
			} else if visited[v] == 0 {
				if dfs(v) {
					return true
				}
			}
		}

		visited[u] = 2
		path = path[:len(path)-1]
		return false
	}

	for id := range cycleNodes {
		if visited[id] == 0 && dfs(id) {
			names := lo.Map(cycle, func(nodeID string, _ int) string {
				return getNodeDisplayName(ctx.GetNodeInfo(nodeID))
			})
			return strings.Join(names, " → ")
		}
	}
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. 连通性与死胡同校验（双向可达性）
// ─────────────────────────────────────────────────────────────────────────────

// validateReachability 验证全图的双向可达性：
// 1. 正向连通：从 Start 节点出发必须能遍历到达图中的每一个节点（杜绝游离孤岛节点）
// 2. 反向连通：除了 End 节点外，所有节点都必须能够流转到达至少一个 End 节点（杜绝断头分支和死胡同）
type validateReachability struct{}

func (v *validateReachability) Validate(ctx *Context) error {
	startNode, ok := lo.Find(lo.Values(ctx.NodesMap), func(n Node) bool { return n.Type == NodeTypeStart })
	if !ok {
		return nil // 由 validateStartEnd 统一报错
	}

	// 1. 正向可达遍历（从 Start 出发）
	reachableFromStart := make(map[string]bool, len(ctx.NodesMap))
	queue := []string{startNode.ID}
	reachableFromStart[startNode.ID] = true

	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]

		for _, edge := range ctx.EdgesMap[curr] {
			if !reachableFromStart[edge.TargetNodeId] {
				reachableFromStart[edge.TargetNodeId] = true
				queue = append(queue, edge.TargetNodeId)
			}
		}
	}

	unreachableNode, found := lo.Find(lo.Values(ctx.NodesMap), func(n Node) bool {
		return !reachableFromStart[n.ID]
	})
	if found {
		return newValidateError(
			"节点 [%s] 无法从开始节点到达（属于游离孤岛节点），请连接到流程中或将其删除",
			getNodeDisplayName(unreachableNode),
		)
	}

	// 2. 反向可达遍历（从所有 End 节点沿着入边反向追溯）
	canReachEnd := make(map[string]bool, len(ctx.NodesMap))
	endNodes := lo.Filter(lo.Values(ctx.NodesMap), func(n Node, _ int) bool { return n.Type == NodeTypeEnd })
	endQueue := lo.Map(endNodes, func(n Node, _ int) string {
		canReachEnd[n.ID] = true
		return n.ID
	})

	for len(endQueue) > 0 {
		curr := endQueue[0]
		endQueue = endQueue[1:]

		for _, prevID := range ctx.PrevNodesMap[curr] {
			if !canReachEnd[prevID] {
				canReachEnd[prevID] = true
				endQueue = append(endQueue, prevID)
			}
		}
	}

	deadEndNode, found := lo.Find(lo.Values(ctx.NodesMap), func(n Node) bool {
		return !canReachEnd[n.ID]
	})
	if found {
		return newValidateError(
			"从节点 [%s] 出发无法流转到任何结束节点（分支陷入死胡同），请确保该分支最终连接到结束节点",
			getNodeDisplayName(deadEndNode),
		)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. 网关拓扑与死锁防护
// ─────────────────────────────────────────────────────────────────────────────

// validateConditionParallelDeadlock 检测「条件网关直接汇聚并行网关」导致的死锁拓扑。
//
// 非法场景：一个 condition 节点有 ≥2 条出边（多个条件分支），
// 且这些分支的目标节点在下游未经排他汇合就全部汇聚至同一个 parallel/inclusion 聚合网关。
//
// 死锁原因：条件网关（WaitForAllPrevNode=3）只激活1条满足条件的分支，
// 其余分支无任何任务记录；而并行聚合网关（WaitForAllPrevNode=1）等待所有前置节点完成，
// 非激活分支永久无法完成 → 死锁。
//
// 正确做法：使用 [selective] 节点作为分叉点，搭配多个 [condition] 子网关，
// 由 EventSelectiveGatewaySplit 为未激活分支写入 skip 记录，聚合网关才能正常推进。
type validateConditionParallelDeadlock struct{}

func (v *validateConditionParallelDeadlock) Validate(ctx *Context) error {
	for _, node := range ctx.NodesMap {
		if node.Type != NodeTypeCondition {
			continue
		}

		outEdges := ctx.GetTargetEdges(node.ID)
		if len(outEdges) <= 1 {
			continue
		}

		// 利用 scanBranchDownstream 全链路深度搜索每个分支下游遇到的聚合网关
		// stopAtExclusiveJoin=true: 若多条条件分支在下游先汇入了一个排他汇聚点（多入边的 condition 节点），
		// 则流出该汇聚点时已闭合为单线，不再作为互斥分支误判死锁
		branchGateways := lo.Map(outEdges, func(edge Edge, _ int) []string {
			return scanBranchDownstream(ctx, edge.TargetNodeId, true).gateways
		})

		// 计算所有分支共同汇聚的聚合网关交集
		commonGateways := intersectMany(branchGateways...)
		if len(commonGateways) > 0 {
			gwNode := ctx.GetNodeInfo(commonGateways[0])
			return newValidateError(
				"检测到无效拓扑：条件网关节点 [%s] 的 %d 条分支均汇聚至并行聚合网关 [%s]。\n"+
					"条件网关运行时只会激活1条满足条件的分支，其余分支因无任务记录导致并行聚合网关永久等待（死锁）。\n"+
					"修复方式：将此分叉节点改为 [selective] 类型，并为每条条件分支单独添加一个 [condition] 子网关",
				getNodeDisplayName(node), len(outEdges), getNodeDisplayName(gwNode),
			)
		}
	}
	return nil
}

// validateParallelForkJoinPairing 检测并行网关分叉必须与聚合网关成对闭合（Fork-Join 闭合规范）。
//
// 业务背景与原理：
//  1. 并行网关（parallel）若有 ≥2 条出边，即为并发分叉点（Fork），同时激活所有分支。
//  2. 所有并发分支必须在到达 End 节点前，统一汇聚到同一个并行/包容聚合网关（Join），
//     严禁任何并发分支未聚合直接连到 End 节点。
//     原因：在 EasyFlow 引擎中，任意分支先到达 End 节点会触发 EventNotify 立即将工单置为终态 END，
//     导致其余尚未审批完成的分支直接被系统丢弃，破坏业务审批完整性。
type validateParallelForkJoinPairing struct{}

func (v *validateParallelForkJoinPairing) Validate(ctx *Context) error {
	for _, node := range ctx.NodesMap {
		if node.Type != NodeTypeParallel {
			continue
		}
		forkEdges := ctx.GetTargetEdges(node.ID)
		if len(forkEdges) < 2 {
			continue // 出边 < 2 不是分叉点（可能是聚合网关或单线过渡），不在此规则约束范围
		}

		// 全链路搜索每条并发分支的聚合网关候选列表及是否直连 End
		branchScans := lo.Map(forkEdges, func(edge Edge, _ int) branchGatewayScan {
			return scanBranchDownstream(ctx, edge.TargetNodeId, false)
		})

		// 1. 若有并发分支未经过聚合网关就直接流向结束节点，立即拦截报错
		if badScan, found := lo.Find(branchScans, func(s branchGatewayScan) bool { return s.reachedEnd }); found {
			endNode := ctx.GetNodeInfo(badScan.endNodeID)
			return newValidateError(
				"并行网关节点 [%s] 的并发分支未经过聚合网关就直接流向了结束节点 [%s]。\n"+
					"并行网关分叉（Fork）与聚合（Join）必须成对闭合，请在流向结束节点前添加并行/包容聚合网关将所有并发分支汇聚",
				getNodeDisplayName(node), getNodeDisplayName(endNode),
			)
		}

		// 2. 检查所有并发分支是否最终汇聚到了同一个聚合网关（交集非空）
		allGateways := lo.Map(branchScans, func(s branchGatewayScan, _ int) []string { return s.gateways })
		commonJoins := intersectMany(allGateways...)
		if len(commonJoins) == 0 {
			return newValidateError(
				"并行网关节点 [%s] 的 %d 条并发分支未汇聚到同一个聚合网关。\n"+
					"并行分叉（Fork）与聚合（Join）必须成对闭合，请确保所有并发分支最终汇聚到同一个并行/包容聚合网关",
				getNodeDisplayName(node), len(forkEdges),
			)
		}
	}
	return nil
}

// validateSelectiveTargetsMustBeCondition 检测 selective 作为分叉网关时的直接目标节点必须全部是 condition 子网关。
//
// 角色区分机制：
// 1. 汇聚网关角色（Join）：当 selective 的入边 ≥ 2 且出边 ≤ 1 时，其扮演的是分支汇聚等待网关（底层具备 WaitForAllPrevNode=1 汇聚能力），
// validateSelectiveTargetsMustBeCondition 检测 selective 节点的所有出边目标必须是 condition 网关。
//
// 语义说明：selective 属于「条件并行分叉网关」，仅用于向下发散分流（selective → condition → 业务节点）。
// 它不能作为分支汇聚节点使用；多分支汇聚必须使用标准的【并行网关（parallel）】。
type validateSelectiveTargetsMustBeCondition struct{}

func (v *validateSelectiveTargetsMustBeCondition) Validate(ctx *Context) error {
	for _, node := range ctx.NodesMap {
		if node.Type != NodeTypeSelective {
			continue
		}

		badEdge, found := lo.Find(ctx.GetTargetEdges(node.ID), func(e Edge) bool {
			return ctx.GetNodeInfo(e.TargetNodeId).Type != NodeTypeCondition
		})
		if found {
			target := ctx.GetNodeInfo(badEdge.TargetNodeId)
			return newValidateError(
				"selective（条件并行网关）节点 [%s] 的出边直接连接了非 condition 节点 [%s]（类型：%s）。\n"+
					"selective 属于条件分叉网关，必须与 condition 子网关配合使用（selective → condition → 业务节点）。\n"+
					"若此处用于多分支汇聚，请将其替换为【并行网关（parallel，纯加号图标）】",
				getNodeDisplayName(node), getNodeDisplayName(target), target.Type,
			)
		}
	}
	return nil
}

// validateSelectiveMinBranches 检测 selective 必须有 ≥2 条出边（至少2个条件分支）。
type validateSelectiveMinBranches struct{}

func (v *validateSelectiveMinBranches) Validate(ctx *Context) error {
	for _, node := range ctx.NodesMap {
		if node.Type != NodeTypeSelective {
			continue
		}
		outCount := len(ctx.GetTargetEdges(node.ID))
		if outCount < 2 {
			return newValidateError(
				"selective（条件并行网关）节点 [%s] 只有 %d 条出边，至少需要 2 条才有条件分叉语义。\n"+
					"若此处用于多分支汇聚，请将其替换为【并行网关（parallel，纯加号图标）】",
				getNodeDisplayName(node), outCount,
			)
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. 节点与边配置合法性校验
// ─────────────────────────────────────────────────────────────────────────────

// validateConditionEdgeExpression 检测条件网关的连接线必须配置条件表达式。
//
//  1. 多分支分叉场景（出边 ≥ 2）：每条出边代表互斥的路由分支，必须配置表达式，否则条件恒真会导致多分支并发冲突。
//  2. Selective 子网关场景（入边来自 selective）：selective 依赖 condition 上的表达式判定分支并对未命中分支写入 skip 记录，
//     必须配置表达式，否则条件恒真导致 selective 失效退化为 parallel。
//  3. 普通单出边过渡场景（入边非 selective 且出边 = 1）：允许使用系统底层默认 "1 = 1" 直通，不强制拦截。
type validateConditionEdgeExpression struct{}

func (v *validateConditionEdgeExpression) Validate(ctx *Context) error {
	for _, node := range ctx.NodesMap {
		if node.Type != NodeTypeCondition {
			continue
		}
		outEdges := ctx.GetTargetEdges(node.ID)
		if len(outEdges) == 0 {
			continue
		}

		isSelectiveBranch := lo.SomeBy(ctx.PrevNodesMap[node.ID], func(prevID string) bool {
			return ctx.GetNodeInfo(prevID).Type == NodeTypeSelective
		})

		mustHaveExpr := len(outEdges) >= 2 || isSelectiveBranch
		if !mustHaveExpr {
			continue
		}

		badEdge, found := lo.Find(outEdges, func(e Edge) bool {
			prop, _ := ToEdgeProperty(e)
			return strings.TrimSpace(prop.Expression) == ""
		})
		if found {
			target := ctx.GetNodeInfo(badEdge.TargetNodeId)
			if isSelectiveBranch {
				return newValidateError(
					"条件网关节点 [%s] 作为 selective 的条件分支流向 [%s]，但未配置条件表达式。\n"+
						"selective 的每个条件分支连接线都必须配置表达式（如 $env == 'backend'），\n"+
						"否则无法识别未激活分支，导致跳过逻辑失效",
					getNodeDisplayName(node), getNodeDisplayName(target),
				)
			}
			return newValidateError(
				"条件网关节点 [%s] 流向 [%s] 的连接线未配置条件表达式。\n"+
					"多分支条件网关的每条连接线都必须填写条件表达式（如 $amount > 1000），\n"+
					"否则表达式默认为恒真，会导致多个分支被同时激活",
				getNodeDisplayName(node), getNodeDisplayName(target),
			)
		}
	}
	return nil
}

// validateNoDuplicateConditionExpressions 检测条件分支是否存在重复的条件表达式。
//
// 业务背景与原理：
//  1. Selective 网关（条件并行网关）：其下游每个 condition 子分支必须具备互斥或区分度的条件表达式。
//     若存在重复表达式（如分支A为 $env == 'prod'，分支B也为 $env == 'prod'），
//     会导致两分支同时激活或同时跳过，通常属于设计者复制节点后未修改表达式的配置笔误。
//  2. 多出边 Condition 网关（出边 ≥ 2）：同一个条件网关的多条出边若条件完全相同，
//     违反条件分流路由语义，属于配置错误。
type validateNoDuplicateConditionExpressions struct{}

func (v *validateNoDuplicateConditionExpressions) Validate(ctx *Context) error {
	for _, node := range ctx.NodesMap {
		switch node.Type {
		case NodeTypeSelective:
			// 检查 selective 下所有 condition 分支的表达式
			outEdges := ctx.GetTargetEdges(node.ID)
			seenExpr := make(map[string]string) // normalizedExpr -> conditionNodeID
			for _, e := range outEdges {
				condNode := ctx.GetNodeInfo(e.TargetNodeId)
				if condNode.Type != NodeTypeCondition {
					continue
				}
				condOutEdges := ctx.GetTargetEdges(condNode.ID)
				for _, ce := range condOutEdges {
					prop, _ := ToEdgeProperty(ce)
					rawExpr := strings.TrimSpace(prop.Expression)
					if rawExpr == "" {
						continue
					}
					// 空白归一化（将连续空格收敛为单空格，防止由于多打空格规避校验）
					normExpr := strings.Join(strings.Fields(rawExpr), " ")
					if prevCondID, exists := seenExpr[normExpr]; exists {
						prevCondNode := ctx.GetNodeInfo(prevCondID)
						return newValidateError(
							"条件并行网关节点 [%s] 的多个条件分支配置了重复的条件表达式: '%s'（分支 [%s] 与 [%s] 重复）。\n"+
								"同一网关下的各个条件分支表达式必须具有区分度，请检查是否存在配置错误或复制粘贴遗漏",
							getNodeDisplayName(node), rawExpr,
							getNodeDisplayName(prevCondNode), getNodeDisplayName(condNode),
						)
					}
					seenExpr[normExpr] = condNode.ID
				}
			}

		case NodeTypeCondition:
			outEdges := ctx.GetTargetEdges(node.ID)
			if len(outEdges) < 2 {
				continue
			}
			seenExpr := make(map[string]string) // normalizedExpr -> targetNodeID
			for _, e := range outEdges {
				prop, _ := ToEdgeProperty(e)
				rawExpr := strings.TrimSpace(prop.Expression)
				if rawExpr == "" {
					continue
				}
				normExpr := strings.Join(strings.Fields(rawExpr), " ")
				if prevTargetID, exists := seenExpr[normExpr]; exists {
					prevTarget := ctx.GetNodeInfo(prevTargetID)
					currTarget := ctx.GetNodeInfo(e.TargetNodeId)
					return newValidateError(
						"条件网关节点 [%s] 流向不同目标的分支配置了重复的条件表达式: '%s'（流向 [%s] 与 [%s] 重复）。\n"+
							"多分支条件网关的各个分支表达式不能相同，请检查是否存在配置错误",
						getNodeDisplayName(node), rawExpr,
						getNodeDisplayName(prevTarget), getNodeDisplayName(currTarget),
					)
				}
				seenExpr[normExpr] = e.TargetNodeId
			}
		}
	}
	return nil
}
