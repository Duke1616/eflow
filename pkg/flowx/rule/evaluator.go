package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// EvaluateRuleGroup 对复合条件组进行规则求值，判断给定的表单变量是否满足触发要求
func EvaluateRuleGroup(group *model.ConditionGroup, vars map[string]any) bool {
	if group == nil || (len(group.Conditions) == 0 && len(group.SubGroups) == 0) {
		return true // 无规则限制，默认放行
	}

	evalCond := func(cond model.Condition) bool {
		return evaluateSingleCondition(cond, vars[cond.Field])
	}
	evalSub := func(sub model.ConditionGroup) bool {
		return EvaluateRuleGroup(&sub, vars)
	}

	// OR 逻辑：只要任一原子条件或任一子条件组命中即可短路通过
	if strings.EqualFold(group.Logic, "OR") {
		return lo.SomeBy(group.Conditions, evalCond) || lo.SomeBy(group.SubGroups, evalSub)
	}

	// 默认为 AND 逻辑：必须所有原子条件与子条件组全部满足
	return lo.EveryBy(group.Conditions, evalCond) && lo.EveryBy(group.SubGroups, evalSub)
}

func evaluateSingleCondition(cond model.Condition, actualVal any) bool {
	switch cond.Operator {
	case model.OpEmpty:
		return actualVal == nil || actualVal == ""
	case model.OpNotEmpty:
		return actualVal != nil && actualVal != ""
	}

	// 若字段不存在且不是判空操作，直接不匹配
	if actualVal == nil {
		return false
	}

	switch cond.Operator {
	case model.OpEqual:
		// 优先数值比较，消除浮点数与整型 JSON 反序列化后的字符串差异 (如 float64(3) vs int(3))
		if leftNum, ok1 := ToNumeric(actualVal); ok1 {
			if rightNum, ok2 := ToNumeric(cond.Value); ok2 {
				return leftNum == rightNum
			}
		}
		return fmt.Sprintf("%v", actualVal) == fmt.Sprintf("%v", cond.Value)
	case model.OpNotEqual:
		if leftNum, ok1 := ToNumeric(actualVal); ok1 {
			if rightNum, ok2 := ToNumeric(cond.Value); ok2 {
				return leftNum != rightNum
			}
		}
		return fmt.Sprintf("%v", actualVal) != fmt.Sprintf("%v", cond.Value)
	case model.OpGreaterThan:
		left, ok1 := ToNumeric(actualVal)
		right, ok2 := ToNumeric(cond.Value)
		return ok1 && ok2 && left > right
	case model.OpGreaterEqual:
		left, ok1 := ToNumeric(actualVal)
		right, ok2 := ToNumeric(cond.Value)
		return ok1 && ok2 && left >= right
	case model.OpLessThan:
		left, ok1 := ToNumeric(actualVal)
		right, ok2 := ToNumeric(cond.Value)
		return ok1 && ok2 && left < right
	case model.OpLessEqual:
		left, ok1 := ToNumeric(actualVal)
		right, ok2 := ToNumeric(cond.Value)
		return ok1 && ok2 && left <= right
	case model.OpIn:
		targetStr := fmt.Sprintf("%v", actualVal)
		slice := ToStringSlice(cond.Value)
		return lo.Contains(slice, targetStr)
	case model.OpNotIn:
		targetStr := fmt.Sprintf("%v", actualVal)
		slice := ToStringSlice(cond.Value)
		return !lo.Contains(slice, targetStr)
	case model.OpContains:
		actualStr := fmt.Sprintf("%v", actualVal)
		targetStr := fmt.Sprintf("%v", cond.Value)
		return strings.Contains(actualStr, targetStr)
	default:
		return false
	}
}

// ToNumeric 通用数值解析转换辅助函数
func ToNumeric(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case int16:
		return float64(n), true
	case int8:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint8:
		return float64(n), true
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case string:
		if num, err := strconv.ParseFloat(n, 64); err == nil {
			return num, true
		}
	}
	return 0, false
}

// ToStringSlice 通用字符串切片解析辅助函数
func ToStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		return lo.Map(s, func(item any, _ int) string {
			return fmt.Sprintf("%v", item)
		})
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}
