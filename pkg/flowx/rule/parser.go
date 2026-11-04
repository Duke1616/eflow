package rule

import (
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// ParseConditionText 将类似 "days > 3"、"dept == 'IT'" 的简易条件文本在编译期解析为标准强类型 ConditionGroup
// 消除运行时的字符串遍历与切分开销，让流程引擎只面对统一的 RuleGroup 领域模型
func ParseConditionText(condition string) *model.ConditionGroup {
	cond := strings.TrimSpace(condition)
	if cond == "" {
		return nil
	}

	// 优先匹配双字符操作符 (如 >=, <=, ==, !=)，避免被单字符 > 或 < 抢先截断
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
