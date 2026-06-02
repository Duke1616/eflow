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
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

// ExecuteResultConsumer 任务执行结果异步消费者，用于实时同步底端 Task 执行器的状态变更并驱动工作流状态演进
type ExecuteResultConsumer struct {
	consumer mq.Consumer
	svc      taskSvc.Service
	logger   *elog.Component
}

// NewExecuteResultConsumer 构造任务执行结果消费者
func NewExecuteResultConsumer(consumer mq.Consumer, svc taskSvc.Service) *ExecuteResultConsumer {
	return &ExecuteResultConsumer{
		consumer: consumer,
		svc:      svc,
		logger:   elog.DefaultLogger,
	}
}

// Start 启动后台消费协程
func (c *ExecuteResultConsumer) Start(ctx context.Context) {
	go func() {
		for {
			if err := c.Consume(ctx); err != nil {
				c.logger.Error("同步修改任务执行状态失败", elog.FieldErr(err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 消费 Kafka 中的单条执行状态消息并触发本地业务状态机扭转
func (c *ExecuteResultConsumer) Consume(ctx context.Context) error {
	ctx, cm, err := mqx.ConsumeMessage(ctx, c.consumer)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	var evt event.ExecuteResultEvent
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	// NOTE: 根据任务执行状态映射触发节点条件，成功/失败分支在这里进行决策扭转
	triggerPosition := domain.TriggerPositionTaskExecutionSuccess
	if domain.TaskStatus(evt.Status) == domain.FAILED {
		triggerPosition = domain.TriggerPositionTaskExecutionFailed
	}

	_, err = c.svc.UpdateTaskStatus(ctx, domain.TaskResult{
		Id:              evt.TaskId,
		Result:          evt.Result,
		WantResult:      evt.WantResult,
		TriggerPosition: triggerPosition.ToString(),
		Status:          domain.TaskStatus(evt.Status),
	})
	return err
}

// Stop 关闭消费者连接释放物理资源
func (c *ExecuteResultConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
