package rule

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConditionText(t *testing.T) {
	testCases := []struct {
		name       string
		input      string
		wantField  string
		wantOp     model.Op
		wantVal    any
		wantNil    bool
	}{
		{
			name:      "大于数值",
			input:     "days > 3",
			wantField: "days",
			wantOp:    model.OpGreaterThan,
			wantVal:   float64(3),
		},
		{
			name:      "大于等于数值",
			input:     "score >= 60",
			wantField: "score",
			wantOp:    model.OpGreaterEqual,
			wantVal:   float64(60),
		},
		{
			name:      "等于单引号字符串",
			input:     "dept == 'IT'",
			wantField: "dept",
			wantOp:    model.OpEqual,
			wantVal:   "IT",
		},
		{
			name:      "不等于双引号字符串",
			input:     `status != "REJECT"`,
			wantField: "status",
			wantOp:    model.OpNotEqual,
			wantVal:   "REJECT",
		},
		{
			name:      "小于等于数值",
			input:     "amount <= 1000.5",
			wantField: "amount",
			wantOp:    model.OpLessEqual,
			wantVal:   1000.5,
		},
		{
			name:      "包含算子in单值",
			input:     "environment in ('backend')",
			wantField: "environment",
			wantOp:    model.OpIn,
			wantVal:   []string{"backend"},
		},
		{
			name:      "包含算子in多值带美元符号",
			input:     "$environment in ('ocr', 'frontend')",
			wantField: "environment",
			wantOp:    model.OpIn,
			wantVal:   []string{"ocr", "frontend"},
		},
		{
			name:      "不包含算子not in",
			input:     "env not in ('test', 'dev')",
			wantField: "env",
			wantOp:    model.OpNotIn,
			wantVal:   []string{"test", "dev"},
		},
		{
			name:    "空字符串返回nil",
			input:   "   ",
			wantNil: true,
		},
		{
			name:    "无操作符返回nil",
			input:   "invalid_syntax",
			wantNil: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			res := ParseConditionText(tc.input)
			if tc.wantNil {
				assert.Nil(t, res)
				return
			}
			require.NotNil(t, res)
			assert.Equal(t, "AND", res.Logic)
			require.Len(t, res.Conditions, 1)
			assert.Equal(t, tc.wantField, res.Conditions[0].Field)
			assert.Equal(t, tc.wantOp, res.Conditions[0].Operator)
			assert.Equal(t, tc.wantVal, res.Conditions[0].Value)
		})
	}
}
