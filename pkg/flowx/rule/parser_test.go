package rule

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParse(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  Condition
	}{
		{"大于数值", "days > 3", Compare{Field: "days", Op: model.OpGreaterThan, Value: float64(3)}},
		{"大于等于数值", "score >= 60", Compare{Field: "score", Op: model.OpGreaterEqual, Value: float64(60)}},
		{"等于单引号字符串", "dept == 'IT'", Compare{Field: "dept", Op: model.OpEqual, Value: "IT"}},
		{"不等于双引号字符串", `status != "REJECT"`, Compare{Field: "status", Op: model.OpNotEqual, Value: "REJECT"}},
		{"小于等于数值", "amount <= 1000.5", Compare{Field: "amount", Op: model.OpLessEqual, Value: 1000.5}},
		{"负数右值", "amount > -3", Compare{Field: "amount", Op: model.OpGreaterThan, Value: float64(-3)}},
		{"单等号兼容", "level = 'high'", Compare{Field: "level", Op: model.OpEqual, Value: "high"}},
		{"中文字段名", "金额 > 100", Compare{Field: "金额", Op: model.OpGreaterThan, Value: float64(100)}},
		{"集合算子in单值", "environment in ('backend')", Compare{Field: "environment", Op: model.OpIn, Value: []string{"backend"}}},
		{"集合算子in多值带美元符号", "$environment in ('ocr', 'frontend')", Compare{Field: "environment", Op: model.OpIn, Value: []string{"ocr", "frontend"}}},
		{"集合算子not in", "env not in ('test', 'dev')", Compare{Field: "env", Op: model.OpNotIn, Value: []string{"test", "dev"}}},
		{"字面量内in不得被当作集合算子", "note == 'a in b'", Compare{Field: "note", Op: model.OpEqual, Value: "a in b"}},
		{"字面量内not in不得被当作集合算子", `note == "a not in b"`, Compare{Field: "note", Op: model.OpEqual, Value: "a not in b"}},
		{"字面量内比较符不得被当作算子", "note = 'a>b'", Compare{Field: "note", Op: model.OpEqual, Value: "a>b"}},
		{"字面量内单等号原样保留", "remark == 'a=b'", Compare{Field: "remark", Op: model.OpEqual, Value: "a=b"}},
		{"字面量整体为带空格的in关键字", "flag == ' IN '", Compare{Field: "flag", Op: model.OpEqual, Value: " IN "}},
		{"字面量含逗号时等值解析不受影响", "remark == 'a,b'", Compare{Field: "remark", Op: model.OpEqual, Value: "a,b"}},
		{"转义引号必须还原成字面量本身", `remark == 'it\'s a=b'`, Compare{Field: "remark", Op: model.OpEqual, Value: "it's a=b"}},
		{"集合元素保留原文不做数值转换", "$env in ('1.50', '2')", Compare{Field: "env", Op: model.OpIn, Value: []string{"1.50", "2"}}},
		{"空文本表示无条件放行", "   ", nil},
		{"语法覆盖不了退回expr", "invalid_syntax", Expr{Text: "invalid_syntax"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Parse(tc.input))
		})
	}
}

