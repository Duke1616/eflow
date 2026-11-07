package rule

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// standardRuleEvaluator 标准规则组求值器实现
type standardRuleEvaluator struct{}

// NewRuleEvaluator 创建规则组求值器实例
func NewRuleEvaluator() IRuleEvaluator {
	return &standardRuleEvaluator{}
}

// DefaultEvaluator 全局默认规则组求值器单例
var DefaultEvaluator IRuleEvaluator = NewRuleEvaluator()

// Evaluate 实现 IRuleEvaluator 接口：对复合条件组进行规则求值
func (e *standardRuleEvaluator) Evaluate(group *model.ConditionGroup, vars model.Variables) bool {
	if group == nil || (len(group.Conditions) == 0 && len(group.SubGroups) == 0) {
		return true // 无规则限制，默认放行
	}

	evalCond := func(cond model.Condition) bool {
		return evaluateSingleCondition(cond, vars[cond.Field])
	}
	evalSub := func(sub model.ConditionGroup) bool {
		return e.Evaluate(&sub, vars)
	}

	// OR 逻辑：只要任一原子条件或任一子条件组命中即可短路通过
	if strings.EqualFold(group.Logic, "OR") {
		return lo.SomeBy(group.Conditions, evalCond) || lo.SomeBy(group.SubGroups, evalSub)
	}

	// 默认为 AND 逻辑：必须所有原子条件与子条件组全部满足
	return lo.EveryBy(group.Conditions, evalCond) && lo.EveryBy(group.SubGroups, evalSub)
}

// EvaluateRuleGroup 包级便捷函数：委托全局默认求值器执行复合条件组求值
func EvaluateRuleGroup(group *model.ConditionGroup, vars model.Variables) bool {
	return DefaultEvaluator.Evaluate(group, vars)
}

// evaluateSingleCondition 委托算子策略注册表执行单项条件匹配
func evaluateSingleCondition(cond model.Condition, actualVal any) bool {
	op, exists := operatorRegistry[cond.Operator]
	if !exists {
		return false
	}
	return op.Match(actualVal, cond.Value)
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
