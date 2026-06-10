package ioc

import (
	"github.com/Duke1616/eflow/internal/event"
	templateEvent "github.com/Duke1616/eflow/internal/event/template"
	"github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
	"github.com/xen0n/go-workwx"
)

func InitTicketEventProducer(q mq.MQ) (ticket.TicketEventProducer, error) {
	return mqx.NewGeneralProducer[event.TicketEvent](q, event.CreateProcessEventName)
}

func InitWechatTicketEventProducer(q mq.MQ) (templateEvent.WechatTicketEventProducer, error) {
	return mqx.NewGeneralProducer[*workwx.OAApprovalDetail](q, templateEvent.WechatTicketEventName)
}

func InitTicketStatusModifyEventProducer(q mq.MQ) (mqx.Producer[event.TicketStatusModifyEvent], error) {
	return mqx.NewGeneralProducer[event.TicketStatusModifyEvent](q, event.OrderStatusModifyEventName)
}
