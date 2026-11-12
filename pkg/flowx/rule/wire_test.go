package rule

import (
	"encoding/json"
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 存量编译产物直接落库、加载时不再重新编译，因此历史 rule_group JSON 必须原样可求值。
// 这里覆盖的都是真实历史形状：原子条件外层套 AND 组、OR 只挂 sub_groups、logic 缺省或小写。
func TestFromGroup_LegacyArtifacts(t *testing.T) {
	testCases := []struct {
		name string
		json string
		vars model.Variables
		want bool
	}{
		{
			name: "单原子条件",
			json: `{"logic":"AND","conditions":[{"field":"amount","op":"GT","value":1000}]}`,
			vars: model.Variables{"amount": "2000.000000"},
			want: true,
		},
		{
			name: "OR只挂子组_每个原子套一层AND",
			json: `{"logic":"OR","sub_groups":[{"logic":"AND","conditions":[{"field":"level","op":"EQ","value":"high"}]},{"logic":"AND","conditions":[{"field":"risk","op":"GTE","value":3}]}]}`,
			vars: model.Variables{"level": "low", "risk": "5.000000"},
			want: true,
		},
		{
			name: "OR只挂子组_全不命中",
			json: `{"logic":"OR","sub_groups":[{"logic":"AND","conditions":[{"field":"level","op":"EQ","value":"high"}]},{"logic":"AND","conditions":[{"field":"risk","op":"GTE","value":3}]}]}`,
			vars: model.Variables{"level": "low", "risk": "1.000000"},
			want: false,
		},
		{
			name: "本组条件与子组混排",
			json: `{"logic":"AND","conditions":[{"field":"dept","op":"EQ","value":"IT"}],"sub_groups":[{"logic":"OR","conditions":[{"field":"amount","op":"GT","value":1000},{"field":"vip","op":"EQ","value":"yes"}]}]}`,
			vars: model.Variables{"dept": "IT", "amount": "500.000000", "vip": "yes"},
			want: true,
		},
		{
			name: "logic缺省按AND",
			json: `{"conditions":[{"field":"a","op":"GT","value":1},{"field":"b","op":"GT","value":2}]}`,
			vars: model.Variables{"a": 2, "b": 1},
			want: false,
		},
		{
			name: "logic小写同样识别",
			json: `{"logic":"or","conditions":[{"field":"a","op":"GT","value":1},{"field":"b","op":"GT","value":2}]}`,
			vars: model.Variables{"a": 0, "b": 3},
			want: true,
		},
		{
			name: "集合算子承接JSON数组字符串",
			json: `{"logic":"AND","conditions":[{"field":"env","op":"IN","value":["backend","frontend"]}]}`,
			vars: model.Variables{"env": `["backend","ocr"]`},
			want: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var group model.ConditionGroup
			require.NoError(t, json.Unmarshal([]byte(tc.json), &group))
			cond := FromGroup(&group)
			require.NotNil(t, cond)
			assert.Equal(t, tc.want, cond.Eval(tc.vars))
		})
	}
}

// 新编译产物的落库形状必须仍是 {logic, conditions, sub_groups}，存量模板零迁移
func TestToGroup_JSONShape(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		want string
	}{
		{
			name: "单原子条件",
			expr: "$amount > 1000",
			want: `{"logic":"AND","conditions":[{"field":"amount","op":"GT","value":1000}]}`,
		},
		{
			name: "集合算子",
			expr: "$env in ('backend', 'frontend')",
			want: `{"logic":"AND","conditions":[{"field":"env","op":"IN","value":["backend","frontend"]}]}`,
		},
		{
			name: "与逻辑平铺进conditions",
			expr: "$a > 1 && $b > 2",
			want: `{"logic":"AND","conditions":[{"field":"a","op":"GT","value":1},{"field":"b","op":"GT","value":2}]}`,
		},
		{
			name: "或逻辑平铺进conditions",
			expr: "$a > 1 || $b > 2",
			want: `{"logic":"OR","conditions":[{"field":"a","op":"GT","value":1},{"field":"b","op":"GT","value":2}]}`,
		},
		{
			name: "嵌套分组落sub_groups",
			expr: "$a > 1 && ($b > 2 || $c > 3)",
			want: `{"logic":"AND","conditions":[{"field":"a","op":"GT","value":1}],"sub_groups":[{"logic":"OR","conditions":[{"field":"b","op":"GT","value":2},{"field":"c","op":"GT","value":3}]}]}`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(ToGroup(Parse(tc.expr)))
			require.NoError(t, err)
			assert.Equal(t, tc.want, string(raw))
		})
	}
}

// 树 → DTO → 落库 → 加载 → 树 必须判定等价且 JSON 不漂移。
// 不做 Go 类型深比对：JSON 反序列化把集合值还原成 []any，与解析期的 []string 类型不同但语义一致，
// 由 toStringSlice 统一承接，因此这里断言的是"再保存不改字节"与"求值结果不变"。
func TestGroupRoundTrip(t *testing.T) {
	inputs := []string{
		"$amount > 1000",
		"$dept == 'IT'",
		"$env in ('backend', 'frontend')",
		"$env not in ['ocr']",
		"$a > 1 && $b > 2",
		"$a > 1 || $b > 2",
		"$a > 1 && ($b > 2 || $c > 3)",
		"($amount > 1000)",
		"$remark == 'a,b'",
	}

	varsList := []model.Variables{
		{"amount": "2000.000000", "dept": "IT", "env": `["backend","ocr"]`, "a": "2", "b": "3", "c": "4", "remark": "a,b"},
		{"amount": float64(500), "dept": "HR", "env": []string{"ocr"}, "a": 0, "b": 0, "c": 0, "remark": "x"},
		{},
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			tree := Parse(input)
			require.NotNil(t, tree)

			saved, err := json.Marshal(ToGroup(tree))
			require.NoError(t, err)

			var group model.ConditionGroup
			require.NoError(t, json.Unmarshal(saved, &group))
			loaded := FromGroup(&group)
			require.NotNil(t, loaded)

			resaved, err := json.Marshal(ToGroup(loaded))
			require.NoError(t, err)
			assert.Equal(t, string(saved), string(resaved), "加载后再保存不得改变落库字节")

			for _, vars := range varsList {
				assert.Equal(t, tree.Eval(vars), loaded.Eval(vars), "vars=%v", vars)
			}
		})
	}
}

// DTO 表达不了的树必须整体返回 nil，由 EdgeSpec.Condition 原文承载，绝不静默丢掉条件
func TestToGroup_ExprTreeReturnsNil(t *testing.T) {
	testCases := []struct {
		name string
		cond Condition
	}{
		{"纯expr节点", Expr{Text: "amount * 2 > 100"}},
		{"算术表达式解析结果", Parse("$amount * 2 > 100")},
		{"与逻辑夹带expr", And{Compare{Field: "a", Op: model.OpGreaterThan, Value: float64(1)}, Expr{Text: "len(tags) > 1"}}},
		{"或逻辑夹带expr", Or{Compare{Field: "a", Op: model.OpGreaterThan, Value: float64(1)}, Expr{Text: "len(tags) > 1"}}},
		{"nil树", nil},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Nil(t, ToGroup(tc.cond))
		})
	}
}
