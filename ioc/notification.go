package ioc

import (
	notificationv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/notification/v1"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/channel"
	"github.com/Duke1616/eflow/internal/pkg/notification/provider"
	"github.com/Duke1616/eflow/internal/pkg/notification/provider/feishu"
	"github.com/Duke1616/eflow/internal/pkg/notification/provider/sequential"
	"github.com/Duke1616/eflow/internal/pkg/notification/sender"
	"github.com/Duke1616/eflow/internal/service/workflow"
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

// InitNotificationSender 初始化通知发送器，提供给底层 Service 和 Event 逻辑使用
// NOTE: 该函数高度聚合了 gRPC 飞书卡片发送、物理连接降级直连飞书卡片等高可用容灾逻辑
func InitNotificationSender(
	larkClient *lark.Client,
	notificationSvc notificationv1.NotificationServiceClient,
	workflowSvc workflow.Service,
) (sender.NotificationSender, error) {

	// 1. 初始化 feishu 相关的 providers
	grpcProvider := feishu.NewGRPCProvider(notificationSvc, workflowSvc)

	larkCardProvider, err := feishu.NewLarkCardProvider(larkClient)
	if err != nil {
		return nil, err
	}

	larkTextProvider, err := feishu.NewLarkTextProvider(larkClient)
	if err != nil {
		return nil, err
	}

	// 2. LarkCard 渠道：优先使用 gRPC 发送消息，失败后自动降级到 LarkCard 直连发送
	larkCardSelector := sequential.NewSelectorBuilder([]provider.Provider{grpcProvider, larkCardProvider})
	larkCardChannel := channel.NewLarkCardChannel(larkCardSelector)

	// 3. LarkText 渠道：仅使用 LarkText 直连发送
	larkTextSelector := sequential.NewSelectorBuilder([]provider.Provider{larkTextProvider})
	larkTextChannel := channel.NewLarkTextChannel(larkTextSelector)

	// 4. 构建分发器并挂载两个渠道
	dispatcher := channel.NewDispatcher(map[notification.Channel]channel.Channel{
		notification.ChannelLarkCard: larkCardChannel,
		notification.ChannelLarkText: larkTextChannel,
	})

	// 5. 实例化并返回最终的通知发送器
	return sender.NewSender(dispatcher), nil
}
