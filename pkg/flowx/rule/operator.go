package rule

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// matchOperator 按算子裁决单项条件；未识别的算子判不通过，因为 op 来自持久化 JSON，误放行会让存量模板走错分支
func matchOperator(op model.Op, actual, target any) bool {
	switch op {
	case model.OpEqual:
		return matchEqual(actual, target)
	case model.OpNotEqual:
		return !matchEqual(actual, target)
	case model.OpGreaterThan:
		return matchNumeric(actual, target, func(a, b float64) bool { return a > b })
	case model.OpGreaterEqual:
		return matchNumeric(actual, target, func(a, b float64) bool { return a >= b })
	case model.OpLessThan:
		return matchNumeric(actual, target, func(a, b float64) bool { return a < b })
	case model.OpLessEqual:
		return matchNumeric(actual, target, func(a, b float64) bool { return a <= b })
	case model.OpIn:
		return matchIn(actual, target)
	case model.OpNotIn:
		return !matchIn(actual, target)
	case model.OpContains:
		return matchContains(actual, target)
	case model.OpEmpty:
		return matchEmpty(actual)
	case model.OpNotEmpty:
		return !matchEmpty(actual)
	default:
		return false
	}
}

// matchEqual 优先按数值比（兼容 JSON 反序列化后 float64 与 int 跨类型），再兼容多选切片，最后退化为字符串比对
func matchEqual(actual, target any) bool {
	if leftNum, ok1 := toNumeric(actual); ok1 {
		if rightNum, ok2 := toNumeric(target); ok2 {
			return leftNum == rightNum
		}
	}

	// 兼容表单多选切片与目标标量的匹配（如 $environment = 'backend'）
	if actualSlice, isSlice := extractSlice(actual); isSlice {
		return lo.Contains(actualSlice, fmt.Sprintf("%v", target))
	}

	return fmt.Sprintf("%v", actual) == fmt.Sprintf("%v", target)
}

func matchNumeric(actual, target any, cmp func(left, right float64) bool) bool {
	left, ok1 := toNumeric(actual)
	right, ok2 := toNumeric(target)
	if !ok1 || !ok2 {
		return false
	}
	return cmp(left, right)
}

// matchIn 标量看是否命中集合，表单多选切片看与集合是否有交集
func matchIn(actual, target any) bool {
	condSlice := toStringSlice(target)
	if len(condSlice) == 0 {
		return false
	}

	if actualSlice, isSlice := extractSlice(actual); isSlice {
		if len(actualSlice) == 0 {
			return false
		}
		return len(lo.Intersect(actualSlice, condSlice)) > 0
	}

	return lo.Contains(condSlice, fmt.Sprintf("%v", actual))
}

// matchContains 切片按元素精准包含，标量按子串匹配
func matchContains(actual, target any) bool {
	if actual == nil || target == nil {
		return false
	}
	targetStr := fmt.Sprintf("%v", target)
	if actualSlice, isSlice := extractSlice(actual); isSlice {
		return lo.Contains(actualSlice, targetStr)
	}
	return strings.Contains(fmt.Sprintf("%v", actual), targetStr)
}

func matchEmpty(actual any) bool {
	if actual == nil || actual == "" {
		return true
	}
	if actualSlice, isSlice := extractSlice(actual); isSlice {
		return len(actualSlice) == 0
	}
	return false
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

func toNumeric(v any) (float64, bool) {
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

// toStringSlice 把条件目标值归一为字符串集合；与 extractSlice 不同，此处不拆分逗号串
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch s := v.(type) {
	case []string:
		return s
	case []any:
		return lo.Map(s, func(item any, _ int) string { return fmt.Sprintf("%v", item) })
	default:
		return []string{fmt.Sprintf("%v", v)}
	}
}
