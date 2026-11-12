package rule

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
)

func TestEval_AllOperators(t *testing.T) {
	vars := model.Variables{
		"age":       28,
		"city":      "Hangzhou",
		"roles":     []string{"admin", "dev"},
		"comment":   "approved by manager",
		"empty_str": "",
	}

	assert.True(t, FromGroup(&model.ConditionGroup{
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
	}).Eval(vars))
}

func TestEval_NestedSubGroups(t *testing.T) {
	vars := model.Variables{
		"amount": 6000,
		"dept":   "IT",
		"role":   "engineer",
	}

	// (amount > 5000 AND dept == 'IT') OR (role == 'director')
	group := &model.ConditionGroup{
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

	assert.True(t, FromGroup(group).Eval(vars))
}

func TestEval_TypeCoercion(t *testing.T) {
	// 验证 JSON 反序列化后常见的 float64 与 int 跨类型数值等值比较
	vars := model.Variables{
		"amount_float": float64(100),
		"count_uint":   uint(5),
		"str_num":      "50",
	}

	group := &model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: "amount_float", Operator: model.OpEqual, Value: 100},   // float64 vs int
			{Field: "amount_float", Operator: model.OpNotEqual, Value: 99}, // float64 vs int
			{Field: "count_uint", Operator: model.OpEqual, Value: int64(5)},
			{Field: "str_num", Operator: model.OpGreaterThan, Value: 20},
		},
	}

	assert.True(t, FromGroup(group).Eval(vars))
}

func TestEval_InWithSliceAndJSON(t *testing.T) {
	// 场景 1: 表单变量为 JSON 数组字符串
	varsJSON := model.Variables{
		"environment": `["ocr","frontend","backend"]`,
	}
	// 命中 backend
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"backend"}},
		},
	}).Eval(varsJSON))
	// 命中 frontend
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"frontend"}},
		},
	}).Eval(varsJSON))
	// 未命中 java
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"java"}},
		},
	}).Eval(varsJSON))

	// 场景 2: 表单变量为原生切片 []string
	varsSlice := model.Variables{
		"environment": []string{"ocr", "frontend", "backend"},
	}
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"backend"}},
		},
	}).Eval(varsSlice))
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "environment", Operator: model.OpIn, Value: []string{"java"}},
		},
	}).Eval(varsSlice))
}

func TestEval_SliceAdaptation(t *testing.T) {
	vars := model.Variables{
		"env_slice":      []any{"frontend", "backend"},
		"env_empty":      []any{},
		"env_json_empty": "[]",
		"env_scalar":     "backend",
	}

	// 1. 等于 (OpEqual): 切片中包含即为 true
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpEqual, Value: "backend"},
		},
	}).Eval(vars))
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpEqual, Value: "frontend"},
		},
	}).Eval(vars))
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpEqual, Value: "ocr"},
		},
	}).Eval(vars))

	// 2. 不等于 (OpNotEqual): 切片中不包含才为 true
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpNotEqual, Value: "ocr"},
		},
	}).Eval(vars))
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpNotEqual, Value: "backend"},
		},
	}).Eval(vars))

	// 3. 包含 (OpContains): 元素匹配
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpContains, Value: "backend"},
		},
	}).Eval(vars))
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpContains, Value: "ocr"},
		},
	}).Eval(vars))

	// 4. 为空 (OpEmpty): 空切片被识别为空
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_empty", Operator: model.OpEmpty, Value: nil},
		},
	}).Eval(vars))
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_json_empty", Operator: model.OpEmpty, Value: nil},
		},
	}).Eval(vars))
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpEmpty, Value: nil},
		},
	}).Eval(vars))

	// 5. 非空 (OpNotEmpty):
	assert.True(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_slice", Operator: model.OpNotEmpty, Value: nil},
		},
	}).Eval(vars))
	assert.False(t, FromGroup(&model.ConditionGroup{
		Conditions: []model.Condition{
			{Field: "env_empty", Operator: model.OpNotEmpty, Value: nil},
		},
	}).Eval(vars))
}

func TestEval_UnknownOperatorNeverPasses(t *testing.T) {
	// op 来自持久化 JSON，存量模板可能带本版本不认识的算子，必须判不通过而不是默认放行
	testCases := []struct {
		name string
		op   model.Op
	}{
		{"未定义算子", model.Op("BOGUS")},
		{"空算子", model.Op("")},
		{"大小写不匹配已知算子", model.Op("eq")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			group := &model.ConditionGroup{
				Conditions: []model.Condition{
					{Field: "dept", Operator: tc.op, Value: "IT"},
				},
			}
			assert.False(t, FromGroup(group).Eval(model.Variables{"dept": "IT"}))
		})
	}
}

// 空树表示无条件约束，调用方按放行处理；FromGroup 对空组必须返回 nil 而不是空 And
func TestEval_NilMeansUnconditional(t *testing.T) {
	assert.Nil(t, FromGroup(nil))
	assert.Nil(t, FromGroup(&model.ConditionGroup{}))
	assert.Nil(t, FromGroup(&model.ConditionGroup{Logic: "AND"}))
	assert.True(t, And(nil).Eval(model.Variables{}))
	assert.False(t, Or(nil).Eval(model.Variables{}))
}
