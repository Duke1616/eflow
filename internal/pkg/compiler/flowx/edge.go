package flowx

import (
	"strings"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
	"github.com/samber/lo"
)

// ConvertEdges 将标准图连线集合转换为 flowx 强类型 EdgeSpec 集合
func ConvertEdges(edges []flowdsl.Edge) []model.EdgeSpec {
	return lo.FilterMap(edges, func(e flowdsl.Edge, _ int) (model.EdgeSpec, bool) {
		if e.Source == "" || e.Target == "" {
			return model.EdgeSpec{}, false
		}

		edgeSpec := model.EdgeSpec{
			Source: e.Source,
			Target: e.Target,
		}

		if cleanExpr := sanitizeExpression(e.Expression); cleanExpr != "" {
			edgeSpec.Condition = cleanExpr
			edgeSpec.RuleGroup = rule.ParseConditionText(cleanExpr)
		}

		return edgeSpec, true
	})
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
