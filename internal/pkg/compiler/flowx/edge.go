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

		rawExpr := strings.TrimSpace(e.Expression)
		if rawExpr != "" && rawExpr != "1 = 1" && rawExpr != "true" {
			// 采用规则引擎统一的规范化工具清洗表达式（消除前缀 $、转义单等号、规范化集合括号等）
			cleanExpr := rule.NormalizeExpression(rawExpr)
			edgeSpec.Condition = cleanExpr
			edgeSpec.RuleGroup = rule.ParseConditionText(rawExpr)
		}

		return edgeSpec, true
	})
}
