package model

// Op 条件判定操作符枚举 (对齐 ealert 规则引擎规范)
type Op string

const (
	OpEqual        Op = "EQ"        // 等于 ==
	OpNotEqual     Op = "NE"        // 不等于 !=
	OpGreaterThan  Op = "GT"        // 大于 >
	OpGreaterEqual Op = "GTE"       // 大于等于 >=
	OpLessThan     Op = "LT"        // 小于 <
	OpLessEqual    Op = "LTE"       // 小于等于 <=
	OpIn           Op = "IN"        // 属于集合 (如 in ["IT", "HR"])
	OpNotIn        Op = "NOT_IN"    // 不属于集合
	OpContains     Op = "CONTAINS"  // 包含子串
	OpEmpty        Op = "EMPTY"     // 为空或不存在
	OpNotEmpty     Op = "NOT_EMPTY" // 非空
)

// Condition 原子判定条件
type Condition struct {
	Field    string `json:"field"` // 表单字段 Key (如 "days", "amount", "dept")
	Operator Op     `json:"op"`    // 比较操作符
	Value    any    `json:"value"` // 目标期望值
}

// ConditionGroup 复合条件组（支持多规则组合与嵌套子组，对齐 ealert 告警规则）
type ConditionGroup struct {
	Logic      string           `json:"logic"`                // "AND" 或 "OR"，默认为 "AND"
	Conditions []Condition      `json:"conditions"`           // 本组包含的具体条件
	SubGroups  []ConditionGroup `json:"sub_groups,omitempty"` // 支持嵌套子条件组
}
