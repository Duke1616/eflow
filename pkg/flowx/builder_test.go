package flowx

import (
	"encoding/json"
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/rule"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// conditionProcess 造一个最小合法流程：开始 -> 排他网关 -> 结束，网关出边由调用方装配
func conditionProcess(edge func(b *ProcessBuilder) *ProcessBuilder) *ProcessBuilder {
	b := NewProcess("k", "n").
		Start("s", "开始").
		ExclusiveGateway("g", "网关").
		End("e", "结束").
		Edge("s", "g")
	return edge(b)
}

// RuleEdge 不承载条件原文，落库 DTO 表达不了的树必须在构建期中断。
// 静默落成 RuleGroup=nil 会让运行期把这条边当成无条件放行——条件全假也照样流转。
func TestRuleEdge_UnpersistableTreeFailsFast(t *testing.T) {
	cmp := rule.Compare{Field: "amount", Op: model.OpGreaterThan, Value: 100.0}
	testCases := []struct {
		name    string
		cond    rule.Condition
		wantErr bool
	}{
		{"纯 Compare 可落库", cmp, false},
		{"And 全 Compare 可落库", rule.And{cmp, cmp}, false},
		{"Or 全 Compare 可落库", rule.Or{cmp, cmp}, false},
		{"嵌套全 Compare 可落库", rule.And{rule.Or{cmp, cmp}, cmp}, false},
		{"nil 表示无条件约束", nil, false},
		{"纯 Expr 无法落库", rule.Expr{Text: "amount * 2 > 100"}, true},
		{"And 夹带 Expr 无法落库", rule.And{cmp, rule.Expr{Text: "len(tags) > 1"}}, true},
		{"Or 夹带 Expr 无法落库", rule.Or{cmp, rule.Expr{Text: "len(tags) > 1"}}, true},
		{"深层嵌套夹带 Expr 无法落库", rule.And{rule.Or{cmp, rule.Expr{Text: "amount < 0"}}}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
				return b.RuleEdge("g", "e", tc.cond)
			})
			if tc.wantErr {
				require.Error(t, b.Validate(), "无法落库的条件树必须在构建期暴露")
				assert.Panics(t, func() { b.Build() }, "Build 必须 Fail-Fast，不得产出静默放行的定义")
				return
			}
			require.NoError(t, b.Validate())
			assert.NotPanics(t, func() { b.Build() })
		})
	}
}

// 对照：ConditionEdge 以原文承载 Expr 兜底，条件为假时必须真的拦下，不能退化成无条件放行
func TestConditionEdge_ExprFallbackStillBlocks(t *testing.T) {
	def := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
		return b.ConditionEdge("g", "e", "$amount * 2 > 100")
	}).Build()

	require.Empty(t, def.Edges[1].RuleGroup, "Expr 兜底无法映射成 DTO，应由 Condition 原文承载")
	require.NotEmpty(t, def.Edges[1].Condition)

	plan := kernel.NewExecutionPlan(def)
	vars := model.Variables{"amount": 1}
	res, err := plan.CalculateTransition("g", vars)
	require.NoError(t, err)
	assert.Empty(t, res.Targets, "条件为假时不得流向结束节点")

	res, err = plan.CalculateTransition("g", model.Variables{"amount": 200})
	require.NoError(t, err)
	require.Len(t, res.Targets, 1)
	assert.Equal(t, "e", res.Targets[0].ID)
}

// 链式构建期只保留首个异常，后续错误不得覆盖根因
func TestRuleEdge_KeepsFirstError(t *testing.T) {
	b := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
		return b.Hook("不存在的节点", "h").
			RuleEdge("g", "e", rule.Expr{Text: "amount > 1"})
	})
	err := b.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "不存在的节点")
}

// transitionPassed 走真实运行期边界判定网关是否放行到结束节点
func transitionPassed(t *testing.T, def model.ProcessDefinition, vars model.Variables) bool {
	t.Helper()
	res, err := kernel.NewExecutionPlan(def).CalculateTransition("g", vars)
	require.NoError(t, err)
	return len(res.Targets) > 0
}

