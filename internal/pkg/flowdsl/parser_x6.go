package flowdsl

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/samber/lo"
)

type x6Parser struct{}

func (p *x6Parser) Match(payload []byte) bool {
	return bytes.Contains(payload, []byte(`"cells"`))
}

func (p *x6Parser) Parse(payload []byte) (IGraph, error) {
	var raw rawX6Graph
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("反序列化 AntV X6 原始数据失败: %w", err)
	}

	graph := NewGraph("x6")

	// 提前分支，彻底去除 if-else 嵌套
	for _, cell := range raw.Cells {
		if cell.isEdge() {
			graph.AddEdge(cell.toEdge())
			continue
		}
		graph.AddNode(cell.toNode())
	}

	graph.BuildIndex()
	return graph, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// AntV X6 专属反序列化结构
// ─────────────────────────────────────────────────────────────────────────────

type rawX6Graph struct {
	Cells []rawX6Cell `json:"cells"`
}

type rawX6Cell struct {
	ID         string          `json:"id"`
	Shape      string          `json:"shape"`
	Label      string          `json:"label"`
	Source     any             `json:"source"`
	Target     any             `json:"target"`
	Data       json.RawMessage `json:"data"`
	Properties json.RawMessage `json:"properties"`
}

func (c *rawX6Cell) isEdge() bool {
	return c.Shape == "edge" || (c.Source != nil && c.Target != nil)
}

func (c *rawX6Cell) toNode() *Node {
	node := &Node{
		ID:   c.ID,
		Name: c.Label,
		Type: NormalizeNodeType(c.Shape),
	}

	payload := lo.Ternary(len(c.Data) > 0, c.Data, c.Properties)
	decodeNodePayload(payload, node)
	return node
}

func (c *rawX6Cell) toEdge() Edge {
	edge := Edge{
		ID:     c.ID,
		Source: extractEndpointID(c.Source),
		Target: extractEndpointID(c.Target),
	}

	payload := lo.Ternary(len(c.Data) > 0, c.Data, c.Properties)
	if len(payload) > 0 {
		var prop rawX6EdgeData
		if err := json.Unmarshal(payload, &prop); err == nil {
			edge.Expression = prop.Expression
			edge.IsPass = prop.IsPass
			edge.IsSkipped = prop.IsSkipped
		}
	}
	return edge
}

type rawX6EdgeData struct {
	Expression string `json:"expression"`
	IsPass     bool   `json:"is_pass"`
	IsSkipped  bool   `json:"is_skipped"`
}

func extractEndpointID(endpoint any) string {
	switch v := endpoint.(type) {
	case string:
		return v
	case map[string]any:
		if cellID, ok := v["cell"].(string); ok {
			return cellID
		}
	}
	return ""
}
