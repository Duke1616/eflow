package template

import (
	"context"

	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
	"github.com/xen0n/go-workwx"
)

// WechatTicketEventProducer 企业微信 OA 审批流审批单详情事件生产者接口定义
type WechatTicketEventProducer interface {
	Produce(ctx context.Context, evt *workwx.OAApprovalDetail) error
}

type wechatTicketEventProducer struct {
	p mqx.Producer[*workwx.OAApprovalDetail]
}

// NewWechatTicketEventProducer 构造企业微信 OA 审批流详情事件发送器
func NewWechatTicketEventProducer(q mq.MQ) (WechatTicketEventProducer, error) {
	p, err := mqx.NewGeneralProducer[*workwx.OAApprovalDetail](q, WechatTicketEventName)
	return &wechatTicketEventProducer{p: p}, err
}

// Produce 发送企微 OA 审批详情数据至消息队列
func (e *wechatTicketEventProducer) Produce(ctx context.Context, evt *workwx.OAApprovalDetail) error {
	return e.p.Produce(ctx, evt)
}
