package domain

const DefaultDispatchPriority = 100

// Dispatch 描述工单字段命中后覆盖自动化节点默认 Runner 的路由规则。
type Dispatch struct {
	Id               int64
	TemplateId       int64
	AutomationNodeID string
	RunnerId         int64
	Field            string
	Value            string
	Priority         int
}

// RunnerRouteDecision 是一次执行尝试使用的 Runner 路由快照。
// RuleID 为 0 表示没有命中规则，最终使用节点默认 Runner。
type RunnerRouteDecision struct {
	DefaultRunnerID  int64
	SelectedRunnerID int64
	RuleID           int64
}

func (d RunnerRouteDecision) Routed() bool { return d.RuleID > 0 }
