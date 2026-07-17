package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

const workflowExecutionSource = "WORKFLOW"

// ExecuteResultConsumer 消费 etask 工作流执行终态并更新本地执行尝试。
type ExecuteResultConsumer struct {
	consumer mq.Consumer
	svc      taskSvc.Service
	logger   *elog.Component
}

// NewExecuteResultConsumer 创建工作流执行结果消费者。
func NewExecuteResultConsumer(consumer mq.Consumer, svc taskSvc.Service) *ExecuteResultConsumer {
	return &ExecuteResultConsumer{consumer: consumer, svc: svc,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("automation.result.consumer"))}
}

// Start 启动后台消费循环。
func (c *ExecuteResultConsumer) Start(ctx context.Context) {
	go func() {
		for {
			if err := c.Consume(ctx); err != nil {
				if ctx.Err() != nil {
					return
				}
				c.logger.Error("处理自动化执行结果失败", elog.FieldErr(err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 处理一条 etask 完成事件。
func (c *ExecuteResultConsumer) Consume(ctx context.Context) error {
	ctx, message, err := mqx.ConsumeMessage(ctx, c.consumer)
	if err != nil {
		return fmt.Errorf("获取 etask 完成事件失败: %w", err)
	}
	var current event.ExecuteResultEvent
	if err = json.Unmarshal(message.Value, &current); err != nil {
		return fmt.Errorf("解析 etask 完成事件失败: %w", err)
	}
	return c.handle(ctx, current)
}

func (c *ExecuteResultConsumer) handle(ctx context.Context, current event.ExecuteResultEvent) error {
	if current.Source != workflowExecutionSource {
		return nil
	}
	if current.RequestID == "" {
		return fmt.Errorf("工作流完成事件缺少 request_id: execution_id=%d", current.ExecID)
	}
	if ctxutil.GetTenantID(ctx) <= 0 {
		return fmt.Errorf("工作流完成事件缺少租户身份: request_id=%s", current.RequestID)
	}
	status := domain.AttemptStatusFailed
	reason := current.TaskResult
	if current.ExecStatus == string(domain.AttemptStatusSuccess) {
		status = domain.AttemptStatusSuccess
		reason = ""
	}
	_, err := c.svc.CompleteAttempt(ctx, current.RequestID, status, current.TaskResult, reason)
	return err
}

// Stop 关闭消费者。
func (c *ExecuteResultConsumer) Stop(_ context.Context) error { return c.consumer.Close() }
