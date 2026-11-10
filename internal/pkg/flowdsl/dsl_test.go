package flowdsl_test

import (
	"testing"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse_LogicFlow(t *testing.T) {
	rawFlowData := map[string]any{
		"nodes": []map[string]any{
			{
				"id":   "start_1",
				"type": "start",
				"properties": map[string]any{
					"name":      "开始节点",
					"is_notify": true,
				},
			},
			{
				"id":   "approve_1",
				"type": "userTask",
				"properties": map[string]any{
					"name": "主管审批",
					"assignees": []map[string]any{
						{
							"rule":   "leaders",
							"values": []string{"1"},
						},
					},
					"fields": []map[string]any{
						{
							"key":      "amount",
							"name":     "申请金额",
							"type":     "number",
							"required": true,
						},
					},
					"is_cosigned": false,
					"is_cc":       false,
				},
			},
			{
				"id":   "auto_1",
				"type": "automation", // 测试历史别名自动规整
				"properties": map[string]any{
					"name":                  "自动回调",
					"codebook_id":           1001,
					"runner_id":             2002,
					"max_execution_seconds": 1800,
					"is_notify":             true,
				},
			},
			{
				"id":   "end_1",
				"type": "end",
				"properties": map[string]any{
					"name": "结束节点",
				},
			},
		},
		"edges": []map[string]any{
			{
				"id":           "e1",
				"sourceNodeId": "start_1",
				"targetNodeId": "approve_1",
			},
			{
				"id":           "e2",
				"sourceNodeId": "approve_1",
				"targetNodeId": "auto_1",
				"properties": map[string]any{
					"expression": "${amount} <= 1000",
					"is_pass":     true,
				},
			},
			{
				"id":           "e3",
				"sourceNodeId": "auto_1",
				"targetNodeId": "end_1",
			},
		},
	}

	graph, err := flowdsl.Parse(rawFlowData)
	require.NoError(t, err)
	require.NotNil(t, graph)
	assert.Equal(t, "logicflow", graph.GetCanvasType())
	assert.Len(t, graph.GetOrderedNodes(), 4)
	assert.Len(t, graph.GetEdges(), 3)

	// 1. 验证开始节点
	startNode, ok := graph.GetStartNode()
	assert.True(t, ok)
	assert.Equal(t, "start_1", startNode.ID)
	startCfg, hasStart := startNode.AsStart()
	assert.True(t, hasStart)
	assert.True(t, startCfg.IsNotify)

	// 2. 验证审批节点强类型数据
	approveNode, err := graph.MustGetNode("approve_1")
	require.NoError(t, err)
	assert.True(t, approveNode.IsUserTask())
	assert.Equal(t, "主管审批", approveNode.Name)
	userCfg, hasUser := approveNode.AsUserTask()
	assert.True(t, hasUser)
	require.Len(t, userCfg.Assignees, 1)
	assert.Equal(t, flowdsl.LEADER, userCfg.Assignees[0].Rule)
	assert.Equal(t, []string{"1"}, userCfg.Assignees[0].Values)
	require.Len(t, userCfg.Fields, 1)
	assert.Equal(t, "amount", userCfg.Fields[0].Key)
	assert.True(t, userCfg.Fields[0].Required)

	// 3. 验证自动化节点（含别名归一化与 AutoTaskConfig）
	autoNode, err := graph.MustGetNode("auto_1")
	require.NoError(t, err)
	assert.Equal(t, flowdsl.NodeAutoTask, autoNode.Type)
	assert.True(t, autoNode.IsAutoTask())
	autoCfg, hasAuto := autoNode.AsAutoTask()
	assert.True(t, hasAuto)
	assert.Equal(t, int64(1001), autoCfg.CodebookID)
	assert.Equal(t, int64(2002), autoCfg.RunnerID)
	assert.Equal(t, int64(1800), autoCfg.MaxExecutionSeconds)

	// 4. 验证边拓扑与表达式
	outEdges := graph.GetOutEdges("approve_1")
	require.Len(t, outEdges, 1)
	assert.Equal(t, "e2", outEdges[0].ID)
	assert.Equal(t, "${amount} <= 1000", outEdges[0].Expression)
	assert.True(t, outEdges[0].IsPass)

	// 5. 验证下游与上游检索
	nextNodes := graph.GetNextNodes("start_1")
	require.Len(t, nextNodes, 1)
	assert.Equal(t, "approve_1", nextNodes[0].ID)

	prevNodes := graph.GetPrevNodes("end_1")
	require.Len(t, prevNodes, 1)
	assert.Equal(t, "auto_1", prevNodes[0].ID)

	// 6. 拓扑检验与环路检测
	assert.False(t, graph.HasCycle())
	assert.NoError(t, graph.Validate())
}

func TestParse_AntV_X6(t *testing.T) {
	x6Data := `{
		"cells": [
			{
				"id": "node_start",
				"shape": "start-node",
				"label": "发起流程",
				"data": {
					"is_notify": true
				}
			},
			{
				"id": "node_approval",
				"shape": "user-task",
				"data": {
					"name": "总监终审",
					"assignees": [
						{ "rule": "appoint", "values": ["U1001"] }
					],
					"fields": [
						{ "key": "comment", "name": "审批意见", "type": "textarea", "required": false }
					],
					"is_cosigned": true
				}
			},
			{
				"id": "node_end",
				"shape": "end-node",
				"label": "流程结束"
			},
			{
				"id": "edge_1",
				"shape": "edge",
				"source": { "cell": "node_start" },
				"target": { "cell": "node_approval" }
			},
			{
				"id": "edge_2",
				"shape": "edge",
				"source": "node_approval",
				"target": "node_end",
				"data": {
					"expression": "${approved} == true"
				}
			}
		]
	}`

	graph, err := flowdsl.Parse(x6Data)
	require.NoError(t, err)
	require.NotNil(t, graph)
	assert.Equal(t, "x6", graph.GetCanvasType())
	assert.Len(t, graph.GetOrderedNodes(), 3)
	assert.Len(t, graph.GetEdges(), 2)

	// 1. 验证起始节点
	startNode, ok := graph.GetStartNode()
	assert.True(t, ok)
	assert.Equal(t, "node_start", startNode.ID)
	assert.Equal(t, "发起流程", startNode.Name)
	startCfg, hasStart := startNode.AsStart()
	assert.True(t, hasStart)
	assert.True(t, startCfg.IsNotify)

	// 2. 验证审批节点
	appNode, ok := graph.GetNode("node_approval")
	assert.True(t, ok)
	assert.Equal(t, flowdsl.NodeUserTask, appNode.Type)
	assert.Equal(t, "总监终审", appNode.Name)
	userCfg, hasUser := appNode.AsUserTask()
	assert.True(t, hasUser)
	assert.True(t, userCfg.IsCosigned)
	require.Len(t, userCfg.Assignees, 1)
	assert.Equal(t, flowdsl.APPOINT, userCfg.Assignees[0].Rule)
	assert.Equal(t, []string{"U1001"}, userCfg.Assignees[0].Values)
	require.Len(t, userCfg.Fields, 1)
	assert.Equal(t, "comment", userCfg.Fields[0].Key)

	// 3. 验证连线与拓扑
	nextFromApp := graph.GetNextNodes("node_approval")
	require.Len(t, nextFromApp, 1)
	assert.Equal(t, "node_end", nextFromApp[0].ID)

	edge2 := graph.GetOutEdges("node_approval")
	require.Len(t, edge2, 1)
	assert.Equal(t, "${approved} == true", edge2[0].Expression)

	// 4. DAG 无环检测与拓扑验证
	assert.False(t, graph.HasCycle())
	assert.NoError(t, graph.Validate())
}

func TestGraph_HasCycle(t *testing.T) {
	// 构造带环的非法流程图：node1 -> node2 -> node3 -> node1
	graph := flowdsl.NewGraph("test")
	graph.AddNode(&flowdsl.Node{ID: "node1", Type: flowdsl.NodeStart})
	graph.AddNode(&flowdsl.Node{ID: "node2", Type: flowdsl.NodeUserTask})
	graph.AddNode(&flowdsl.Node{ID: "node3", Type: flowdsl.NodeUserTask})

	graph.AddEdge(flowdsl.Edge{ID: "e1", Source: "node1", Target: "node2"})
	graph.AddEdge(flowdsl.Edge{ID: "e2", Source: "node2", Target: "node3"})
	graph.AddEdge(flowdsl.Edge{ID: "e3", Source: "node3", Target: "node1"})

	graph.BuildIndex()
	assert.True(t, graph.HasCycle(), "应该成功检测到环路")
}

func TestParse_EmptyOrInvalid(t *testing.T) {
	t.Run("nil payload", func(t *testing.T) {
		g, err := flowdsl.Parse(nil)
		assert.NoError(t, err)
		assert.Equal(t, "empty", g.GetCanvasType())
		assert.Empty(t, g.GetOrderedNodes())
	})

	t.Run("empty string", func(t *testing.T) {
		g, err := flowdsl.Parse("")
		assert.NoError(t, err)
		assert.Equal(t, "empty", g.GetCanvasType())
	})

	t.Run("unrecognized payload", func(t *testing.T) {
		_, err := flowdsl.Parse(`{"foo":"bar"}`)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "未能识别的画布数据格式")
	})
}
