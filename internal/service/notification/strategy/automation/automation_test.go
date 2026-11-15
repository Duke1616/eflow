package automation_test

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	sendermocks "github.com/Duke1616/eflow/internal/pkg/notification/mocks"
	strategymocks "github.com/Duke1616/eflow/internal/service/notification/mocks"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	"github.com/Duke1616/eflow/internal/service/notification/strategy/automation"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type AutomationTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockBase   *strategymocks.MockService
	mockSender *sendermocks.MockNotificationSender
	strategy   strategy.SendStrategy
}

func (s *AutomationTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBase = strategymocks.NewMockService(s.ctrl)
	s.mockSender = sendermocks.NewMockNotificationSender(s.ctrl)

	s.strategy = automation.NewNotification(s.mockBase, s.mockSender)
	s.mockBase.EXPECT().Logger().Return(elog.DefaultLogger).AnyTimes()
}

func (s *AutomationTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *AutomationTestSuite) TestSend() {
	ctx := context.Background()
	info := strategy.Info{
		FlowContext: strategy.FlowContext{
			InstID: 200,
			CurrentNode: &strategy.CurrentNode{
				NodeID: "auto1",
			},
		},
	}

	// 1. Mock GetNode
	s.mockBase.EXPECT().GetNode(info, "auto1").Return([]*flowdsl.Node{}, &flowdsl.Node{
		ID:       "auto1",
		Type:     flowdsl.NodeAutoTask,
		AutoTask: &flowdsl.AutoTaskConfig{
			IsNotify:     true,
			NotifyMethod: []int64{2}, // ProcessNowSend
		},
	}, nil)

	// 2. Mock IsGlobalNotify
	s.mockBase.EXPECT().IsGlobalNotify(gomock.Any()).Return(true)

	// 3. Mock FetchRequiredData
	s.mockBase.EXPECT().FetchRequiredData(gomock.Any(), gomock.Any(), gomock.Any()).Return(&strategy.NotificationData{
		TName: "测试模板",
		StartUser: domain.User{
			LarkUserId: "user123",
		},
	}, nil)

	// 4. Mock Sender
	s.mockSender.EXPECT().Send(gomock.Any(), gomock.Any()).Return(notification.NewSuccessResponse(0, "success"), nil)

	resp, err := s.strategy.Send(ctx, info)
	s.NoError(err)
	s.NotNil(resp)
	s.Equal("success", resp.Status)
}

func TestAutomationSuite(t *testing.T) {
	suite.Run(t, new(AutomationTestSuite))
}

// TestAutomationNotificationChannelReceiver 自动化结果卡片的收件标识必须随通知渠道切换，
// 且当前渠道解析不出标识时不得投递空收件人。
func TestAutomationNotificationChannelReceiver(t *testing.T) {
	bothIDs := domain.User{Username: "luan", LarkUserId: "fs_luan", WechatUserId: "wx_luan"}
	larkOnly := domain.User{Username: "luan", LarkUserId: "fs_luan"}

	testCases := []struct {
		name         string
		channel      strategy.Channel
		user         domain.User
		wantReceiver string
		wantSend     int
	}{
		{name: "飞书渠道取飞书标识", channel: strategy.ChannelLarkCard, user: bothIDs, wantReceiver: "fs_luan", wantSend: 1},
		{name: "企微渠道取企微标识", channel: strategy.ChannelWechat, user: bothIDs, wantReceiver: "wx_luan", wantSend: 1},
		{name: "渠道缺省按飞书兜底", channel: "", user: bothIDs, wantReceiver: "fs_luan", wantSend: 1},
		{name: "企微渠道缺企微标识不投递", channel: strategy.ChannelWechat, user: larkOnly, wantSend: 0},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockBase := strategymocks.NewMockService(ctrl)
			mockSender := sendermocks.NewMockNotificationSender(ctrl)
			mockBase.EXPECT().Logger().Return(elog.DefaultLogger).AnyTimes()

			info := strategy.Info{
				Channel: testCase.channel,
				FlowContext: strategy.FlowContext{
					InstID:      200,
					CurrentNode: &strategy.CurrentNode{NodeID: "auto1"},
				},
			}

			mockBase.EXPECT().GetNode(gomock.Any(), "auto1").Return([]*flowdsl.Node{}, &flowdsl.Node{
				ID:   "auto1",
				Type: flowdsl.NodeAutoTask,
				AutoTask: &flowdsl.AutoTaskConfig{
					IsNotify:     true,
					NotifyMethod: []int64{automation.ProcessNowSend},
				},
			}, nil)
			mockBase.EXPECT().IsGlobalNotify(gomock.Any()).Return(true)
			mockBase.EXPECT().FetchRequiredData(gomock.Any(), gomock.Any(), gomock.Any()).Return(&strategy.NotificationData{
				TName:     "发布流程",
				StartUser: testCase.user,
			}, nil)

			var sent []notification.Notification
			mockSender.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context,
				msg notification.Notification) (notification.Response, error) {
				sent = append(sent, msg)
				return notification.NewSuccessResponse(0, "success"), nil
			}).Times(testCase.wantSend)

			_, err := automation.NewNotification(mockBase, mockSender).Send(context.Background(), info)
			require.NoError(t, err)
			require.Len(t, sent, testCase.wantSend)
			if testCase.wantSend == 0 {
				return
			}
			require.Equal(t, testCase.wantReceiver, sent[0].Receiver)
			require.Equal(t, notification.Channel(testCase.channel), sent[0].Channel)
		})
	}
}
