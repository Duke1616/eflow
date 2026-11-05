package flowdsl

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/samber/lo"
)

type logicFlowParser struct{}

func (p *logicFlowParser) Match(payload []byte) bool {
	return bytes.Contains(payload, []byte(`"nodes"`))
}

func (p *logicFlowParser) Parse(payload []byte) (IGraph, error) {
	var raw rawLogicFlow
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("反序列化 LogicFlow 原始数据失败: %w", err)
	}

	graph := NewGraph("logicflow")

	// 1. 解析节点集合
	lo.ForEach(raw.Nodes, func(rn rawLFNode, _ int) {
		node := &Node{
			ID:   rn.ID,
			Type: NormalizeNodeType(rn.Type),
		}
		decodeNodePayload(rn.Properties, node)
		graph.AddNode(node)
	})

	// 2. 解析连线拓扑
	lo.ForEach(raw.Edges, func(re rawLFEdge, _ int) {
		edge := Edge{
			ID:     re.ID,
			Source: re.SourceNodeId,
			Target: re.TargetNodeId,
		}

		if len(re.Properties) > 0 {
			var prop rawLFEdgeProp
			if err := json.Unmarshal(re.Properties, &prop); err == nil {
				edge.Expression = prop.Expression
				edge.IsPass = prop.IsPass
				edge.IsSkipped = prop.IsSkipped
			}
		}

		graph.AddEdge(edge)
	})

	graph.BuildIndex()
	return graph, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// LogicFlow 内部专属反序列化结构（防腐，不对外暴露）
// ─────────────────────────────────────────────────────────────────────────────

type rawLogicFlow struct {
	Nodes []rawLFNode `json:"nodes"`
	Edges []rawLFEdge `json:"edges"`
}

type rawLFNode struct {
	ID         string          `json:"id"`
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
}

type rawLFEdge struct {
	ID           string          `json:"id"`
	SourceNodeId string          `json:"sourceNodeId"`
	TargetNodeId string          `json:"targetNodeId"`
	Properties   json.RawMessage `json:"properties"`
}

type rawLFEdgeProp struct {
	Expression string `json:"expression"`
	IsPass     bool   `json:"is_pass"`
	IsSkipped  bool   `json:"is_skipped"`
}
