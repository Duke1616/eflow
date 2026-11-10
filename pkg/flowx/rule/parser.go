package rule

import (
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// simpleParser 轻量级条件文本解析器实现
type simpleParser struct{}

// NewSimpleParser 创建简易条件文本解析器实例
func NewSimpleParser() IConditionParser {
	return &simpleParser{}
}

// DefaultParser 全局默认条件文本解析器单例
var DefaultParser IConditionParser = NewSimpleParser()

// Parse 实现 IConditionParser 接口
func (p *simpleParser) Parse(condition string) *model.ConditionGroup {
	cond := strings.TrimSpace(condition)
	if cond == "" {
		return nil
	}

	// 复合逻辑表达式 (含 && 或 ||) 超出轻量解析器职责范畴，返回 nil 委托 expr 字节码引擎执行
	if strings.Contains(cond, "&&") || strings.Contains(cond, "||") {
		return nil
	}

	lower := strings.ToLower(cond)

	// 1. 优先解析集合包含算子: "not in" 与 "in"
	if strings.Contains(lower, " not in ") || strings.Contains(lower, " in ") {
		var (
			op    model.Op
			opStr string
		)
		if strings.Contains(lower, " not in ") {
			op = model.OpNotIn
			opStr = " not in "
		} else {
			op = model.OpIn
			opStr = " in "
		}

		idx := strings.Index(lower, opStr)
		leftKey := strings.TrimSpace(cond[:idx])
		leftKey = strings.TrimPrefix(leftKey, "$")

		valPart := strings.TrimSpace(cond[idx+len(opStr):])
		valPart = strings.TrimPrefix(valPart, "(")
		valPart = strings.TrimSuffix(valPart, ")")

		rawItems := strings.Split(valPart, ",")
		vals := lo.FilterMap(rawItems, func(item string, _ int) (string, bool) {
			cleaned := strings.Trim(strings.TrimSpace(item), `"'`)
			return cleaned, cleaned != ""
		})

		return &model.ConditionGroup{
			Logic: "AND",
			Conditions: []model.Condition{
				{
					Field:    leftKey,
					Operator: op,
					Value:    vals,
				},
			},
		}
	}

	// 2. 匹配数值与等值比较符: >=, <=, ==, !=, >, <, =
	candidates := []struct {
		sym string
		op  model.Op
	}{
		{">=", model.OpGreaterEqual},
		{"<=", model.OpLessEqual},
		{"==", model.OpEqual},
		{"!=", model.OpNotEqual},
		{">", model.OpGreaterThan},
		{"<", model.OpLessThan},
		{"=", model.OpEqual}, // 兼容 SQL 风格的单等号
	}

	var (
		matchedSym string
		matchedOp  model.Op
	)

	for _, c := range candidates {
		if strings.Contains(cond, c.sym) {
			matchedSym = c.sym
			matchedOp = c.op
			break
		}
	}

	if matchedSym == "" {
		return nil
	}

	parts := strings.Split(cond, matchedSym)
	if len(parts) != 2 {
		return nil
	}

	leftKey := strings.TrimSpace(parts[0])
	leftKey = strings.TrimPrefix(leftKey, "$")
	rightValStr := strings.Trim(strings.TrimSpace(parts[1]), `"'`)

	var rightVal any = rightValStr
	if num, err := strconv.ParseFloat(rightValStr, 64); err == nil {
		rightVal = num
	}

	return &model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{
				Field:    leftKey,
				Operator: matchedOp,
				Value:    rightVal,
			},
		},
	}
}

// ParseConditionText 包级便捷函数：将简易条件文本在编译期解析为标准强类型 ConditionGroup
func ParseConditionText(condition string) *model.ConditionGroup {
	return DefaultParser.Parse(condition)
}
