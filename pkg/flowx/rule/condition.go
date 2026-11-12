package rule

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// Condition 条件树节点。求值即多态分派，新增节点类型不必改动求值路径。
// 落库不在此列：历史 JSON 是封闭契约，新增节点必须同步扩展 ToGroup 的映射。
// nil 表示无条件约束，由调用方按放行处理。
type Condition interface {
	Eval(vars model.Variables) bool
}

// And 全部子节点成立才放行
type And []Condition

func (n And) Eval(vars model.Variables) bool {
	return lo.EveryBy(n, func(c Condition) bool { return c.Eval(vars) })
}

// Or 任一子节点成立即放行
type Or []Condition

func (n Or) Eval(vars model.Variables) bool {
	return lo.SomeBy(n, func(c Condition) bool { return c.Eval(vars) })
}

// Compare 单字段比较。集合算子的 Value 是 []string，其余是标量。
// 宽松类型（表单数值以字符串抵达、多选以 JSON 数组字符串抵达）由 matchOperator 统一承接。
type Compare struct {
	Field string
	Op    model.Op
	Value any
}

func (c Compare) Eval(vars model.Variables) bool {
	return matchOperator(c.Op, vars[c.Field], c.Value)
}

// Expr 兜底节点：条件语法覆盖不了的表达式（算术、函数调用）交给 expr 字节码引擎。
// 求值失败判不通过——宁可分支不命中，也不能误放行。
type Expr struct {
	Text string
}

func (e Expr) Eval(vars model.Variables) bool {
	res, err := evalExpr(e.Text, vars)
	if err != nil {
		return false
	}
	return res
}
