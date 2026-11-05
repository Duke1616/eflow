package easyflow

import (
	"fmt"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
)

// StartNodeHandler 处理开始节点
type StartNodeHandler struct{}

func (h *StartNodeHandler) Type() flowdsl.NodeType { return NodeTypeStart }
func (h *StartNodeHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	nodeName := node.Name
	if nodeName == "" {
		nodeName = DefaultNameStart
	}
	n := model.Node{
		NodeID:        node.ID,
		NodeName:      nodeName,
		NodeType:      model.RootNode,
		UserIDs:       []string{UserStarter},
		NodeEndEvents: []string{EventStart},
	}
	return []model.Node{n}, nil
}

// EndNodeHandler 处理结束节点
type EndNodeHandler struct{}

func (h *EndNodeHandler) Type() flowdsl.NodeType { return NodeTypeEnd }
func (h *EndNodeHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	nodeName := node.Name
	if nodeName == "" {
		nodeName = DefaultNameEnd
	}
	n := model.Node{
		NodeID:          node.ID,
		NodeName:        nodeName,
		NodeType:        model.EndNode,
		PrevNodeIDs:     ctx.GetPrevNodeIDs(node.ID),
		NodeStartEvents: []string{EventNotify},
	}
	return []model.Node{n}, nil
}

// UserNodeHandler 处理用户/审批节点
type UserNodeHandler struct{}

func (h *UserNodeHandler) Type() flowdsl.NodeType { return NodeTypeUser }
func (h *UserNodeHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	nodeName := node.Name
	if nodeName == "" {
		nodeName = DefaultNameUser
	}

	startEvents := []string{EventNotify}
	var userIDs []string
	isCosigned := 0

	if cfg := node.UserTask; cfg != nil {
		if cfg.IsCC {
			startEvents = []string{EventCarbonCopy}
		}
		if cfg.IsCosigned {
			isCosigned = 1
		}
		for _, a := range cfg.Assignees {
			if a.Rule == flowdsl.APPOINT || a.Rule == "" {
				userIDs = append(userIDs, a.Values...)
			}
		}
	}

	// NOTE: 兼容历史 LogicFlow 直接在 properties 里配置 approved 静态审批人列表
	if len(userIDs) == 0 && node.Props != nil {
		if approvedRaw, ok := node.Props["approved"]; ok {
			if approvedList, ok := approvedRaw.([]any); ok {
				for _, item := range approvedList {
					if str, ok := item.(string); ok {
						userIDs = append(userIDs, str)
					}
				}
			} else if approvedStrs, ok := approvedRaw.([]string); ok {
				userIDs = append(userIDs, approvedStrs...)
			}
		}
	}

	if userIDs == nil {
		userIDs = []string{}
	}

	n := model.Node{
		NodeID:          node.ID,
		NodeName:        nodeName,
		NodeType:        model.TaskNode,
		PrevNodeIDs:     findAndProxySrcNodes(ctx, node),
		UserIDs:         userIDs,
		IsCosigned:      isCosigned,
		NodeStartEvents: startEvents,
	}

	n.TaskFinishEvents = append(getPassEvents(ctx, node.ID), getRejectEvents(ctx, node.ID)...)

	return []model.Node{n}, nil
}

// Gateway Handlers

type ParallelHandler struct{}

func (h *ParallelHandler) Type() flowdsl.NodeType { return NodeTypeParallel }
func (h *ParallelHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	edgesDst := ctx.GetTargetEdges(node.ID)
	inevitableNodes := make([]string, 0, len(edgesDst))
	for _, edge := range edgesDst {
		inevitableNodes = append(inevitableNodes, ctx.GetOrGenerateProxyID(node.ID, edge.Target))
	}
	gw := model.HybridGateway{
		Conditions:         nil,
		InevitableNodes:    inevitableNodes,
		WaitForAllPrevNode: 1,
	}
	n := model.Node{
		NodeID:      node.ID,
		NodeName:    DefaultNameParallel,
		NodeType:    model.GateWayNode,
		GWConfig:    gw,
		PrevNodeIDs: findAndProxySrcNodesForGateway(ctx, node),
	}
	return []model.Node{n}, nil
}

type InclusionHandler struct{}

