package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/expr-lang/expr/ast"
	exprparser "github.com/expr-lang/expr/parser"
)

// Parse 把画布条件文本编译成条件树。空文本返回 nil，表示无条件放行。
// 语法分析复用 expr 的解析器，这里只把它认得的"字段 算子 字面量"与逻辑组合映射成条件树。
// 映射不了就整体退回 Expr 节点，绝不产出丢掉部分条件的半成品树——那会让分支静默放行。
func Parse(text string) Condition {
	if strings.TrimSpace(text) == "" {
		return nil
	}

	exprText := toExprText(text)
	tree, err := exprparser.Parse(exprText)
	if err != nil {
		return Expr{Text: exprText}
	}
	if cond := fromAST(tree.Node); cond != nil {
		return cond
	}
	return Expr{Text: exprText}
}

// ParseStandard 把标准 expr 文本原样交给字节码引擎：不做 SQL 方言适配，也不映射成宽松比较树。
// 新接入方走这条路——写什么就执行什么，类型语义只有一套，不会因写法多个 * 1 就换一套裁决。
// 空文本返回 nil，表示无条件放行。
func ParseStandard(text string) Condition {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return Expr{Text: text}
}

// fromAST 只接受比较与逻辑组合；算术、函数调用、成员访问等返回 nil，交由 expr 字节码引擎求值
func fromAST(node ast.Node) Condition {
	switch n := node.(type) {
	case *ast.BinaryNode:
		if n.Operator == "&&" || n.Operator == "||" {
			return logicFromAST(n)
		}
		op, ok := comparisonOp(n.Operator)
		if !ok {
			return nil
		}
		return compareFromAST(n, op)

	case *ast.UnaryNode:
		// expr 把 not in 解析成 not (x in [...])，只有这一种否定形式能映射回集合算子
		if n.Operator != "not" {
			return nil
		}
		inner, ok := n.Node.(*ast.BinaryNode)
		if !ok || inner.Operator != "in" {
			return nil
		}
		return compareFromAST(inner, model.OpNotIn)
	}
	return nil
}

func compareFromAST(n *ast.BinaryNode, op model.Op) Condition {
	field, ok := n.Left.(*ast.IdentifierNode)
	if !ok {
		return nil
	}
	value, ok := literalFromAST(n.Right)
	if !ok {
		return nil
	}
	return Compare{Field: field.Value, Op: op, Value: value}
}

// logicFromAST 摊平同种逻辑：expr 的 && 左结合，a && b && c 解析成嵌套二元节点，
// 摊平后落库形状才与历史平铺的 conditions 一致
func logicFromAST(n *ast.BinaryNode) Condition {
	children := make([]Condition, 0, 2)

	var collect func(ast.Node) bool
	collect = func(node ast.Node) bool {
		if sub, ok := node.(*ast.BinaryNode); ok && sub.Operator == n.Operator {
			return collect(sub.Left) && collect(sub.Right)
		}
		child := fromAST(node)
		if child == nil {
			return false
		}
		children = append(children, child)
		return true
	}

	if !collect(n) {
		return nil
	}
	if n.Operator == "&&" {
		return And(children)
	}
	return Or(children)
}

// literalFromAST 取右值。数值统一成 float64、数字字符串转数值，
// 与历史编译产物的落库类型和宽松比较语义保持一致（表单数值常以 "%f" 字符串抵达）。
func literalFromAST(node ast.Node) (any, bool) {
	if arr, ok := node.(*ast.ArrayNode); ok {
		items := make([]string, 0, len(arr.Nodes))
		for _, item := range arr.Nodes {
			v, ok := setElement(item)
			if !ok {
				return nil, false
			}
			items = append(items, fmt.Sprintf("%v", v))
		}
		return items, true
	}

	v, ok := literalValue(node)
	if !ok {
		return nil, false
	}
	switch n := v.(type) {
	case int:
		return float64(n), true
	case string:
		if num, err := strconv.ParseFloat(n, 64); err == nil {
			return num, true
		}
	}
	return v, true
}

// setElement 取集合元素，比标量右值多放行一种裸词：SQL 风格的 in (backend, java) 枚举不带引号。
// 标量右值不放行裸词，因为方言层剥掉 $ 之后无法区分变量引用与裸词，
// 把 $amount == $limit 收成 Compare{amount, EQ, "limit"} 会让它恒不命中；退回 Expr 才是正确的变量比较。
func setElement(node ast.Node) (any, bool) {
	if ident, ok := node.(*ast.IdentifierNode); ok {
		return ident.Value, true
	}
	return literalValue(node)
}

// literalValue 取字面量原生值，不做任何类型转换：集合元素必须保留原文，
// 否则 in ('1.50') 会被转成 "1.5" 而永不命中
func literalValue(node ast.Node) (any, bool) {
	switch n := node.(type) {
	case *ast.StringNode:
		return n.Value, true
	case *ast.IntegerNode:
		return n.Value, true
	case *ast.FloatNode:
		return n.Value, true
	case *ast.BoolNode:
		return n.Value, true
	case *ast.UnaryNode:
		if n.Operator != "-" {
			return nil, false
		}
		v, ok := literalValue(n.Node)
		if !ok {
			return nil, false
		}
		switch num := v.(type) {
		case int:
			return -num, true
		case float64:
			return -num, true
		}
	}
	return nil, false
}

func comparisonOp(sym string) (model.Op, bool) {
	switch sym {
	case "==":
		return model.OpEqual, true
	case "!=":
		return model.OpNotEqual, true
	case ">":
		return model.OpGreaterThan, true
	case ">=":
		return model.OpGreaterEqual, true
	case "<":
		return model.OpLessThan, true
	case "<=":
		return model.OpLessEqual, true
	case "in":
		return model.OpIn, true
	}
	return "", false
}
