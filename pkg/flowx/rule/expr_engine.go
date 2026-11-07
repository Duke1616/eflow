package rule

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// IExpressionEngine 动态文本表达式执行引擎接口
type IExpressionEngine interface {
	// Evaluate 执行条件文本表达式并返回布尔判定结果
	Evaluate(expression string, env map[string]any) (bool, error)
}

// ExprEngine 基于 expr-lang/expr 高性能字节码虚拟机实现的表达式引擎
// 内部使用并发安全的 sync.Map 对编译后的 AST Program 进行字节码级缓存
type ExprEngine struct {
	cache sync.Map // map[string]*vm.Program
}

var defaultExprEngine = &ExprEngine{}

// GetDefaultEngine 获取默认全局表达式引擎单例
func GetDefaultEngine() *ExprEngine {
	return defaultExprEngine
}

// Evaluate 执行表达式求值
func (e *ExprEngine) Evaluate(expression string, env map[string]any) (bool, error) {
	cleanExpr := NormalizeExpression(expression)
	if cleanExpr == "" {
		return true, nil
	}

	program, err := e.compile(cleanExpr, env)
	if err != nil {
		return false, fmt.Errorf("编译表达式 [%s] 失败: %w", expression, err)
	}

	output, err := expr.Run(program, env)
	if err != nil {
		return false, fmt.Errorf("执行表达式 [%s] 失败: %w", expression, err)
	}

	boolVal, ok := output.(bool)
	if !ok {
		return false, fmt.Errorf("表达式 [%s] 返回结果非布尔值: %v", expression, output)
	}

	return boolVal, nil
}

func (e *ExprEngine) compile(cleanExpr string, env map[string]any) (*vm.Program, error) {
	if cached, ok := e.cache.Load(cleanExpr); ok {
		return cached.(*vm.Program), nil
	}

	program, err := expr.Compile(cleanExpr, expr.Env(env), expr.AsBool())
	if err != nil {
		return nil, err
	}

	e.cache.Store(cleanExpr, program)
	return program, nil
}

var (
	// reDollarVar 匹配并剥除前端变量前缀 $ (如 $days -> days)
	reDollarVar = regexp.MustCompile(`\$([a-zA-Z0-9_]+)`)
	// reSqlIn 匹配 SQL 风格的集合圆括号 in ('a', 'b') 并转为数组中括号 ['a', 'b']
	reSqlIn = regexp.MustCompile(`(?i)\s+(not\s+in|in)\s*\(([^)]+)\)`)
	// reSingleEqual 替换单等号为双等号 (兼容 SQL 风格等号)
	reSingleEqual = regexp.MustCompile(`([^=!><])=([^=])`)
)

// NormalizeExpression 将前端画布/SQL风格的弱表达式规范化为标准 expr 语法
// 例如: "$environment in ('backend', 'ocr')" -> "environment in ['backend', 'ocr']"
// 例如: "days = 3" -> "days == 3"
func NormalizeExpression(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}

	// 1. 去除 $ 变量前缀
	s = reDollarVar.ReplaceAllString(s, "$1")

	// 2. 转换 SQL 风格的 in (...) -> in [...]
	s = reSqlIn.ReplaceAllStringFunc(s, func(match string) string {
		sub := reSqlIn.FindStringSubmatch(match)
		if len(sub) == 3 {
			op := strings.ToLower(strings.TrimSpace(sub[1]))
			items := sub[2]
			return fmt.Sprintf(" %s [%s]", op, items)
		}
		return match
	})

	// 3. 规范化单等号 = 为 == (避免影响 ==, !=, >=, <=)
	for reSingleEqual.MatchString(s) {
		s = reSingleEqual.ReplaceAllString(s, "${1}==${2}")
	}

	return s
}
