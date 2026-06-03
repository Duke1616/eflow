package ticket

import (
	"context"

	"github.com/Duke1616/eflow/internal/event"
)

// TicketEventProducer 工单创建流程事件发送器接口定义
type TicketEventProducer interface {
	Produce(ctx context.Context, evt event.TicketEvent) error
}
