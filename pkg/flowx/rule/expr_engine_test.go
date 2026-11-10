package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExprEngine_Evaluate(t *testing.T) {
	engine := GetDefaultEngine()

	testCases := []struct {
		name       string
		expression string
		env        map[string]any
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "数值比较大于",
			expression: "$days > 3",
			env:        map[string]any{"days": 5},
			wantResult: true,
		},
		{
			name:       "字符串单等号兼容",
			expression: "$dept = 'IT'",
			env:        map[string]any{"dept": "IT"},
			wantResult: true,
		},
		{
			name:       "SQL风格集合包含in",
			expression: "$environment in ('backend', 'frontend')",
			env:        map[string]any{"environment": "backend"},
			wantResult: true,
		},
		{
			name:       "SQL风格集合包含not_in",
			expression: "$environment not in ('ocr', 'algo')",
			env:        map[string]any{"environment": "backend"},
			wantResult: true,
		},
		{
			name:       "复合逻辑与或运算",
			expression: "($days > 3 && $dept == 'HR') || $role == 'admin'",
			env:        map[string]any{"days": 2, "dept": "HR", "role": "admin"},
			wantResult: true,
		},
		{
			name:       "标准分支条件",
			expression: "$level in ('high') || $risk >= 3",
			env:        map[string]any{"level": "high", "risk": 3},
			wantResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := engine.Evaluate(tc.expression, tc.env)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)
		})
	}
}
