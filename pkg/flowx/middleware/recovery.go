package middleware

import (
	"context"
	"fmt"
	"runtime/debug"

	"github.com/gotomicro/ego/core/elog"
)

// PanicHandler 自定义 Panic 拦截处理函数
type PanicHandler func(ctx context.Context, actCtx *ActionContext, r any, stack []byte)

// Recovery 构建 Panic 恢复拦截器
// 用于兜底保护流转流水线，防止任意自定义 Worker 或外部 Hook 触发 Panic 导致主协程或 HTTP 服务崩溃
// 默认使用 elog 记录 Panic 堆栈日志，并支持传入自定义处理函数
func Recovery(handlers ...PanicHandler) Middleware {
	logger := elog.DefaultLogger.With(elog.FieldComponentName("flowx.recovery"))

	return func(next Handler) Handler {
		return func(ctx context.Context, actCtx *ActionContext) (err error) {
			defer func() {
				if r := recover(); r != nil {
					stack := debug.Stack()

					// 默认使用 elog 记录结构化 Panic 堆栈
					logger.Error("flowx execution panic recovered",
						elog.String("action", string(actCtx.ActionType)),
						elog.String("instance_id", actCtx.InstanceID),
						elog.String("task_id", actCtx.TaskID),
						elog.String("operator", actCtx.Operator),
						elog.Any("panic", r),
						elog.String("stack", string(stack)),
					)

					if len(handlers) > 0 && handlers[0] != nil {
						handlers[0](ctx, actCtx, r, stack)
					}
					err = fmt.Errorf("flowx execution panic: %v", r)
				}
			}()
			return next(ctx, actCtx)
		}
	}
}
