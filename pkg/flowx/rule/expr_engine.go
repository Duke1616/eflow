package rule

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
	"github.com/samber/lo"
)

// exprCache 按 cacheKey 缓存编译后的字节码，sync.Map 承接并发求值
var exprCache sync.Map // map[string]*vm.Program

// evalExpr 求值已规范化的 expr 文本（由 toExprText 产出），承接条件语法之外的算术与函数调用。
// 文本改写全部发生在词法层，这里不做二次规范化——重复改写正是历史上字面量被静默篡改的根源。
func evalExpr(text string, env map[string]any) (bool, error) {
	if text == "" {
		return true, nil
	}

	program, err := compileExpr(text, env)
	if err != nil {
		return false, fmt.Errorf("编译表达式 [%s] 失败: %w", text, err)
	}

	output, err := expr.Run(program, env)
	if err != nil {
		return false, fmt.Errorf("执行表达式 [%s] 失败: %w", text, err)
	}

	boolVal, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("表达式 [%s] 返回结果非布尔值: %v", text, output)
	}
	return boolVal, nil
}

// compileExpr 编译期绑定 env 是刻意的 fail-closed 设计：缺字段在编译期就失败判 false。
// 换成 AllowUndefinedVariables 会让缺字段退化成 nil 参与比较，使 != 与 not in 翻成 true 误放行。
func compileExpr(text string, env map[string]any) (*vm.Program, error) {
	key := cacheKey(text, env)
	if cached, ok := exprCache.Load(key); ok {
		return cached.(*vm.Program), nil
	}

	program, err := expr.Compile(text, expr.Env(env), expr.AsBool())
	if err != nil {
		return nil, err
	}

	exprCache.Store(key, program)
	return program, nil
}

// cacheKey 键必须覆盖编译的全部输入：expr.Env 把字段名集合吃进编译期，只按文本缓存会让
// 齐全 env 编出的程序被缺字段 env 复用，nil == nil 反判 true 造成误放行。
// 字段类型不进键——实测类型差异只导致编译/运行报错，不改变裁决。
func cacheKey(text string, env map[string]any) string {
	fields := lo.Keys(env)
	sort.Strings(fields)
	return text + "\x00" + strings.Join(fields, ",")
}
