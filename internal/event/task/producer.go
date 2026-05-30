package task

import (
	"context"

	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
)

// ExecuteResultEventProducer 任务执行结果事件生产者接口定义
type ExecuteResultEventProducer interface {
	Produce(ctx context.Context, evt event.ExecuteResultEvent) error
}

type executeResultEventProducer struct {
	p mqx.Producer[event.ExecuteResultEvent]
}

// NewExecuteResultEventProducer 构造任务执行结果事件发送器
func NewExecuteResultEventProducer(q mq.MQ) (ExecuteResultEventProducer, error) {
	p, err := mqx.NewGeneralProducer[event.ExecuteResultEvent](q, event.ExecuteResultEventName)
	return &executeResultEventProducer{p: p}, err
}

// Produce 发送任务执行结果事件消息到 MQ
func (e *executeResultEventProducer) Produce(ctx context.Context, evt event.ExecuteResultEvent) error {
	return e.p.Produce(ctx, evt)
}
