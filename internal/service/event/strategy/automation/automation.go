package automation

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/sender"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/errs"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/Duke1616/enotify/notify/feishu"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
)

const (
	// ProcessEndSend 流程结束后发送
	ProcessEndSend = 1
	// ProcessNowSend 当前节点通过直接发送
	ProcessNowSend = 2
)

type Notification struct {
	strategy.Service
	sender sender.NotificationSender
}

func NewNotification(base strategy.Service, sender sender.NotificationSender) *Notification {
	return &Notification{
		Service: base,
		sender:  sender,
	}
}

func (n *Notification) Send(ctx context.Context, info strategy.Info) (notification.Response, error) {
	// 1. 获取当前节点信息
	_, rawProps, err := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeNodeNotConfigured), err.Error()), fmt.Errorf("%w: %v", errs.ErrNodeNotConfigured, err)
	}

	property, err := easyflow.ToNodeProperty[easyflow.AutomationProperty](easyflow.Node{Properties: rawProps})
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeNodeNotConfigured), err.Error()), err
	}

	// 2. 权限与触发校验
	if !n.IsGlobalNotify(info.Workflow) {
		return notification.NewSuccessResponse(0, "全局通知已关闭"), nil
	}

	if !property.IsNotify {
		n.Logger().Debug("【自动化节点】未开启消息通知，正常跳过",
			elog.Int("instID", info.InstID),
			elog.Int64("ticketID", info.Ticket.Id),
			elog.String("nodeID", info.CurrentNode.NodeID),
			elog.String("nodeName", info.CurrentNode.NodeName))
		return notification.NewSuccessResponse(0, "【自动化节点】未开启消息通知，跳过发送"), nil
	}

	if !containsAutoNotifyMethod(property.NotifyMethod, ProcessNowSend) {
		n.Logger().Debug("【自动化节点】未开启即时发送通知模式，正常跳过",
			elog.Int("instID", info.InstID),
			elog.Int64("ticketID", info.Ticket.Id),
			elog.String("nodeID", info.CurrentNode.NodeID),
			elog.String("nodeName", info.CurrentNode.NodeName))
		return notification.NewSuccessResponse(0, "【自动化节点】未开启即时发送通知模式，跳过发送"), nil
	}

	// 3. 获取元数据与自动化任务结果
	nodes, _, _ := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	data, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return notification.NewErrorResponse(string(errs.ErrorCodeFetchDataFailed), err.Error()), fmt.Errorf("%w: %v", errs.ErrFetchData, err)
	}

	// 4. 发送消息
	fields := strategy.BuildAutomationOutputFields(data.AutomationOutput)

	// 每两个字段插入一个空行（保持原有格式）
	formattedFields := make([]notification.Field, 0, len(fields)*2)
	for i, field := range fields {
		formattedFields = append(formattedFields, field)
		if (i+1)%2 == 0 && i < len(fields)-1 {
			formattedFields = append(formattedFields, notification.Field{
				IsShort: false,
				Tag:     "lark_md",
				Content: "",
			})
		}
	}

	return n.sender.Send(ctx, notification.Notification{
		Channel:      notification.Channel(info.Channel),
		ReceiverType: feishu.ReceiveIDTypeUserID,
		WorkFlowID:   info.Workflow.Id,
		Receiver:     data.StartUser.LarkUserId,
		Template: notification.Template{
			Name:     domain.NotifyTypeApproval,
			Title:    rule.GenerateAutoTitle("你提交", data.TName),
			Fields:   formattedFields,
			HideForm: true,
		},
	})
}

func containsAutoNotifyMethod(methods []int64, target int64) bool {
	return slice.Contains(methods, target)
}