func (h *InclusionHandler) Type() flowdsl.NodeType { return NodeTypeInclusion }
func (h *InclusionHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	edgesDst := ctx.GetTargetEdges(node.ID)
	inevitableNodes := make([]string, 0, len(edgesDst))
	for _, edge := range edgesDst {
		inevitableNodes = append(inevitableNodes, ctx.GetOrGenerateProxyID(node.ID, edge.Target))
	}
	gw := model.HybridGateway{
		Conditions:         nil,
		InevitableNodes:    inevitableNodes,
		WaitForAllPrevNode: 0,
	}
	n := model.Node{
		NodeID:      node.ID,
		NodeName:    DefaultNameInclusion,
		NodeType:    model.GateWayNode,
		GWConfig:    gw,
		PrevNodeIDs: findAndProxySrcNodesForGateway(ctx, node),
	}
	return []model.Node{n}, nil
}

type SelectiveHandler struct{}

func (h *SelectiveHandler) Type() flowdsl.NodeType { return NodeTypeSelective }
func (h *SelectiveHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	edgesDst := ctx.GetTargetEdges(node.ID)
	inevitableNodes := make([]string, 0, len(edgesDst))
	for _, edge := range edgesDst {
		inevitableNodes = append(inevitableNodes, ctx.GetOrGenerateProxyID(node.ID, edge.Target))
	}
	gw := model.HybridGateway{
		Conditions:         nil,
		InevitableNodes:    inevitableNodes,
		WaitForAllPrevNode: 1,
	}
	n := model.Node{
		NodeID:          node.ID,
		NodeName:        DefaultNameSelective,
		NodeType:        model.GateWayNode,
		GWConfig:        gw,
		PrevNodeIDs:     findAndProxySrcNodesForGateway(ctx, node),
		NodeStartEvents: []string{EventSelectiveGatewaySplit},
	}
	return []model.Node{n}, nil
}

type ConditionHandler struct{}

func (h *ConditionHandler) Type() flowdsl.NodeType { return NodeTypeCondition }
func (h *ConditionHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	edgesDst := ctx.GetTargetEdges(node.ID)
	conditions := make([]model.Condition, 0, len(edgesDst))
	for _, edge := range edgesDst {
		expr := edge.Expression
		if expr == "" {
			expr = "1 = 1"
		}
		conditions = append(conditions, model.Condition{
			Expression: expr,
			NodeID:     ctx.GetOrGenerateProxyID(node.ID, edge.Target),
		})
	}

	gw := model.HybridGateway{
		Conditions:         conditions,
		InevitableNodes:    []string{},
		WaitForAllPrevNode: 3,
	}
	n := model.Node{
		NodeID:      node.ID,
		NodeName:    node.Name,
		NodeType:    model.GateWayNode,
		GWConfig:    gw,
		PrevNodeIDs: ctx.GetPrevNodeIDs(node.ID),
	}
	return []model.Node{n}, nil
}

type AutomationNodeHandler struct{}

func (h *AutomationNodeHandler) Type() flowdsl.NodeType { return NodeTypeAuto }
func (h *AutomationNodeHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	cfg := node.AutoTask
	if cfg == nil || cfg.CodebookID <= 0 {
		return nil, fmt.Errorf("自动化节点 %s 未配置有效的脚本文件", node.ID)
	}
	if cfg.CompensationNodeID != "" {
		if cfg.CompensationNodeID == node.ID {
			return nil, fmt.Errorf("自动化节点 %s 不能将自身配置为补偿节点", node.ID)
		}
		compensationNode := ctx.GetNodeInfo(cfg.CompensationNodeID)
		if compensationNode == nil || compensationNode.Type != NodeTypeAuto {
			return nil, fmt.Errorf("自动化节点 %s 配置的补偿节点 %s 不存在或不是自动化节点",
				node.ID, cfg.CompensationNodeID)
		}
	}
	n := model.Node{
		NodeID:          node.ID,
		NodeName:        node.Name,
		NodeType:        model.TaskNode,
		PrevNodeIDs:     ctx.GetPrevNodeIDs(node.ID),
		UserIDs:         []string{AutomationApproval},
		NodeStartEvents: []string{EventAutomation},
		NodeEndEvents:   []string{EventNotify},
	}
	return []model.Node{n}, nil
}

type ChatGroupNodeHandler struct{}

func (h *ChatGroupNodeHandler) Type() flowdsl.NodeType { return NodeTypeChat }
func (h *ChatGroupNodeHandler) Handle(ctx *Context, node *flowdsl.Node) ([]model.Node, error) {
	n := model.Node{
		NodeID:          node.ID,
		NodeName:        node.Name,
		NodeType:        model.TaskNode,
		PrevNodeIDs:     ctx.GetPrevNodeIDs(node.ID),
		UserIDs:         []string{ChatGroupApproval},
		NodeStartEvents: []string{EventChatGroup},
	}
	return []model.Node{n}, nil
}

