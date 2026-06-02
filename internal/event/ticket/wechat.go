package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
	"github.com/xen0n/go-workwx"
)

// WechatTicketConsumer 企业微信 OA 审批流回调事件消费者
type WechatTicketConsumer struct {
	svc         ticketSvc.Service
	templateSvc templateSvc.Service
	userSvc     UserService
	consumer    mq.Consumer
	logger      *elog.Component
}

// NewWechatTicketConsumer 构造企业微信 OA 审批流回调事件消费者
func NewWechatTicketConsumer(svc ticketSvc.Service, templateSvc templateSvc.Service, userSvc UserService, consumer mq.Consumer) *WechatTicketConsumer {
	return &WechatTicketConsumer{
		svc:         svc,
		consumer:    consumer,
		userSvc:     userSvc,
		templateSvc: templateSvc,
		logger:      elog.DefaultLogger,
	}
}

// Start 启动企业微信工单同步事件消费协程
func (c *WechatTicketConsumer) Start(ctx context.Context) {
	go func() {
		for {
			err := c.Consume(ctx)
			if err != nil {
				c.logger.Error("同步企业微信工单创建工单事件失败", elog.Any("err", err))
				time.Sleep(time.Second)
			}
		}
	}()
}

// Consume 监听消费单条企业微信 OA 同步消息
func (c *WechatTicketConsumer) Consume(ctx context.Context) error {
	ctx, cm, err := mqx.ConsumeMessage(ctx, c.consumer)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}

	var evt workwx.OAApprovalDetail
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}

	// NOTE: 转换企业微信 OA 表单信息为 key-value map 格式
	data, err := wechatMarshal(evt)
	if err != nil {
		return fmt.Errorf("数据转换失败: %w", err)
	}

	// NOTE: 查询本地绑定了对应外部企业微信 OA 模板 ID 的模版
	t, err := c.templateSvc.DetailTemplateByExternalTemplateId(ctx, evt.TemplateID)
	if err != nil {
		return fmt.Errorf("查看模版信息错误: %w", err)
	}

	// NOTE: 通过微信 ID 查找本地绑定的系统用户账号
	wUser, err := c.userSvc.FindByWechatUser(ctx, evt.Applicant.UserID)
	if err != nil {
		c.logger.Error("未找到绑定的本地用户，降级使用微信UserID作为申请人名",
			elog.String("user", evt.Applicant.UserID), elog.FieldErr(err))
		wUser = &userv1.User{
			Username: evt.Applicant.UserID,
		}
	}

	// NOTE: 写入并初始化本地 Ticket 工单流转实例
	_, err = c.svc.CreateBizTicket(ctx, domain.Ticket{
		CreateBy:   wUser.Username,
		TemplateId: t.Id,
		WorkflowId: t.WorkflowId,
		Data:       data,
		Status:     domain.START,
		Provide:    domain.WECHAT,
	})
	if err != nil {
		return err
	}

	return nil
}

func wechatMarshal(oaDetail workwx.OAApprovalDetail) (map[string]interface{}, error) {
	evtData, err := json.Marshal(oaDetail)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	err = json.Unmarshal(evtData, &data)
	return data, err
}

// Stop 关闭消费者连接释放物理资源
func (c *WechatTicketConsumer) Stop(_ context.Context) error {
	return c.consumer.Close()
}
