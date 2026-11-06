package process

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/service/engine"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

// ProcessEventConsumer 流程引擎启动消息消费者（Consumer-Driven：只依赖 IProcessLauncher 角色契约）
type ProcessEventConsumer struct {
	launcher    engine.IProcessLauncher
	workFlowSvc workflowSvc.Service
	ticketSvc   ticketSvc.Service
	consumer    mq.Consumer
	logger      *elog.Component
}

// NewProcessEventConsumer 实例化流程启动事件消费者
func NewProcessEventConsumer(
	launcher engine.IProcessLauncher,
	workFlowSvc workflowSvc.Service,
	ticketSvc ticketSvc.Service,
	consumer mq.Consumer,
) *ProcessEventConsumer {
	return &ProcessEventConsumer{
		launcher:    launcher,
		workFlowSvc: workFlowSvc,
		ticketSvc:   ticketSvc,
		consumer:    consumer,
		logger:      elog.DefaultLogger,
	}
}

// Start 启动后台反射消费监听协程
func (c *ProcessEventConsumer) Start(ctx context.Context) {
	go mqx.ConsumeLoop(ctx, c.Consume, func(err error) {
		c.logger.Error("流程引擎反射事件消费处理失败", elog.FieldErr(err))
	})
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
	if ctxutil.GetTenantID(ctx) <= 0 {
		return fmt.Errorf("流程创建消息缺少租户 metadata: topic=%s", cm.Topic)
	}

	return c.handleTask(ctx, evt)
}

func (c *ProcessEventConsumer) handleTask(ctx context.Context, evt event.TicketEvent) error {
	// 1. 幂等校验：若工单已绑定流程实例，无需重复启动流程引擎，防止 MQ 重试产生多套孤儿流程
	ticket, err := c.ticketSvc.GetByID(ctx, evt.Id)
	if err != nil {
		return fmt.Errorf("查询待启动工单详情失败: %w", err)
	}
	if ticket.Process.IsBound() {
		c.logger.Info("工单已绑定流程实例，跳过重复启动",
			elog.Int64("ticketId", evt.Id),
			elog.String("instance", ticket.Process.DisplayID()))
		return nil
	}

	flow, err := c.workFlowSvc.Find(ctx, evt.WorkflowId)
	if err != nil {
		return fmt.Errorf("查询流程信息失败: %w", err)
	}

	// 2. 解码事件变量并显式装配关键工单上下文
	variables := evt.DecodeVariables()
	if ticket.CreateBy != "" {
		domain.VarStarter.Set(&variables, ticket.CreateBy)
	}
	if evt.Id > 0 {
		domain.VarTicketID.Set(&variables, evt.Id)
	}

	// 3. 调用流程启动角色契约（多态调度，隔离底层引擎差异）
	launchRes, err := c.launcher.Start(ctx, domain.ProcessLaunchReq{
		EngineType:   flow.ResolveEngineType(),
		TicketId:     evt.Id,
		WorkflowId:   flow.Id,
		ProcessId:    flow.ProcessId,
		WorkflowName: flow.Name,
		Starter:      ticket.CreateBy,
		Variables:    variables,
	})
	if err != nil {
		if markErr := c.ticketSvc.MarkProcessStartFailed(ctx, evt.Id, err.Error()); markErr != nil {
			return fmt.Errorf("启动流程引擎失败: %w; 记录启动失败状态失败: %v", err, markErr)
		}
		c.logger.Error("启动流程引擎失败，已保留工单等待重试",
			elog.Int64("ticketId", evt.Id), elog.FieldErr(err))
		return nil
	}

	// 4. 统一回写引擎类型与全局实例标识，激活流程实例绑定关系
	if err = c.ticketSvc.BindProcess(ctx, evt.Id, launchRes.EngineType, launchRes.InstanceID); err != nil {
		return fmt.Errorf("绑定工单流程引擎实例失败: %w", err)
	}
	return nil
}

// Stop 关闭消费者连接释放物理资源
func (c *ProcessEventConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
