package start

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/sender"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/errs"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/gotomicro/ego/core/elog"
)

type Notification struct {
	strategy.Service
	sender sender.NotificationSender
}

// NewNotification 构造流程启动通知策略
func NewNotification(base strategy.Service, sender sender.NotificationSender) *Notification {
	return &Notification{
		Service: base,
		sender:  sender,
	}
}

func (s *Notification) Send(ctx context.Context, info strategy.Info) (notification.Response, error) {
	// 1. 全局通知校验
	if !s.IsGlobalNotify(info.Workflow) {
		return notification.NewSuccessResponse(0, "全局通知已关闭"), nil
	}

	// 2. 开始节点通常通知发起人
	s.Logger().Debug("开始节点发送通知",
		elog.Int("instance_id", info.InstID),
		elog.String("node_id", info.CurrentNode.NodeID))

	// 3. 加载基础通知元数据
	nodes, _, err := s.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeNodeNotConfigured), err.Error()), fmt.Errorf("%w: %v", errs.ErrNodeNotConfigured, err)
	}

	data, err := s.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeFetchDataFailed), err.Error()), err
	}

	title := rule.GenerateTitle(data.StartUser.DisplayName, data.TName)
	fields := rule.GetFields(data.Rules, info.Ticket.Provide.ToUint8(), info.Ticket.Data)

	msg := notification.Notification{
		Channel:    notification.Channel(info.Channel),
		WorkFlowID: info.Workflow.Id,
		Receiver:   data.StartUser.LarkUserId,
		Template: notification.Template{
			Name:     domain.NotifyTypeRevoke,
			Title:    title,
			Fields:   strategy.ConvertRuleFields(fields),
			Values:   notification.GenerateCallbackValues(info.Ticket.Id, "100001", info.Ticket.TenantID),
			HideForm: false, // 设为 false 让撤销卡片的按钮正常显示
		},
	}

	if msg.Receiver != "" {
		if _, sendErr := s.sender.Send(ctx, msg); sendErr != nil {
			return notification.Response{}, sendErr
		}
	}

	return notification.NewSuccessResponse(0, "success"), nil
}
