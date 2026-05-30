package template

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
	"github.com/xen0n/go-workwx"
)

// WechatApprovalCallbackConsumer 企业微信 OA 审批状态回调事件异步消费者
type WechatApprovalCallbackConsumer struct {
	svc      templateSvc.Service
	consumer mq.Consumer
	producer WechatTicketEventProducer
	workApp  *workwx.WorkwxApp
	logger   *elog.Component
}

// NewWechatApprovalCallbackConsumer 构造企业微信 OA 审批回调消费者
func NewWechatApprovalCallbackConsumer(svc templateSvc.Service, q mq.MQ, p WechatTicketEventProducer, workApp *workwx.WorkwxApp) (*WechatApprovalCallbackConsumer, error) {
	groupID := "wechat_oa_callback"
	consumer, err := q.Consumer(WechatCallbackEventName, groupID)
	if err != nil {
		return nil, err
	}
	return &WechatApprovalCallbackConsumer{
		svc:      svc,
		consumer: consumer,
		logger:   elog.DefaultLogger,
		producer: p,
		workApp:  workApp,
	}, nil
}

// Start 启动消息队列消费监听协程
func (c *WechatApprovalCallbackConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				c.logger.Error("创建企业微信工单事件失败", elog.Any("err", err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 监听获取企业微信 OA 审批流回调事件，完成模板自愈绑定并触发详情拉取
func (c *WechatApprovalCallbackConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	var evt workwx.OAApprovalInfo
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	// NOTE: 在此自动自愈式增量获取或创建与企微绑定的工作流表单模版
	if _, err = c.svc.FindOrCreateByWechat(ctx, domain.WechatInfo{
		TemplateId:   evt.TemplateID,
		TemplateName: evt.SpName,
		SpNo:         evt.SpNo,
	}); err != nil {
		c.logger.Error("模版已经存在或新增模版失败", elog.Any("err", err))
		return err
	}

	return c.sendCreateTicketEvent(ctx, evt.SpNo)
}

func (c *WechatApprovalCallbackConsumer) sendCreateTicketEvent(ctx context.Context, spNo string) error {
	// NOTE: 远程调用企微 OA 客户端，抓取本次审批流向的具体详细表单数值
	spInfo, err := c.workApp.GetOAApprovalDetail(spNo)
	if err != nil {
		return err
	}

	// NOTE: 将该审批单详情作为事件，再次写入 wechat_order_events 等待 ticket 队列消费处理
	err = c.producer.Produce(ctx, spInfo)
	if err != nil {
		c.logger.Error("传输企业微信工单 (Ticket) 失败",
			elog.FieldErr(err),
			elog.Any("event", spInfo.SpNo),
		)
	}

	return err
}

// Stop 关闭消费者释放物理连接资源
func (c *WechatApprovalCallbackConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
