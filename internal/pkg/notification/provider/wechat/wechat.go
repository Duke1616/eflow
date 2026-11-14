package wechat

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/provider"
	"github.com/xen0n/go-workwx"
)

//go:generate mockgen -source=./wechat.go -package=wechatmocks -destination=./mocks/wechat.mock.go -typed MarkdownSender

// MarkdownSender 抽象企微 markdown 消息发送能力，*workwx.WorkwxApp 天然满足。
// 收件人仅设置 ChatID 时 SDK 自动路由到群聊会话接口，否则走应用消息接口。
type MarkdownSender interface {
	SendMarkdownMessage(recipient *workwx.Recipient, content string, isSafe bool) error
}

var _ provider.Provider = (*wechatProvider)(nil)

type wechatProvider struct {
	sender MarkdownSender
}

func NewProvider(sender MarkdownSender) provider.Provider {
	return &wechatProvider{sender: sender}
}

func (p *wechatProvider) Send(_ context.Context, n notification.Notification) (notification.Response, error) {
	recipient, err := buildRecipient(n)
	if err != nil {
		return notification.Response{}, err
	}

	if err = p.sender.SendMarkdownMessage(recipient, renderMarkdown(n.Template), false); err != nil {
		return notification.Response{}, fmt.Errorf("企微消息发送失败: %w", err)
	}

	return notification.Response{}, nil
}

func buildRecipient(n notification.Notification) (*workwx.Recipient, error) {
	if n.Receiver == "" {
		return nil, fmt.Errorf("企微通知接收人为空")
	}
	if n.ReceiverType == notification.ReceiverTypeChatGroup {
		return &workwx.Recipient{ChatID: n.Receiver}, nil
	}
	return &workwx.Recipient{UserIDs: []string{n.Receiver}}, nil
}

// 企微 markdown 消息内容上限 2048 字节，超限会被 API 直接拒绝
const maxMarkdownBytes = 2048

// renderMarkdown 将统一通知模板降级渲染为企微 markdown 文本。
// 企微不支持飞书卡片的交互表单与回调，审批动作仍由 Web/OA 端完成，此处只保证信息可达。
func renderMarkdown(t notification.Template) string {
	var b strings.Builder
	if t.Title != "" {
		b.WriteString("**")
		b.WriteString(t.Title)
		b.WriteString("**\n")
	}
	for _, f := range t.Fields {
		if f.Content == "" {
			continue
		}
		b.WriteString(f.Content)
		b.WriteString("\n")
	}
	if t.Text != "" {
		b.WriteString(t.Text)
		b.WriteString("\n")
	}
	if t.Remark != "" {
		b.WriteString(t.Remark)
		b.WriteString("\n")
	}
	return truncateMarkdown(b.String())
}

func truncateMarkdown(content string) string {
	const suffix = "…"
	if len(content) <= maxMarkdownBytes {
		return content
	}
	// 按 rune 边界回退截断点，避免切碎多字节字符产生非法 UTF-8
	end := maxMarkdownBytes - len(suffix)
	for end > 0 && !utf8.ValidString(content[:end]) {
		end--
	}
	return content[:end] + suffix
}
