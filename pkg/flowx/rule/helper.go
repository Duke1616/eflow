package rule

import "github.com/Duke1616/eflow/pkg/flowx/model"

// Eq 构造单字段等于判定的原子条件组
func Eq(field string, value any) model.ConditionGroup {
	return model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: field, Operator: model.OpEqual, Value: value},
		},
	}
}

// Ne 构造单字段不等于判定的原子条件组
func Ne(field string, value any) model.ConditionGroup {
	return model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: field, Operator: model.OpNotEqual, Value: value},
		},
	}
}

// Gt 构造单字段大于判定的原子条件组
func Gt(field string, value any) model.ConditionGroup {
	return model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: field, Operator: model.OpGreaterThan, Value: value},
		},
	}
}

// Gte 构造单字段大于等于判定的原子条件组
func Gte(field string, value any) model.ConditionGroup {
	return model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: field, Operator: model.OpGreaterEqual, Value: value},
		},
	}
}

// Lt 构造单字段小于判定的原子条件组
func Lt(field string, value any) model.ConditionGroup {
	return model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: field, Operator: model.OpLessThan, Value: value},
		},
	}
}

// Lte 构造单字段小于等于判定的原子条件组
func Lte(field string, value any) model.ConditionGroup {
	return model.ConditionGroup{
		Logic: "AND",
		Conditions: []model.Condition{
			{Field: field, Operator: model.OpLessEqual, Value: value},
		},
	}
}
