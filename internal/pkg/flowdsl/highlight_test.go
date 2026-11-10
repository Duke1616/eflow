package flowdsl_test

import (
	"testing"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHighlightGraph(t *testing.T) {
	graph := flowdsl.NewGraph("logicflow")
	graph.AddNode(&flowdsl.Node{ID: "start", Type: flowdsl.NodeStart})
	graph.AddNode(&flowdsl.Node{ID: "node1", Type: flowdsl.NodeUserTask})
	graph.AddNode(&flowdsl.Node{ID: "node2", Type: flowdsl.NodeUserTask})
	graph.AddNode(&flowdsl.Node{ID: "end", Type: flowdsl.NodeEnd})

	// 初始边顺序：先通过的边，后未通过的边
	graph.AddEdge(flowdsl.Edge{ID: "e1", Source: "start", Target: "node1"})
	graph.AddEdge(flowdsl.Edge{ID: "e2", Source: "node1", Target: "end"})
	graph.AddEdge(flowdsl.Edge{ID: "e3", Source: "start", Target: "node2"}) // 跳过的分支

	trace := flowdsl.ExecutionTrace{
		TraversedEdges: map[string][]string{
			"start": {"node1", "node2"},
			"node1": {"end"},
		},
		NodeStatusMap: map[string]int{
			"start": 1,
			"node1": 1,
			"node2": 5, // 跳过
			"end":   1,
		},
	}

	flowdsl.HighlightGraph(graph, trace)

	// 1. 验证边的标记状态
	edgeMap := make(map[string]flowdsl.Edge)
	for _, e := range graph.Edges {
		edgeMap[e.ID] = e
	}

	assert.True(t, edgeMap["e1"].IsPass, "e1 应当标记为通过")
	assert.False(t, edgeMap["e1"].IsSkipped)

	assert.True(t, edgeMap["e2"].IsPass, "e2 应当标记为通过")
	assert.False(t, edgeMap["e2"].IsSkipped)

	assert.True(t, edgeMap["e3"].IsSkipped, "e3 应当标记为跳过")
	assert.False(t, edgeMap["e3"].IsPass)

	// 2. 验证图层优先级：通过/跳过的连线应当排在普通连线之后
	// 在 e1, e2, e3 中，e3 优先级为 1，e1 与 e2 优先级为 2
	assert.Equal(t, "e3", graph.Edges[0].ID, "跳过连线优先级为 1，应排在通过连线之前")
}

func TestHighlightCanvas_LogicFlow(t *testing.T) {
	rawCanvas := map[string]any{
		"nodes": []any{
			map[string]any{"id": "node_a", "type": "start"},
			map[string]any{"id": "node_b", "type": "userTask"},
			map[string]any{"id": "node_c", "type": "userTask"},
		},
		"edges": []any{
			map[string]any{
				"id":           "edge_1",
				"sourceNodeId": "node_a",
				"targetNodeId": "node_b",
				"properties":   map[string]any{},
			},
			map[string]any{
				"id":           "edge_2",
				"sourceNodeId": "node_a",
				"targetNodeId": "node_c",
			},
		},
	}

	trace := flowdsl.ExecutionTrace{
		TraversedEdges: map[string][]string{
			"node_a": {"node_b"},
		},
		NodeStatusMap: map[string]int{
			"node_a": 1,
			"node_b": 1,
			"node_c": 0,
		},
	}

	highlighted, err := flowdsl.HighlightCanvas(rawCanvas, trace)
	require.NoError(t, err)

	edges, ok := highlighted["edges"].([]any)
	require.True(t, ok)
	require.Len(t, edges, 2)

	// 验证未通过的 edge_2 排在前面，高亮通过的 edge_1 排在后面（提升 Z-Index）
	lastEdge := edges[1].(map[string]any)
	assert.Equal(t, "edge_1", lastEdge["id"])

	lastProps := lastEdge["properties"].(map[string]any)
	assert.Equal(t, true, lastProps["is_pass"])
	assert.Nil(t, lastProps["is_skipped"])

	firstEdge := edges[0].(map[string]any)
	assert.Equal(t, "edge_2", firstEdge["id"])
	firstProps := firstEdge["properties"].(map[string]any)
	assert.Nil(t, firstProps["is_pass"])
}

func TestHighlightCanvas_AntV_X6(t *testing.T) {
	rawCanvas := map[string]any{
		"cells": []any{
			map[string]any{"id": "node_1", "shape": "rect"},
			map[string]any{"id": "node_2", "shape": "rect"},
			map[string]any{
				"id":    "edge_x",
				"shape": "edge",
				"source": map[string]any{
					"cell": "node_1",
				},
				"target": map[string]any{
					"cell": "node_2",
				},
				"data": map[string]any{},
			},
		},
	}

	trace := flowdsl.ExecutionTrace{
		TraversedEdges: map[string][]string{
			"node_1": {"node_2"},
		},
		NodeStatusMap: map[string]int{
			"node_1": 1,
			"node_2": 1,
		},
	}

	highlighted, err := flowdsl.HighlightCanvas(rawCanvas, trace)
	require.NoError(t, err)

	cells, ok := highlighted["cells"].([]any)
	require.True(t, ok)
	require.Len(t, cells, 3)

	// 找到 edge_x 检查 data 中的 is_pass
	var edgeCell map[string]any
	for _, cell := range cells {
		cm := cell.(map[string]any)
		if cm["id"] == "edge_x" {
			edgeCell = cm
			break
		}
	}

	require.NotNil(t, edgeCell)
	data := edgeCell["data"].(map[string]any)
	assert.Equal(t, true, data["is_pass"])
}

func TestHighlightCanvas_LogicFlow_SkippedEdge(t *testing.T) {
	rawCanvas := map[string]any{
		"nodes": []any{
			map[string]any{"id": "fork", "type": "gateway"},
			map[string]any{"id": "frontend", "type": "userTask"},
			map[string]any{"id": "join", "type": "gateway"},
		},
		"edges": []any{
			map[string]any{
				"id":           "e_fork_fe",
				"sourceNodeId": "fork",
				"targetNodeId": "frontend",
			},
			map[string]any{
				"id":           "e_fe_join",
				"sourceNodeId": "frontend",
				"targetNodeId": "join",
			},
		},
	}

	trace := flowdsl.ExecutionTrace{
		TraversedEdges: map[string][]string{
			"fork":     {"frontend"},
			"frontend": {"join"},
		},
		NodeStatusMap: map[string]int{
			"fork":     1, // 通过
			"frontend": 5, // 跳过
			"join":     0, // 等待
		},
	}

	highlighted, err := flowdsl.HighlightCanvas(rawCanvas, trace)
	require.NoError(t, err)

	edges := highlighted["edges"].([]any)
	require.Len(t, edges, 2)

	for _, e := range edges {
		edgeMap := e.(map[string]any)
		props := edgeMap["properties"].(map[string]any)
		assert.Equal(t, true, props["is_skipped"], "跳过分支的边必须被打上 is_skipped 属性")
		assert.Nil(t, props["is_pass"])
	}
}
