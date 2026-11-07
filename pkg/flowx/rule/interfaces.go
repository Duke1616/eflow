package rule

import "github.com/Duke1616/eflow/pkg/flowx/model"

// IConditionParser 条件文本解析器接口
// 职责单一：将原始条件文本在编译期解析为统一强类型的 ConditionGroup
type IConditionParser interface {
	Parse(condition string) *model.ConditionGroup
}

// IRuleEvaluator 规则组求值器接口
// 职责单一：裁决复合条件组在当前流程变量上下文中是否满足放行条件
type IRuleEvaluator interface {
	Evaluate(group *model.ConditionGroup, vars model.Variables) bool
}
