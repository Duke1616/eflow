package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// evalExpr 只接受已规范化的文本（由 toExprText 产出），承接条件语法之外的算术与函数调用
func TestEvalExpr(t *testing.T) {
	testCases := []struct {
		name       string
		expression string
		env        map[string]any
		wantResult bool
		wantErr    bool
	}{
		{
			name:       "算术运算",
			expression: "amount * 2 > 100",
			env:        map[string]any{"amount": 100},
			wantResult: true,
		},
		{
			name:       "内置函数",
			expression: "len(tags) > 1",
			env:        map[string]any{"tags": []string{"a", "b"}},
			wantResult: true,
		},
		{
			name:       "复合逻辑与或运算",
			expression: "(days > 3 && dept == 'HR') || role == 'admin'",
			env:        map[string]any{"days": 2, "dept": "HR", "role": "admin"},
			wantResult: true,
		},
		{
			name:       "集合包含",
			expression: "environment in ['backend', 'frontend']",
			env:        map[string]any{"environment": "backend"},
			wantResult: true,
		},
		{
			name:       "字面量内单等号原样参与比较",
			expression: "remark == 'a=b'",
			env:        map[string]any{"remark": "a=b"},
			wantResult: true,
		},
		{
			name:       "空文本放行",
			expression: "",
			env:        nil,
			wantResult: true,
		},
		{
			name:       "强类型不匹配必须报错而不是静默放行",
			expression: "days > 3",
			env:        map[string]any{"days": "5.000000"},
			wantErr:    true,
		},
		{
			name:       "非布尔结果必须报错",
			expression: "amount + 1",
			env:        map[string]any{"amount": 1},
			wantErr:    true,
		},
		{
			name:       "语法错误必须报错",
			expression: "a ===",
			env:        map[string]any{"a": 1},
			wantErr:    true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := evalExpr(tc.expression, tc.env)
			if tc.wantErr {
				require.Error(t, err)
				assert.False(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantResult, got)
		})
	}
}

// 同一表达式同一字段集合换值，不得串用上一次的结果
func TestEvalExpr_CacheIsPerExpression(t *testing.T) {
	got, err := evalExpr("amount > 100", map[string]any{"amount": 200})
	require.NoError(t, err)
	assert.True(t, got)

	got, err = evalExpr("amount > 100", map[string]any{"amount": 50})
	require.NoError(t, err)
	assert.False(t, got)
}

// 缓存键必须覆盖 env 的字段名集合：expr.Env 把字段集合吃进编译期，只按文本缓存会让
// 齐全 env 编出的程序被缺字段 env 复用，nil == nil 反判 true 造成误放行。
// 每个子用例用独立文本，保证对该文本而言缓存是冷的，先后顺序不影响裁决
func TestEvalExpr_CacheKeyCoversFieldSet(t *testing.T) {
	testCases := []struct {
		name string
		text string
		warm map[string]any
		then map[string]any
		want bool
	}{
		{
			name: "齐全先编译_缺字段后求值不得误放行",
			text: "fk_a == fk_b",
			warm: map[string]any{"fk_a": 1, "fk_b": 1},
			then: map[string]any{"fk_other": 1},
			want: false,
		},
		{
			name: "缺字段先编译_齐全后求值仍须正确",
			text: "fk_c == fk_d",
			warm: map[string]any{"fk_other": 1},
			then: map[string]any{"fk_c": 1, "fk_d": 1},
			want: true,
		},
		{
			name: "否定型缺字段不得误放行",
			text: "fk_e != fk_f",
			warm: map[string]any{"fk_e": 1, "fk_f": 1},
			then: map[string]any{"fk_other": 1},
			want: false,
		},
		{
			name: "字段集合相同而类型不同仍按当前值裁决",
			text: "fk_g > 100",
			warm: map[string]any{"fk_g": 200.0},
			then: map[string]any{"fk_g": 50.0},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _ = evalExpr(tc.text, tc.warm)
			got, _ := evalExpr(tc.text, tc.then)
			assert.Equal(t, tc.want, got)
		})
	}
}
