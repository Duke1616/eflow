package middleware

import "context"

// ActionType 引擎核心流转动作类型
type ActionType string

const (
	ActionStart    ActionType = "START"
	ActionPass     ActionType = "PASS"
	ActionReject   ActionType = "REJECT"
	ActionRevoke   ActionType = "REVOKE"
	ActionComplete ActionType = "COMPLETE"
)

// ActionContext 中间件管道执行上下文
type ActionContext struct {
	ActionType ActionType     // 当前执行的核心动作
	InstanceID string         // 流程实例唯一标识
	TaskID     string         // 当前操作的物理任务 ID (仅审批与驳回生效)
	Operator   string         // 当前操作人账号
	Comment    string         // 处理意见
	Variables  map[string]any // 表单业务变量
	Extra      map[string]any // 允许在不同拦截器间传递的元数据 (如链路追踪 TraceID、租户编号)
}

// Handler 中间件最终执行的核心处理函数
type Handler func(ctx context.Context, actCtx *ActionContext) error

// Middleware 洋葱圈模型拦截器，装饰下一个 Handler
type Middleware func(next Handler) Handler

// Compose 组合多个中间件形成洋葱调用链 (从右往左包裹，执行时从左往右)
func Compose(mws []Middleware, core Handler) Handler {
	handler := core
	for i := len(mws) - 1; i >= 0; i-- {
		handler = mws[i](handler)
	}
	return handler
}