// 复合条件必须解析成条件树而不是退回 expr：SYSTEM 渠道的表单数值以 "%f" 字符串抵达引擎，
// expr 强类型会编译报错并被 evaluateEdge 吞成 false，分支静默走错
func TestParse_Compound(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		vars model.Variables
		want bool
	}{
		{
			name: "与逻辑全命中",
			expr: "$amount > 1000 && $dept == 'IT'",
			vars: model.Variables{"amount": "2000.000000", "dept": "IT"},
			want: true,
		},
		{
			name: "与逻辑一项不命中",
			expr: "$amount > 1000 && $dept == 'HR'",
			vars: model.Variables{"amount": "2000.000000", "dept": "IT"},
			want: false,
		},
		{
			name: "或逻辑一项命中",
			expr: "$amount > 1000 || $dept == 'HR'",
			vars: model.Variables{"amount": "2000.000000", "dept": "IT"},
			want: true,
		},
		{
			name: "或逻辑全不命中",
			expr: "$amount > 5000 || $dept == 'HR'",
			vars: model.Variables{"amount": "2000.000000", "dept": "IT"},
			want: false,
		},
		{
			name: "括号分组",
			expr: "$amount > 1000 && ($dept == 'IT' || $dept == 'HR')",
			vars: model.Variables{"amount": "2000.000000", "dept": "HR"},
			want: true,
		},
		{
			name: "整体包裹",
			expr: "($amount > 1000)",
			vars: model.Variables{"amount": "2000.000000"},
			want: true,
		},
		{
			name: "集合与比较混合",
			expr: "$env in ('backend') || $risk >= 3",
			vars: model.Variables{"env": `["backend","ocr"]`, "risk": "1.000000"},
			want: true,
		},
		{
			name: "多选切片与数值混合",
			expr: "$env == 'backend' && $amount > 1000",
			vars: model.Variables{"env": `["backend","ocr"]`, "amount": "2000.000000"},
			want: true,
		},
		{
			name: "字面量内的与逻辑不得切分",
			expr: "$remark == 'a && b'",
			vars: model.Variables{"remark": "a && b"},
			want: true,
		},
		{
			name: "字面量内的或逻辑不得切分",
			expr: "$remark == 'a || b'",
			vars: model.Variables{"remark": "a || b"},
			want: true,
		},
		{
			name: "三段与逻辑",
			expr: "$a > 1 && $b > 2 && $c > 3",
			vars: model.Variables{"a": "2", "b": "3", "c": "4"},
			want: true,
		},
		{
			name: "与逻辑优先级高于或逻辑",
			expr: "$a > 9 || $b > 2 && $c > 3",
			vars: model.Variables{"a": "1", "b": "3", "c": "4"},
			want: true,
		},
		{
			name: "与逻辑优先级高于或逻辑且不命中",
			expr: "$a > 9 || $b > 2 && $c > 9",
			vars: model.Variables{"a": "1", "b": "3", "c": "4"},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := Parse(tc.expr)
			require.NotNil(t, cond)
			_, isExpr := cond.(Expr)
			require.False(t, isExpr, "复合条件必须解析成条件树，不能退回 expr")
			assert.Equal(t, tc.want, cond.Eval(tc.vars))
		})
	}
}

func TestParse_SetLiteral(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantOp  model.Op
		wantVal []string
	}{
		{"圆括号集合", "$env in ('a', 'b')", model.OpIn, []string{"a", "b"}},
		{"方括号集合", "$env in ['a', 'b']", model.OpIn, []string{"a", "b"}},
		{"方括号not in", "$env not in ['a']", model.OpNotIn, []string{"a"}},
		{"引号内逗号不得拆分", "$env in ('a,b', 'c')", model.OpIn, []string{"a,b", "c"}},
		{"方括号内引号逗号不得拆分", "$env in ['a,b', 'c']", model.OpIn, []string{"a,b", "c"}},
		{"单值集合", "$env in ('a')", model.OpIn, []string{"a"}},
		{"空集合", "$env in ()", model.OpIn, []string{}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, Compare{Field: "env", Op: tc.wantOp, Value: tc.wantVal}, Parse(tc.input))
		})
	}
}

// SQL 风格的大写集合算子必须与小写完全等价：expr 只认小写 in / not in，
// 方言层若只转括号不归一算子，in 会编译失败静默判不命中，not in 更会因取反而恒真放行
func TestParse_SetOperatorCaseInsensitive(t *testing.T) {
	testCases := []struct {
		name    string
		input   string
		wantOp  model.Op
		wantVal []string
		hit     model.Variables
		miss    model.Variables
	}{
		{"大写 IN 圆括号", "$env IN ('backend')", model.OpIn, []string{"backend"},
			model.Variables{"env": "backend"}, model.Variables{"env": "ocr"}},
		{"大写 IN 方括号", "$env IN ['backend']", model.OpIn, []string{"backend"},
			model.Variables{"env": "backend"}, model.Variables{"env": "ocr"}},
		{"混合大小写 In", "$env In ('backend','ocr')", model.OpIn, []string{"backend", "ocr"},
			model.Variables{"env": "ocr"}, model.Variables{"env": "dev"}},
		{"大写 NOT IN 圆括号", "$env NOT IN ('backend')", model.OpNotIn, []string{"backend"},
			model.Variables{"env": "ocr"}, model.Variables{"env": "backend"}},
		{"大写 NOT IN 方括号", "$env NOT IN ['backend']", model.OpNotIn, []string{"backend"},
			model.Variables{"env": "ocr"}, model.Variables{"env": "backend"}},
		{"混合大小写 Not In", "$env Not In ('backend','ocr')", model.OpNotIn, []string{"backend", "ocr"},
			model.Variables{"env": "dev"}, model.Variables{"env": "backend"}},
		{"算子与括号间无空白", "$env IN('backend')", model.OpIn, []string{"backend"},
			model.Variables{"env": "backend"}, model.Variables{"env": "ocr"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := Parse(tc.input)
			assert.Equal(t, Compare{Field: "env", Op: tc.wantOp, Value: tc.wantVal}, cond)
			assert.True(t, cond.Eval(tc.hit), "命中变量必须放行")
			assert.False(t, cond.Eval(tc.miss), "不命中变量必须拦下")
		})
	}
}

