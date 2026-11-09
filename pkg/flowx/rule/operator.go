package rule

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// IOperator 算子求值策略接口
// 将各种比对逻辑（等于、比较、包含、判空）解耦为独立算子对象，遵循开闭原则 (OCP)
type IOperator interface {
	Match(actual any, target any) bool
}

// equalOperator 等值比对算子
type equalOperator struct{}

func (o *equalOperator) Match(actual any, target any) bool {
	if leftNum, ok1 := ToNumeric(actual); ok1 {
		if rightNum, ok2 := ToNumeric(target); ok2 {
			return leftNum == rightNum
		}
	}

	// 智能兼容表单多选切片与目标标量的匹配（如 $environment = 'backend'）
	if actualSlice, isSlice := extractSlice(actual); isSlice {
		targetStr := fmt.Sprintf("%v", target)
		return lo.Contains(actualSlice, targetStr)
	}

	return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", target)
}

// notEqualOperator 不等比对算子
type notEqualOperator struct {
	eq equalOperator
}

func (o *notEqualOperator) Match(actual any, target any) bool {
	return !o.eq.Match(actual, target)
}

// numericCompareOperator 数值大小比对算子
type numericCompareOperator struct {
	cmp func(left, right float64) bool
}

func (o *numericCompareOperator) Match(actual any, target any) bool {
	left, ok1 := ToNumeric(actual)
	right, ok2 := ToNumeric(target)
	if !ok1 || !ok2 {
		return false
	}
	return o.cmp(left, right)
}

// inOperator 集合包含算子（支持标量 in 集合，以及表单多选切片与目标集合的交集匹配）
type inOperator struct{}

func (o *inOperator) Match(actual any, target any) bool {
	condSlice := ToStringSlice(target)
	if len(condSlice) == 0 {
		return false
	}

	if actualSlice, isSlice := extractSlice(actual); isSlice {
		if len(actualSlice) == 0 {
			return false
		}
		return len(lo.Intersect(actualSlice, condSlice)) > 0
	}

	targetStr := fmt.Sprintf("%v", actual)
	return lo.Contains(condSlice, targetStr)
}

// notInOperator 集合不包含算子
type notInOperator struct {
	in inOperator
}

func (o *notInOperator) Match(actual any, target any) bool {
	return !o.in.Match(actual, target)
}

// containsOperator 字符串子串包含算子（切片按元素精准包含，标量按子串匹配）
type containsOperator struct{}

func (o *containsOperator) Match(actual any, target any) bool {
	if actual == nil || target == nil {
		return false
	}
	targetStr := fmt.Sprintf("%v", target)
	if actualSlice, isSlice := extractSlice(actual); isSlice {
		return lo.Contains(actualSlice, targetStr)
	}
	actualStr := fmt.Sprintf("%v", actual)
	return strings.Contains(actualStr, targetStr)
}

// emptyOperator 判空算子（支持标量为空与空切片判定）
type emptyOperator struct{}

func (o *emptyOperator) Match(actual any, _ any) bool {
	if actual == nil || actual == "" {
		return true
	}
	if actualSlice, isSlice := extractSlice(actual); isSlice {
		return len(actualSlice) == 0
	}
	return false
}

// notEmptyOperator 非空算子
type notEmptyOperator struct {
	empty emptyOperator
}

func (o *notEmptyOperator) Match(actual any, _ any) bool {
	return !o.empty.Match(actual, nil)
}

// operatorRegistry 全局算子注册表
var operatorRegistry = map[model.Op]IOperator{
	model.OpEqual:        &equalOperator{},
	model.OpNotEqual:     &notEqualOperator{},
	model.OpGreaterThan:  &numericCompareOperator{cmp: func(a, b float64) bool { return a > b }},
	model.OpGreaterEqual: &numericCompareOperator{cmp: func(a, b float64) bool { return a >= b }},
	model.OpLessThan:     &numericCompareOperator{cmp: func(a, b float64) bool { return a < b }},
	model.OpLessEqual:    &numericCompareOperator{cmp: func(a, b float64) bool { return a <= b }},
	model.OpIn:           &inOperator{},
	model.OpNotIn:        &notInOperator{},
	model.OpContains:     &containsOperator{},
	model.OpEmpty:        &emptyOperator{},
	model.OpNotEmpty:     &notEmptyOperator{},
}

// extractSlice 提取表单实际值的切片表示并指示其是否属于集合/切片类型（支持 []string, []any, JSON array 字符串, 逗号分隔字符串）
func extractSlice(val any) ([]string, bool) {
	if val == nil {
		return nil, false
	}
	switch v := val.(type) {
	case []string:
		return v, true
	case []any:
		return lo.Map(v, func(item any, _ int) string { return fmt.Sprintf("%v", item) }), true
	case string:
		str := strings.TrimSpace(v)
		if strings.HasPrefix(str, "[") && strings.HasSuffix(str, "]") {
			var slice []string
			if err := json.Unmarshal([]byte(str), &slice); err == nil {
				return slice, true
			}
		}
		if strings.Contains(str, ",") {
			parts := strings.Split(str, ",")
			return lo.Map(parts, func(p string, _ int) string { return strings.TrimSpace(p) }), true
		}
	}
	return nil, false
}

// extractActualSlice 兼容历史调用，返回切片
func extractActualSlice(val any) []string {
	slice, _ := extractSlice(val)
	return slice
}
