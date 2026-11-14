package wechat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Duke1616/eflow/internal/pkg/notification"
	wechatmocks "github.com/Duke1616/eflow/internal/pkg/notification/provider/wechat/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xen0n/go-workwx"
	"go.uber.org/mock/gomock"
)

func TestProvider_Send(t *testing.T) {
	sdkErr := errors.New("wechat api down")

	testCases := []struct {
		name          string
		src           notification.Notification
		sendErr       error
		wantErr       string
		wantErrIs     error
		wantRecipient *workwx.Recipient
		assertContent bool
		wantContent   string
		wantContains  []string
		wantTruncated bool
	}{
		{
			name: "个人消息默认按 UserID 投递",
			src: notification.Notification{
				Receiver: "wx_alice",
				Template: notification.Template{Title: "审批提醒"},
			},
			wantRecipient: &workwx.Recipient{UserIDs: []string{"wx_alice"}},
			wantContains:  []string{"**审批提醒**"},
		},
		{
			name: "显式 user_id 类型按 UserID 投递",
			src: notification.Notification{
				Receiver:     "wx_bob",
				ReceiverType: notification.ReceiverTypeUser,
				Template:     notification.Template{Title: "抄送通知"},
			},
			wantRecipient: &workwx.Recipient{UserIDs: []string{"wx_bob"}},
		},
		{
			name: "群聊消息按 ChatID 投递",
			src: notification.Notification{
				Receiver:     "chat_123",
				ReceiverType: notification.ReceiverTypeChatGroup,
				Template:     notification.Template{Title: "执行结果"},
			},
			wantRecipient: &workwx.Recipient{ChatID: "chat_123"},
		},
		{
			name:    "接收人为空前置失败不触达 SDK",
			src:     notification.Notification{},
			wantErr: "企微通知接收人为空",
		},
		{
			name: "SDK 失败保留根因",
			src: notification.Notification{
				Receiver: "wx_alice",
				Template: notification.Template{Title: "审批提醒"},
			},
			sendErr:       sdkErr,
			wantErr:       "企微消息发送失败",
			wantErrIs:     sdkErr,
			wantRecipient: &workwx.Recipient{UserIDs: []string{"wx_alice"}},
		},
		{
			name: "模板完整渲染并跳过空白占位字段",
			src: notification.Notification{
				Receiver: "wx_alice",
				Template: notification.Template{
					Title: "审批提醒",
					Fields: []notification.Field{
						{Content: "**申请人:**\n张三"},
						{Content: ""},
						{Content: "**事由:**\n请假"},
					},
					Text:   "请及时处理",
					Remark: "已自动通过",
				},
			},
			wantRecipient: &workwx.Recipient{UserIDs: []string{"wx_alice"}},
			assertContent: true,
			wantContent:   "**审批提醒**\n**申请人:**\n张三\n**事由:**\n请假\n请及时处理\n已自动通过\n",
		},
		{
			name: "空模板渲染为空内容",
			src: notification.Notification{
				Receiver: "wx_alice",
			},
			wantRecipient: &workwx.Recipient{UserIDs: []string{"wx_alice"}},
			assertContent: true,
			wantContent:   "",
		},
		{
			name: "超长内容按字节上限截断且保持合法 UTF-8",
			src: notification.Notification{
				Receiver: "wx_alice",
				Template: notification.Template{
					Fields: []notification.Field{{Content: strings.Repeat("工单数据很长", 500)}},
				},
			},
			wantRecipient: &workwx.Recipient{UserIDs: []string{"wx_alice"}},
			wantTruncated: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSender := wechatmocks.NewMockMarkdownSender(ctrl)
			var gotRecipient *workwx.Recipient
			var gotContent string
			if tc.wantRecipient != nil {
				mockSender.EXPECT().
					SendMarkdownMessage(gomock.Any(), gomock.Any(), false).
					DoAndReturn(func(r *workwx.Recipient, content string, _ bool) error {
						gotRecipient, gotContent = r, content
						return tc.sendErr
					})
			}

			_, err := NewProvider(mockSender).Send(context.Background(), tc.src)

			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				}
			} else {
				require.NoError(t, err)
			}

			if tc.wantRecipient != nil {
				assert.Equal(t, tc.wantRecipient, gotRecipient)
			}
			if tc.assertContent {
				assert.Equal(t, tc.wantContent, gotContent)
			}
			for _, fragment := range tc.wantContains {
				assert.Contains(t, gotContent, fragment)
			}
			if tc.wantTruncated {
				assert.LessOrEqual(t, len(gotContent), maxMarkdownBytes)
				assert.True(t, utf8.ValidString(gotContent))
				assert.True(t, strings.HasSuffix(gotContent, "…"))
			}
		})
	}
}