// 名为 In / Not 的普通字段不得被算子归一误伤，否则字段取值会落到关键字上恒不命中
func TestParse_FieldNamedLikeSetOperator(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		vars  model.Variables
		want  bool
	}{
		{"字段 In 大于比较成立", "$In > 5", model.Variables{"In": 10.0}, true},
		{"字段 In 大于比较不成立", "$In > 5", model.Variables{"In": 3.0}, false},
		{"字段 In 等值比较成立", "$In == 10", model.Variables{"In": 10.0}, true},
		{"字段 Not 等值比较成立", "$Not == 'x'", model.Variables{"Not": "x"}, true},
		{"字段 Not 等值比较不成立", "$Not == 'x'", model.Variables{"Not": "y"}, false},
		{"字段 In 后接真集合算子", "$In in (10,20)", model.Variables{"In": 10.0}, true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Parse(tc.input).Eval(tc.vars))
		})
	}
}

// 解析器对适配前后的文本必须产出完全一致的条件树：
// kernel 会对只落了 condition 的存量编译产物再解析一次，此时 in (...) 已被改写成 in [...]
func TestParse_IdempotentUnderDialect(t *testing.T) {
	inputs := []string{
		"$env in ('backend', 'frontend')",
		"$env not in ('ocr')",
		"$env in ('a,b', 'c')",
		"$amount > 1000",
		"$dept == 'IT'",
		"$level = 'high'",
		"$remark == 'a=b'",
		"$note == 'a in b'",
		"$amount > 1000 && $dept == 'IT'",
		"$env in ('backend') || $risk >= 3",
		"($amount > 1000)",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			assert.Equal(t, Parse(input), Parse(toExprText(input)))
		})
	}
}

// 解析不了就整体退回 expr，绝不能产出丢掉部分条件的半成品树
func TestParse_UnsupportedFallsBackToExpr(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{"无操作符", "invalid_syntax", "invalid_syntax"},
		{"重复比较符", "a == b == c", "a == b == c"},
		{"复合中夹带无法解析的子段", "$a > 1 && invalid_syntax", "a > 1 && invalid_syntax"},
		{"空括号", "()", "()"},
		{"标量裸词按变量引用处理", "$dept == IT", "dept == IT"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, Expr{Text: tc.want}, Parse(tc.input))
		})
	}
}

// 条件语法覆盖不了的表达式仍要能求值，这是保留 Expr 兜底节点的唯一理由
func TestParse_ExprFallbackStillEvaluates(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		vars model.Variables
		want bool
	}{
		{"算术运算", "$amount * 2 > 100", model.Variables{"amount": 100}, true},
		{"内置函数", "len($tags) > 1", model.Variables{"tags": []string{"a", "b"}}, true},
		{"函数调用不命中", "len($tags) > 3", model.Variables{"tags": []string{"a", "b"}}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := Parse(tc.expr)
			require.IsType(t, Expr{}, cond, "该表达式超出条件语法，应退回 expr")
			assert.Equal(t, tc.want, cond.Eval(tc.vars))
		})
	}
}

