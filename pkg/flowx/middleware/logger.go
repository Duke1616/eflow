package middleware

import (
	"context"
	"time"

	"github.com/gotomicro/ego/core/elog"
)

// LogFunc 自定义日志函数签名
type LogFunc func(ctx context.Context, actCtx *ActionContext, cost time.Duration, err error)

// Logger 构建基于 elog 的结构化流转审计与耗时观测中间件
// 自动记录 ActionType、InstanceID、TaskID、Operator、Cost 等结构化字段，异常时记录 Error 级别日志
func Logger(loggers ...*elog.Component) Middleware {
	var l *elog.Component
	if len(loggers) > 0 && loggers[0] != nil {
		l = loggers[0]
	} else {
		l = elog.DefaultLogger.With(elog.FieldComponentName("flowx"))
	}

	return func(next Handler) Handler {
		return func(ctx context.Context, actCtx *ActionContext) error {
			start := time.Now()
			err := next(ctx, actCtx)
			cost := time.Since(start)

			fields := []elog.Field{
				elog.String("action", string(actCtx.ActionType)),
				elog.String("instance_id", actCtx.InstanceID),
				elog.String("operator", actCtx.Operator),
				elog.Duration("cost", cost),
			}
			if actCtx.TaskID != "" {
				fields = append(fields, elog.String("task_id", actCtx.TaskID))
			}

			if err != nil {
				fields = append(fields, elog.FieldErr(err))
				l.Error("flowx action executed with error", fields...)
			} else {
				l.Info("flowx action executed successfully", fields...)
			}

			return err
		}
	}
}

// CustomLogger 提供函数闭包形式的自定义日志中间件，便于单元测试或接入其他三方日志门面
func CustomLogger(logFn LogFunc) Middleware {
	return func(next Handler) Handler {
		return func(ctx context.Context, actCtx *ActionContext) error {
			start := time.Now()
			err := next(ctx, actCtx)
			cost := time.Since(start)

			if logFn != nil {
				logFn(ctx, actCtx, cost, err)
			}
			return err
		}
	}
}
