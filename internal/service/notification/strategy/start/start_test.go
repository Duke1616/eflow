package start_test

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	sendermocks "github.com/Duke1616/eflow/internal/pkg/notification/mocks"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	strategymocks "github.com/Duke1616/eflow/internal/service/notification/mocks"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	"github.com/Duke1616/eflow/internal/service/notification/strategy/start"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
)

type StartTestSuite struct {
	suite.Suite
	ctrl       *gomock.Controller
	mockBase   *strategymocks.MockService
	mockSender *sendermocks.MockNotificationSender
	strategy   strategy.SendStrategy
}

func (s *StartTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockBase = strategymocks.NewMockService(s.ctrl)
	s.mockSender = sendermocks.NewMockNotificationSender(s.ctrl)

	s.strategy = start.NewNotification(s.mockBase, s.mockSender)
	s.mockBase.EXPECT().Logger().Return(elog.DefaultLogger).AnyTimes()
}

func (s *StartTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *StartTestSuite) TestSend_RevokeCardToStarter() {
	ctx := context.Background()
	info := strategy.Info{
		FlowContext: strategy.FlowContext{
			InstID:      301,
			CurrentNode: &strategy.CurrentNode{NodeID: "start_node"},
			Ticket: domain.Ticket{
				Id:      4001,
				Provide: domain.SYSTEM,
			},
		},
	}

	s.mockBase.EXPECT().IsGlobalNotify(gomock.Any()).Return(true)
	s.mockBase.EXPECT().GetNode(gomock.Any(), "start_node").Return([]*flowdsl.Node{}, &flowdsl.Node{ID: "start_node"}, nil)
	s.mockBase.EXPECT().FetchRequiredData(gomock.Any(), gomock.Any(), gomock.Any()).Return(&strategy.NotificationData{
		TName: "资源申请",
		StartUser: domain.User{
			DisplayName: "陆安",
			LarkUserId:  "fs_luan",
		},
		Rules: []rule.Rule{
			{Title: "申请理由", Field: "reason", Type: "input"},
		},
	}, nil)

	s.mockSender.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context,
		msg notification.Notification) (notification.Response, error) {
		s.Equal("fs_luan", msg.Receiver)
		// 重要：开始节点发送的是撤销模版
		s.Equal(domain.NotifyTypeRevoke, msg.Template.Name)
		// 验证标题生成
		s.Contains(msg.Template.Title, "陆安")
		s.Contains(msg.Template.Title, "资源申请")
		// 验证 task_id 注入 (虽然是硬编码，但流程撤销需要它)
		foundTaskID := false
		for _, v := range msg.Template.Values {
			if v.Key == "task_id" && v.Value == "100001" {
				foundTaskID = true
			}
		}
		s.True(foundTaskID)

		return notification.Response{}, nil
	})

	resp, err := s.strategy.Send(ctx, info)
	s.NoError(err)
	s.Equal("success", resp.Status)
}

func TestStartSuite(t *testing.T) {
	suite.Run(t, new(StartTestSuite))
}

// TestStartNotificationChannelReceiver 发起人收件标识必须随通知渠道切换：
// 飞书与企微的用户 ID 命名空间互不相通，硬编码飞书 ID 会让企微渠道投递到不存在的人。
func TestStartNotificationChannelReceiver(t *testing.T) {
	bothIDs := domain.User{Username: "luan", DisplayName: "陆安", LarkUserId: "fs_luan", WechatUserId: "wx_luan"}
	larkOnly := domain.User{Username: "luan", DisplayName: "陆安", LarkUserId: "fs_luan"}

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
					InstID:      301,
					CurrentNode: &strategy.CurrentNode{NodeID: "start_node"},
					Ticket:      domain.Ticket{Id: 4001, Provide: domain.SYSTEM},
				},
			}

			mockBase.EXPECT().IsGlobalNotify(gomock.Any()).Return(true)
			mockBase.EXPECT().GetNode(gomock.Any(), "start_node").Return([]*flowdsl.Node{}, &flowdsl.Node{ID: "start_node"}, nil)
			mockBase.EXPECT().FetchRequiredData(gomock.Any(), gomock.Any(), gomock.Any()).Return(&strategy.NotificationData{
				TName:     "资源申请",
				StartUser: testCase.user,
			}, nil)

			var sent []notification.Notification
			mockSender.EXPECT().Send(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context,
				msg notification.Notification) (notification.Response, error) {
				sent = append(sent, msg)
				return notification.Response{}, nil
			}).Times(testCase.wantSend)

			_, err := start.NewNotification(mockBase, mockSender).Send(context.Background(), info)
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