// 字段对字段比较必须退回 Expr 做真正的变量比较。方言层剥掉 $ 之后无法区分变量引用与裸词，
// 若把右值标识符收成字面量，$amount == $limit 会恒不命中、$amount != $limit 会恒真误放行
func TestParse_FieldReferenceFallsBackToExpr(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		vars model.Variables
		want bool
	}{
		{"等值成立", "$amount == $limit", model.Variables{"amount": 100, "limit": 100}, true},
		{"等值不成立", "$amount == $limit", model.Variables{"amount": 100, "limit": 200}, false},
		{"大于成立", "$amount > $limit", model.Variables{"amount": 200, "limit": 100}, true},
		{"大于不成立", "$amount > $limit", model.Variables{"amount": 100, "limit": 200}, false},
		{"不等值不得恒真", "$amount != $limit", model.Variables{"amount": 100, "limit": 100}, false},
		{"不等值成立", "$amount != $limit", model.Variables{"amount": 100, "limit": 200}, true},
		{"字符串字段相等", "$env == $other", model.Variables{"env": "backend", "other": "backend"}, true},
		{"字符串字段不等", "$env == $other", model.Variables{"env": "backend", "other": "java"}, false},
		{"缺字段必须判不通过", "$amount != $limit", model.Variables{"unrelated": 1}, false},
		{"复合中夹带字段比较", "$amount == $limit && $dept == 'IT'", model.Variables{"amount": 1, "limit": 1, "dept": "IT"}, true},
		{"复合中字段比较不成立", "$amount == $limit && $dept == 'IT'", model.Variables{"amount": 1, "limit": 2, "dept": "IT"}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cond := Parse(tc.expr)
			require.IsType(t, Expr{}, cond, "字段对字段比较应退回 expr")
			assert.Equal(t, tc.want, cond.Eval(tc.vars))
		})
	}
}

// 集合枚举沿用 SQL 裸词风格（in (backend, java) 不带引号），必须仍解析成 Compare。
// 标量右值不放行裸词、集合元素放行，这条不对称是刻意的
func TestParse_BareWordSetElements(t *testing.T) {
	testCases := []struct {
		name string
		expr string
		want Condition
	}{
		{"裸词多值", "$env in (backend, java)", Compare{Field: "env", Op: model.OpIn, Value: []string{"backend", "java"}}},
		{"裸词单值", "env in (backend)", Compare{Field: "env", Op: model.OpIn, Value: []string{"backend"}}},
		{"裸词与引号混用", "$env in (backend, 'ocr')", Compare{Field: "env", Op: model.OpIn, Value: []string{"backend", "ocr"}}},
		{"裸词 not in", "$env not in (backend, java)", Compare{Field: "env", Op: model.OpNotIn, Value: []string{"backend", "java"}}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, Parse(tc.expr))
		})
	}
}

// 标准模式必须原文直送：任何方言改写都会让新接入方写的 expr 被静默篡改
func TestParseStandard_KeepsTextVerbatim(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  Condition
	}{
		{"空文本无条件放行", "", nil},
		{"纯空白无条件放行", "   ", nil},
		{"前后空白被裁剪", "  amount > 100  ", Expr{Text: "amount > 100"}},
		{"不剥 $ 前缀", "$amount > 100", Expr{Text: "$amount > 100"}},
		{"不补单等号", "amount = 200", Expr{Text: "amount = 200"}},
		{"不归一大写算子", "env IN ['backend']", Expr{Text: "env IN ['backend']"}},
		{"不改集合圆括号", "amount in (1..5)", Expr{Text: "amount in (1..5)"}},
		{"简单比较也不映射成 Compare", "amount > 100", Expr{Text: "amount > 100"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseStandard(tc.input))
		})
	}
}

// 标准模式只有一套严格语义：同一写法不会因 AST 形状落到宽松比较上
func TestParseStandard_StrictTyping(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		vars  model.Variables
		want  bool
	}{
		{"原生数值比较成立", "amount > 100", model.Variables{"amount": 200.0}, true},
		{"数值字符串不转数值", "amount > 100", model.Variables{"amount": "200.000000"}, false},
		{"原生集合包含成立", "env in ['backend']", model.Variables{"env": "backend"}, true},
		{"JSON数组字符串不还原集合", "env in ['backend']", model.Variables{"env": `["backend","ocr"]`}, false},
		{"区间语法原生可用", "amount in (1..5)", model.Variables{"amount": 3.0}, true},
		{"变量集合原生可用", "env in (envList)", model.Variables{"env": "backend", "envList": []string{"backend"}}, true},
		{"缺字段判不通过", "amount > 100", model.Variables{}, false},
		{"老方言 $ 前缀不认", "$amount > 100", model.Variables{"amount": 200.0}, false},
		{"老方言单等号不认", "amount = 200", model.Variables{"amount": 200.0}, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseStandard(tc.input).Eval(tc.vars))
		})
	}
}
