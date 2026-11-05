package feishu

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	sendermocks "github.com/Duke1616/eflow/internal/pkg/notification/mocks"
	channelmocks "github.com/Duke1616/eflow/internal/service/channel/mocks"
	"github.com/gotomicro/ego/core/elog"
	larkcallback "github.com/larksuite/oapi-sdk-go/v3/event/dispatcher/callback"
	"github.com/samber/lo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLarkCallbackHandler_Handle_StateGuard(t *testing.T) {
	testCases := []struct {
		name      string
		evt       domain.ChannelActionEvent
		mockSetup func(mockActionSvc *channelmocks.MockIChannelActionService, mockSender *sendermocks.MockNotificationSender)
		wantErr   bool
	}{
		{
			name: "审批操作失败时拦截卡片更新并推送私信告警",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				Action:        domain.ChannelActionPass,
				ChannelUserID: "bcegag66",
				MessageID:     "om_12345",
				TaskID:        5242,
				TicketID:      2735,
				TenantID:      2,
			},
			mockSetup: func(mockActionSvc *channelmocks.MockIChannelActionService, mockSender *sendermocks.MockNotificationSender) {
				// 业务调度返回失败
				mockActionSvc.EXPECT().DispatchAction(gomock.Any(), gomock.Any()).Return(&domain.ChannelActionResult{
					Success:  false,
					Action:   domain.ChannelActionPass,
					Operator: "bcegag66",
					Remark:   "您无权审批此任务",
					Error:    errors.New("权限不匹配"),
				}, nil)

				// 严禁更新待办卡片，必须向操作者推送私信告警
				mockSender.EXPECT().Send(gomock.Any(), gomock.Cond(func(x any) bool {
					notif, ok := x.(notification.Notification)
					if !ok {
						return false
					}
					return notif.Receiver == "bcegag66" &&
						notif.Channel == notification.ChannelLarkText
				})).Return(notification.NewSuccessResponse(0, "ok"), nil)
			},
			wantErr: true,
		},
		{
			name: "调度服务返回未知异常时直接返回错误并阻断",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				Action:        domain.ChannelActionPass,
				ChannelUserID: "bcegag66",
				MessageID:     "om_12345",
				TaskID:        5242,
				TicketID:      2735,
			},
			mockSetup: func(mockActionSvc *channelmocks.MockIChannelActionService, mockSender *sendermocks.MockNotificationSender) {
				mockActionSvc.EXPECT().DispatchAction(gomock.Any(), gomock.Any()).
					Return(nil, errors.New("底层存储异常"))
			},
			wantErr: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockActionSvc := channelmocks.NewMockIChannelActionService(ctrl)
			mockSender := sendermocks.NewMockNotificationSender(ctrl)
			tc.mockSetup(mockActionSvc, mockSender)

			h := &larkCallbackHandler{
				channelActionSvc: mockActionSvc,
				sender:           mockSender,
				logger:           getTestLogger(),
			}

			err := h.Handle(context.Background(), tc.evt)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func getTestLogger() *elog.Component {
	return elog.DefaultLogger.With(elog.FieldComponentName("TestLarkCallbackHandler"))
}

func TestExtractChannelActionEvent_ActionFallback(t *testing.T) {
	// 模拟飞书回执卡片上的“查看进度”按钮交互：Name 为空，Action 存放在 Value["action"] 中
	cte := &larkcallback.CardActionTriggerEvent{
		Event: &larkcallback.CardActionTriggerRequest{
			Action: &larkcallback.CallBackAction{
				Name: "",
				Value: map[string]interface{}{
					"action":    "progress",
					"ticket_id": "2735",
					"task_id":   "5246",
					"tenant_id": "2",
				},
			},
			Operator: &larkcallback.Operator{
				UserID: lo.ToPtr("bcegag66"),
			},
			Context: &larkcallback.Context{
				OpenMessageID: "om_123456",
			},
		},
	}

	evt := extractChannelActionEvent(cte)
	require.Equal(t, domain.ChannelActionProgress, evt.Action)
	require.Equal(t, int64(2735), evt.TicketID)
	require.Equal(t, 5246, evt.TaskID)
	require.Equal(t, int64(2), evt.TenantID)
	require.Equal(t, "bcegag66", evt.ChannelUserID)
}
