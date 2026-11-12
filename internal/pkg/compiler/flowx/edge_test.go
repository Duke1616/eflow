package flowx

import (
	"testing"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 编译边界回归：画布常见条件都必须编译出规则组，否则运行期退回强类型 expr，
// 而 SYSTEM 渠道的表单数值是 "%f" 字符串、多选是 JSON 数组字符串，expr 会报错或静默判 false
func TestConvertEdges_CompilesRuleGroup(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		vars model.Variables
		want bool
	}{
		{
			name: "数值大于",
			expr: "$amount > 1000",
			vars: model.Variables{"amount": "2000.000000"},
			want: true,
		},
		{
			name: "数值等于",
			expr: "$amount == 100",
			vars: model.Variables{"amount": "100.000000"},
			want: true,
		},
		{
			name: "多选集合包含",
			expr: "$env in ('backend', 'frontend')",
			vars: model.Variables{"env": `["backend","ocr"]`},
			want: true,
		},
		{
			name: "多选等值",
			expr: "$env == 'backend'",
			vars: model.Variables{"env": `["backend","ocr"]`},
			want: true,
		},
		{
			name: "复合与逻辑",
			expr: "$amount > 1000 && $dept == 'IT'",
			vars: model.Variables{"amount": "2000.000000", "dept": "IT"},
			want: true,
		},
		{
			name: "复合或逻辑",
			expr: "$env in ('backend') || $risk >= 3",
			vars: model.Variables{"env": `["backend","ocr"]`, "risk": "1.000000"},
			want: true,
		},
		{
			name: "集合不命中",
			expr: "$env in ('java')",
			vars: model.Variables{"env": `["backend","ocr"]`},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			specs := ConvertEdges([]flowdsl.Edge{{Source: "a", Target: "b", Expression: tc.expr}})
			require.Len(t, specs, 1)
			require.NotNil(t, specs[0].RuleGroup, "必须编译出规则组，不能退回 expr")
			assert.Equal(t, tc.want, rule.FromGroup(specs[0].RuleGroup).Eval(tc.vars))
		})
	}
}

func TestConvertEdges_SkipsUnconditionalExpression(t *testing.T) {
	testCases := []struct {
		name string
		expr string
	}{
		{"空表达式", ""},
		{"恒真占位", "1 = 1"},
		{"恒真字面量", "true"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			specs := ConvertEdges([]flowdsl.Edge{{Source: "a", Target: "b", Expression: tc.expr}})
			require.Len(t, specs, 1)
			assert.Nil(t, specs[0].RuleGroup)
			assert.Empty(t, specs[0].Condition)
		})
	}
}

func TestConvertEdges_DropsIncompleteEdge(t *testing.T) {
	specs := ConvertEdges([]flowdsl.Edge{
		{Source: "", Target: "b", Expression: "$a > 1"},
		{Source: "a", Target: "", Expression: "$a > 1"},
		{Source: "a", Target: "b", Expression: "$a > 1"},
	})

	require.Len(t, specs, 1)
	assert.Equal(t, "a", specs[0].Source)
	assert.Equal(t, "b", specs[0].Target)
}
