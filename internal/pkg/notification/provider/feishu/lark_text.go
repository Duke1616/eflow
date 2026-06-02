package feishu

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/provider"
	"github.com/Duke1616/enotify/notify"
	"github.com/Duke1616/enotify/notify/feishu"
	lark "github.com/larksuite/oapi-sdk-go/v3"
)

type larkTextProvider struct {
	handler notify.Handler
}

func NewLarkTextProvider(lark *lark.Client) (provider.Provider, error) {
	handler, err := feishu.NewHandler(lark)
	if err != nil {
		return nil, err
	}

	return &larkTextProvider{
		handler: handler,
	}, nil
}

func (f *larkTextProvider) Send(ctx context.Context, src notification.Notification) (
	notification.Response, error) {
	content, err := json.Marshal(map[string]string{"text": src.Template.Text})
	if err != nil {
		return notification.Response{}, fmt.Errorf("序列化文本消息失败: %w", err)
	}

	msg := feishu.NewCreateBuilder(src.Receiver).SetReceiveIDType(src.ReceiverType).
		SetContent(feishu.NewFeishuCustom("text", string(content))).Build()

	if err = f.handler.Send(ctx, msg); err != nil {
		return notification.Response{}, fmt.Errorf("触发发送信息失败: %w", err)
	}

	return notification.Response{}, nil
}
