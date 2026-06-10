package domain

// Dispatch 模版自动化节点、自动派发调度节点
type Dispatch struct {
	Id         int64
	TemplateId int64
	RunnerId   int64
	Field      string
	Value      string
}