// 标准模式落库后 RuleGroup 必为 nil，加载时若用 rule.Parse 重编译就会套回方言与宽松比较，
// 严格语义在 Deploy→load 往返后静默翻转。这条防线锁住往返前后裁决必须一致。
func TestStandardEdge_RoundTripKeepsStrictSemantics(t *testing.T) {
	native := model.Variables{"amount": 200.0, "env": "backend", "tags": []string{"a", "b"}}
	stringified := model.Variables{"amount": "200.000000", "env": `["backend","ocr"]`, "tags": []string{"a", "b"}}
	testCases := []struct {
		name       string
		expression string
		vars       model.Variables
		want       bool
	}{
		{"原生数值比较成立", "amount > 100", native, true},
		{"数值字符串严格模式不转数值", "amount > 100", stringified, false},
		{"原生集合包含成立", "env in ['backend']", native, true},
		{"JSON数组字符串严格模式不还原集合", "env in ['backend']", stringified, false},
		{"等值比较成立", "env == 'backend'", native, true},
		{"等值比较不成立", "env == 'backend'", stringified, false},
		{"算术运算成立", "amount * 2 > 100", native, true},
		{"算术运算遇字符串报错判不通过", "amount * 2 > 100", stringified, false},
		{"内置函数成立", "len(tags) > 1", native, true},
		{"内置函数不成立", "len(tags) > 3", native, false},
		{"复合逻辑成立", "amount >= 100 && amount <= 200", native, true},
		{"复合逻辑遇字符串判不通过", "amount >= 100 && amount <= 200", stringified, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
				return b.StandardEdge("g", "e", tc.expression)
			}).Build()
			require.True(t, def.Edges[1].Standard)
			require.Nil(t, def.Edges[1].RuleGroup, "标准模式无法用 DTO 表达，只能由 Condition 原文承载")

			blob, err := json.Marshal(def)
			require.NoError(t, err)
			var loaded model.ProcessDefinition
			require.NoError(t, json.Unmarshal(blob, &loaded))

			assert.Equal(t, tc.want, transitionPassed(t, def, tc.vars), "部署前裁决必须符合严格语义")
			assert.Equal(t, tc.want, transitionPassed(t, loaded, tc.vars), "落库加载后裁决不得翻转")
		})
	}
}

// 标准模式不做任何方言适配：老写法一律不认，但 expr 原生的区间与变量集合因此不再被改写破坏
func TestStandardEdge_NoDialectAdaptation(t *testing.T) {
	testCases := []struct {
		name       string
		expression string
		vars       model.Variables
		want       bool
	}{
		{"不剥 $ 前缀", "$amount > 100", model.Variables{"amount": 200.0}, false},
		{"不补单等号", "amount = 200", model.Variables{"amount": 200.0}, false},
		{"不归一大写算子", "env IN ['backend']", model.Variables{"env": "backend"}, false},
		{"区间语法不被改成数组", "amount in (1..5)", model.Variables{"amount": 3.0}, true},
		{"区间语法不命中", "amount in (1..5)", model.Variables{"amount": 200.0}, false},
		{"变量集合不被当成字面量", "env in (envList)", model.Variables{
			"env": "backend", "envList": []string{"backend", "ocr"}}, true},
		{"空文本无条件放行", "", model.Variables{}, true},
		{"纯空白无条件放行", "   ", model.Variables{}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
				return b.StandardEdge("g", "e", tc.expression)
			}).Build()
			assert.Equal(t, tc.want, transitionPassed(t, def, tc.vars))
		})
	}
}

// 同一条文本在两种模式下语义必须不同且各自稳定：老路宽松兼容存量，新路严格可预期
func TestConditionEdgeVsStandardEdge_Semantics(t *testing.T) {
	vars := model.Variables{"amount": "200.000000"}
	legacy := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
		return b.ConditionEdge("g", "e", "$amount > 100")
	}).Build()
	standard := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
		return b.StandardEdge("g", "e", "amount > 100")
	}).Build()

	assert.True(t, transitionPassed(t, legacy, vars), "老方言把数值字符串转成数值再比")
	assert.False(t, transitionPassed(t, standard, vars), "标准模式严格类型，字符串与数值比较报错判不通过")
}

// 存量兼容：老路边序列化不得多出 standard 字段，无该字段的历史 JSON 必须仍走宽松方言
func TestStandardEdge_LegacyWireShapeUnchanged(t *testing.T) {
	legacy := conditionProcess(func(b *ProcessBuilder) *ProcessBuilder {
		return b.ConditionEdge("g", "e", "$amount > 100")
	}).Build()
	blob, err := json.Marshal(legacy.Edges[1])
	require.NoError(t, err)
	assert.NotContains(t, string(blob), "standard", "omitempty 必须保证存量边字节形状不变")

	var old model.EdgeSpec
	require.NoError(t, json.Unmarshal([]byte(`{"source":"g","target":"e","condition":"$amount > 100"}`), &old))
	require.False(t, old.Standard, "缺字段即老方言")

	def := model.ProcessDefinition{
		Key: "k", Name: "n", Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "s", Type: model.NodeStart},
			{ID: "g", Type: model.NodeGatewayExclusive},
			{ID: "e", Type: model.NodeEnd},
		},
		Edges: []model.EdgeSpec{{Source: "s", Target: "g"}, old},
	}
	assert.True(t, transitionPassed(t, def, model.Variables{"amount": "200.000000"}),
		"历史 JSON 加载后必须仍走宽松比较，不得被新字段改变裁决")
}