// 辅助函数

func isGateway(t flowdsl.NodeType) bool {
	return t == NodeTypeParallel || t == NodeTypeInclusion || t == NodeTypeSelective
}

func findAndProxySrcNodes(ctx *Context, node *flowdsl.Node) []string {
	prevIDs := ctx.GetPrevNodeIDs(node.ID)
	result := make([]string, 0, len(prevIDs))
	for _, pid := range prevIDs {
		// 用户节点不需要代理，直接连接
		result = append(result, pid)
	}
	return result
}

func findAndProxySrcNodesForGateway(ctx *Context, node *flowdsl.Node) []string {
	prevIDs := ctx.GetPrevNodeIDs(node.ID)
	nodes := make([]string, 0, len(prevIDs))
	for _, pid := range prevIDs {
		n := ctx.GetNodeInfo(pid)
		if n != nil && n.Type == NodeTypeCondition {
			nodes = append(nodes, ctx.GetOrGenerateProxyForGateway(pid, node.ID))
		} else {
			nodes = append(nodes, pid)
		}
	}
	return nodes
}

func getPassEvents(ctx *Context, nodeID string) []string {
	var events []string
	existEvent := make(map[string]struct{})
	edges := ctx.GetTargetEdges(nodeID)

	for _, e := range edges {
		info := ctx.GetNodeInfo(e.Target)
		if info == nil {
			continue
		}
		switch info.Type {
		case NodeTypeParallel:
			addEvent(&events, existEvent, EventTaskParallelNodePass)
		case NodeTypeInclusion:
			addEvent(&events, existEvent, EventTaskInclusionNodePass)
		}
	}
	return events
}

func getRejectEvents(ctx *Context, nodeID string) []string {
	var events []string
	existEvent := make(map[string]struct{})
	parents := getParents(ctx, nodeID)

	for _, p := range parents {
		grandParents := getParents(ctx, p.ID)
		if p.Type == NodeTypeCondition && hasType(grandParents, NodeTypeParallel, NodeTypeInclusion, NodeTypeSelective) {
			addEvent(&events, existEvent, EventConcurrentRejectCleanup)
		}
		if p.Type == NodeTypeCondition && hasType(grandParents, NodeTypeInclusion) {
			addEvent(&events, existEvent, EventInclusionPassCleanup)
		}
		if isGateway(p.Type) && hasType(grandParents, NodeTypeCondition) {
			addEvent(&events, existEvent, EventGatewayConditionReject)
		}
		if isGateway(p.Type) && hasProxyInGateway(ctx, p.ID) {
			addEvent(&events, existEvent, EventUserNodeRejectProxyCleanup)
		}
	}
	return events
}

type nodeWithInfo struct {
	ID   string
	Type flowdsl.NodeType
}

func getParents(ctx *Context, nodeID string) []nodeWithInfo {
	prevIDs := ctx.GetPrevNodeIDs(nodeID)
	res := make([]nodeWithInfo, 0, len(prevIDs))
	for _, pid := range prevIDs {
		n := ctx.GetNodeInfo(pid)
		if n != nil {
			res = append(res, nodeWithInfo{ID: n.ID, Type: n.Type})
		}
	}
	return res
}

func hasType(nodes []nodeWithInfo, types ...flowdsl.NodeType) bool {
	typeSet := make(map[flowdsl.NodeType]struct{})
	for _, t := range types {
		typeSet[t] = struct{}{}
	}
	for _, n := range nodes {
		if _, ok := typeSet[n.Type]; ok {
			return true
		}
	}
	return false
}

func addEvent(events *[]string, exist map[string]struct{}, event string) {
	if _, ok := exist[event]; !ok {
		*events = append(*events, event)
		exist[event] = struct{}{}
	}
}

func hasProxyInGateway(ctx *Context, gatewayId string) bool {
	edges := ctx.GetTargetEdges(gatewayId)
	for _, edge := range edges {
		if checkBranchHasProxy(ctx, edge.Target, make(map[string]bool)) {
			return true
		}
	}
	return false
}

func checkBranchHasProxy(ctx *Context, nodeID string, visited map[string]bool) bool {
	if visited[nodeID] {
		return false
	}
	visited[nodeID] = true
	node := ctx.GetNodeInfo(nodeID)
	if node != nil && node.Type == NodeTypeCondition {
		return true
	}
	edges := ctx.GetTargetEdges(nodeID)
	for _, edge := range edges {
		if checkBranchHasProxy(ctx, edge.Target, visited) {
			return true
		}
	}
	return false
}
