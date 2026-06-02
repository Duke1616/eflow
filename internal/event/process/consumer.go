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
	"github.com/Duke1616/eflow/pkg/mqx"
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
func NewProcessEventConsumer(workFlowSvc workflowSvc.Service, ticketSvc ticketSvc.Service, consumer mq.Consumer) *ProcessEventConsumer {
	return &ProcessEventConsumer{
		workFlowSvc: workFlowSvc,
		ticketSvc:   ticketSvc,
		consumer:    consumer,
		logger:      elog.DefaultLogger,
	}
}

// Start 启动后台反射消费监听协程
func (c *ProcessEventConsumer) Start(ctx context.Context) {
	go func() {
		for {
			if err := c.Consume(ctx); err != nil {
				c.logger.Error("流程引擎反射事件消费处理失败", elog.FieldErr(err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 消费 Kafka 中的单条工单创建流程事件并触发引擎启动
func (c *ProcessEventConsumer) Consume(ctx context.Context) error {
	ctx, cm, err := mqx.ConsumeMessage(ctx, c.consumer)
	if err != nil {
		return fmt.Errorf("获取流程创建消息失败: %w", err)
	}

	var evt event.TicketEvent
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("反序列化流程创建消息失败 %w", err)
	}

	return c.handleTask(ctx, evt)
}

func (c *ProcessEventConsumer) handleTask(ctx context.Context, evt event.TicketEvent) error {
	flow, err := c.workFlowSvc.Find(ctx, evt.WorkflowId)
	if err != nil {
		return fmt.Errorf("查询流程信息失败: %w", err)
	}

	instanceId, err := engine.InstanceStart(flow.ProcessId, "业务申请", flow.Name, evt.Variables)
	if err != nil {
		return fmt.Errorf("启动流程引擎失败: %w", err)
	}

	// 将生成的流程引擎实例 ID 回写反登记到对应工单上，激活流程实例绑定关系
	if err = c.ticketSvc.BindProcessInstanceID(ctx, evt.Id, instanceId); err != nil {
		return fmt.Errorf("绑定工单流程引擎实例ID失败: %w", err)
	}
	return nil
}

// Stop 关闭消费者连接释放物理资源
func (c *ProcessEventConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
