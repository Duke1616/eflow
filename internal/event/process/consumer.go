package process

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/internal/event"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

// ProcessEventConsumer 流程引擎反射事件消费者
type ProcessEventConsumer struct {
	workFlowSvc workflowSvc.Service
	ticketSvc   ticketSvc.Service
	consumer    mq.Consumer
	logger      *elog.Component
}

// NewProcessEventConsumer 实例化反射事件消费者
func NewProcessEventConsumer(q mq.MQ, workFlowSvc workflowSvc.Service, ticketSvc ticketSvc.Service) (*ProcessEventConsumer, error) {
	groupID := "create_process_instance"
	consumer, err := q.Consumer(event.CreateProcessEventName, groupID)
	if err != nil {
		return nil, err
	}

	return &ProcessEventConsumer{
		consumer:    consumer,
		workFlowSvc: workFlowSvc,
		ticketSvc:   ticketSvc,
		logger:      elog.DefaultLogger,
	}, nil
}

// Start 启动流程引擎实例初始化事件监听消费协程
func (c *ProcessEventConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				c.logger.Error("同步创建流程实例事件失败", elog.Any("err", err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 监听获取工单启动事件并在 easy-workflow 中自动启动实例
func (c *ProcessEventConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}
	var evt event.TicketEvent
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	flow, err := c.workFlowSvc.Find(ctx, evt.WorkflowId)
	if err != nil {
		return fmt.Errorf("查询流程信息失败: %w", err)
	}

	_, err = engine.InstanceStart(flow.ProcessId, "业务申请", flow.Name, evt.Variables)
	if err != nil {
		return fmt.Errorf("启动流程引擎失败: %w", err)
	}

	return nil
}

// Stop 停止消费者并释放连接资源
func (c *ProcessEventConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
