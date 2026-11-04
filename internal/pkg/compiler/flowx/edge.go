package flowx

import (
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
)

// ConvertEdges 将 LogicFlow 连线集合转换为 flowx 强类型 EdgeSpec 集合
func ConvertEdges(rawEdges []Edge) []model.EdgeSpec {
	result := make([]model.EdgeSpec, 0, len(rawEdges))

	for _, raw := range rawEdges {
		if raw.SourceNodeId == "" || raw.TargetNodeId == "" {
			continue
		}

		edgeSpec := model.EdgeSpec{
			Source: raw.SourceNodeId,
			Target: raw.TargetNodeId,
		}

		// 提取并清洗连线上的跳转表达式
		expr := extractExpression(raw.Properties)
		if expr != "" {
			cleanExpr := sanitizeExpression(expr)
			if cleanExpr != "" {
				edgeSpec.Condition = cleanExpr
				edgeSpec.RuleGroup = rule.ParseConditionText(cleanExpr)
			}
		}

		result = append(result, edgeSpec)
	}

	return result
}

// extractExpression 尝试从不同结构的属性中提取条件表达式
func extractExpression(raw any) string {
	if raw == nil {
		return ""
	}

	// 1. 若已经是 map 结构
	if m, ok := raw.(map[string]any); ok {
		if expr, ok := m["expression"].(string); ok && strings.TrimSpace(expr) != "" {
			return strings.TrimSpace(expr)
		}
	}

	// 2. 尝试反序列化为 EdgeProperty
	prop, err := DecodeProperty[EdgeProperty](raw)
	if err == nil && strings.TrimSpace(prop.Expression) != "" {
		return strings.TrimSpace(prop.Expression)
	}

	return ""
}

// sanitizeExpression 清洗条件表达式中的多余前缀 (例如将 "$days > 3" 转换为 "days > 3")
func sanitizeExpression(expr string) string {
	expr = strings.TrimSpace(expr)
	if expr == "" || expr == "1 = 1" || expr == "true" {
		return ""
	}

	// 去除首个变量名可能携带的 '$' 前缀
	if strings.HasPrefix(expr, "$") {
		expr = strings.TrimPrefix(expr, "$")
	}

	return strings.TrimSpace(expr)
}
