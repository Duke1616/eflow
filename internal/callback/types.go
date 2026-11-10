package callback

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
)

const (
	// LarkCallbackEventName 飞书卡片回调事件名
	LarkCallbackEventName = "lark_card_callback_events"
)

// IChannelCallbackHandler 跨渠道交互事件回调处理器泛型接口
// 抹平各平台专有 Payload 与响应结构差异
type IChannelCallbackHandler[Req any, Resp any] interface {
	// Channel 获取当前处理器对应的渠道类型
	Channel() domain.ChannelType
	// OnAction 响应第三方交互回调并驱动流转
	OnAction(ctx context.Context, req Req) (Resp, error)
}
