package rule

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// 方言适配是唯一的文本改写点：剥 $ 前缀、单等号归一、in 的圆括号换方括号。
// 字面量与空白必须原样保留，历史上正是正则二次改写把 'a=b'、'$5' 静默篡改掉的。
func TestToExprText(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
		want string
	}{
		{"剥除变量前缀并转换集合括号", "$environment in ('backend', 'ocr')", "environment in ['backend', 'ocr']"},
		{"not in 同样转换集合括号", "$env not in ('test', 'dev')", "env not in ['test', 'dev']"},
		{"单等号补齐为双等号", "days = 3", "days == 3"},
		{"比较符不得被补成双等号", "a >= 1 && b <= 2 && c != 3", "a >= 1 && b <= 2 && c != 3"},
		{"字面量内单等号原样保留", "remark == 'a=b'", "remark == 'a=b'"},
		{"字面量内美元符号原样保留", "price == '$5'", "price == '$5'"},
		{"双引号字面量同样受保护", `note == "a=b in c"`, `note == "a=b in c"`},
		{"转义引号不得提前结束字面量", `remark == 'it\'s a=b'`, `remark == 'it\'s a=b'`},
		{"变量前缀与字面量保护同时生效", "$remark = 'a=b'", "remark == 'a=b'"},
		{"分组括号不得被当成集合括号", "($a > 1)", "(a > 1)"},
		{"函数调用括号不得被当成集合括号", "len($tags) > 1", "len(tags) > 1"},
		{"下标方括号原样保留", "$tags[0] == 'a'", "tags[0] == 'a'"},
		{"in 与括号之间的空白不影响识别", "$env in   ('a')", "env in   ['a']"},
		{"空白原样保留", "   ", "   "},
		{"大写 IN 归一为小写", "$env IN ('backend')", "env in ['backend']"},
		{"大写 NOT IN 归一为小写", "$env NOT IN ('backend')", "env not in ['backend']"},
		{"混合大小写 In 归一为小写", "$env In ('backend','ocr')", "env in ['backend','ocr']"},
		{"混合大小写 Not In 归一为小写", "$env Not In ('backend')", "env not in ['backend']"},
		{"方括号集合的大写算子同样归一", "$env IN ['backend']", "env in ['backend']"},
		{"方括号集合的大写 NOT IN 同样归一", "$env NOT IN ['backend']", "env not in ['backend']"},
		{"名为 In 的字段不得被误改成关键字", "$In > 5", "In > 5"},
		{"名为 In 的字段等值比较不受影响", "$In == 10", "In == 10"},
		{"名为 Not 的字段不得被误改成关键字", "$Not == 'x'", "Not == 'x'"},
		{"名为 In 的字段后接真算子只归一算子", "$In in (10,20)", "In in [10,20]"},
		{"not 后面不是 in 时不得归一", "$remark is NOT empty", "remark is NOT empty"},
		{"字面量内的大写 IN 原样保留", "remark == 'IN (x)'", "remark == 'IN (x)'"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, toExprText(tc.raw))
		})
	}
}

// 适配必须幂等：存量编译产物落的是已适配文本，kernel 加载时还会再走一次
func TestToExprText_Idempotent(t *testing.T) {
	inputs := []string{
		"a === b",
		"a = = b",
		"remark == 'a=b'",
		"$env in ('a', 'b')",
		"x != 1 && y <= 2",
		"len($tags) > 1",
		"($a > 1) && ($b in ('x'))",
		"$env IN ('a', 'b')",
		"$env NOT IN ['a']",
	}

	for _, raw := range inputs {
		t.Run(raw, func(t *testing.T) {
			once := toExprText(raw)
			assert.Equal(t, once, toExprText(once))
		})
	}
}

// 未闭合引号只能吞到结尾交给 expr 报错，绝不能越界 panic
func TestToExprText_UnterminatedLiteral(t *testing.T) {
	assert.NotPanics(t, func() {
		assert.Equal(t, "remark == 'abc", toExprText("remark == 'abc"))
	})
}
