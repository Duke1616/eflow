package rule

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
)

func TestEvaluateRuleGroup_AllOperators(t *testing.T) {
	vars := map[string]any{
		"age":       28,
		"city":      "Hangzhou",
		"roles":     []string{"admin", "dev"},
		"comment":   "approved by manager",
		"empty_str": "",
	}

	assert.True(t, EvaluateRuleGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "age", Operator: model.OpEqual, Value: 28},
			{Field: "age", Operator: model.OpGreaterThan, Value: 20},
			{Field: "age", Operator: model.OpGreaterEqual, Value: 28},
			{Field: "age", Operator: model.OpLessThan, Value: 30},
			{Field: "age", Operator: model.OpLessEqual, Value: 28},
			{Field: "city", Operator: model.OpIn, Value: []string{"Beijing", "Hangzhou"}},
			{Field: "city", Operator: model.OpNotIn, Value: []string{"Shanghai", "Shenzhen"}},
			{Field: "comment", Operator: model.OpContains, Value: "manager"},
			{Field: "empty_str", Operator: model.OpEmpty, Value: nil},
			{Field: "city", Operator: model.OpNotEmpty, Value: nil},
		},
	}, vars))
}

func TestEvaluateRuleGroup_NestedSubGroups(t *testing.T) {
	vars := map[string]any{
		"amount": 6000,
		"dept":   "IT",
		"role":   "engineer",
	}

	// (amount > 5000 AND dept == 'IT') OR (role == 'director')
	rule := &model.ConditionGroup{
		Logic: "OR",
		SubGroups: []model.ConditionGroup{
			{
				Logic: "AND",
				Conditions: []model.Condition{
					{Field: "amount", Operator: model.OpGreaterThan, Value: 5000},
					{Field: "dept", Operator: model.OpEqual, Value: "IT"},
				},
			},
			{
				Logic: "AND",
				Conditions: []model.Condition{
					{Field: "role", Operator: model.OpEqual, Value: "director"},
				},
			},
		},
	}

	assert.True(t, EvaluateRuleGroup(rule, vars))
}

func TestEvaluateRuleGroup_TypeCoercion(t *testing.T) {
	// 验证 JSON 反序列化后常见的 float64 与 int 跨类型数值等值比较
	vars := map[string]any{
		"amount_float": float64(100),
		"count_uint":   uint(5),
		"str_num":      "50",
	}

	rule := &model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: "amount_float", Operator: model.OpEqual, Value: 100},   // float64 vs int
			{Field: "amount_float", Operator: model.OpNotEqual, Value: 99}, // float64 vs int
			{Field: "count_uint", Operator: model.OpEqual, Value: int64(5)},
			{Field: "str_num", Operator: model.OpGreaterThan, Value: 20},
		},
	}

	assert.True(t, EvaluateRuleGroup(rule, vars))
}

func TestEvaluateRuleGroup_InWithSliceAndJSON(t *testing.T) {
	// 场景 1: 表单变量为 JSON 数组字符串
	varsJSON := map[string]any{
		"environment": `["ocr","frontend","backend"]`,
	}
	// 命中 backend
	assert.True(t, EvaluateRuleGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"backend"}},
		},
	}, varsJSON))
	// 命中 frontend
	assert.True(t, EvaluateRuleGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"frontend"}},
		},
	}, varsJSON))
	// 未命中 java
	assert.False(t, EvaluateRuleGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"java"}},
		},
	}, varsJSON))

	// 场景 2: 表单变量为原生切片 []string
	varsSlice := map[string]any{
		"environment": []string{"ocr", "frontend", "backend"},
	}
	assert.True(t, EvaluateRuleGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"backend"}},
		},
	}, varsSlice))
	assert.False(t, EvaluateRuleGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"java"}},
		},
	}, varsSlice))
}
