package ticket

import (
	"context"

	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
)

// TicketEventProducer 工单创建流程启动事件发送器接口定义
type TicketEventProducer interface {
	Produce(ctx context.Context, evt event.TicketEvent) error
}

type ticketEventProducer struct {
	p mqx.Producer[event.TicketEvent]
}

// NewTicketEventProducer 实例化流程启动事件发送器
func NewTicketEventProducer(q mq.MQ) (TicketEventProducer, error) {
	p, err := mqx.NewGeneralProducer[event.TicketEvent](q, event.CreateProcessEventName)
	return &ticketEventProducer{p: p}, err
}

// Produce 发送工单事件消息到 MQ
func (t *ticketEventProducer) Produce(ctx context.Context, evt event.TicketEvent) error {
	return t.p.Produce(ctx, evt)
}
