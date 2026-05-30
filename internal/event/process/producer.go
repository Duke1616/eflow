package process

import (
	"context"

	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
)

// OrderStatusModifyEventProducer 工单流程状态变更事件生产者
type OrderStatusModifyEventProducer interface {
	Produce(ctx context.Context, evt event.TicketStatusModifyEvent) error
}

type orderStatusModifyEventProducer struct {
	p mqx.Producer[event.TicketStatusModifyEvent]
}

// NewOrderStatusModifyEventProducer 构造流程状态修改事件发送器
func NewOrderStatusModifyEventProducer(q mq.MQ) (OrderStatusModifyEventProducer, error) {
	p, err := mqx.NewGeneralProducer[event.TicketStatusModifyEvent](q, event.OrderStatusModifyEventName)
	return &orderStatusModifyEventProducer{p: p}, err
}

// Produce 发送状态变更事件消息到 MQ
func (o *orderStatusModifyEventProducer) Produce(ctx context.Context, evt event.TicketStatusModifyEvent) error {
	return o.p.Produce(ctx, evt)
}
