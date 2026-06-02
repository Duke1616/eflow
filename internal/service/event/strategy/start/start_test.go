package start_test

import (
	"context"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	sendermocks "github.com/Duke1616/eflow/internal/pkg/notification/mocks"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	strategymocks "github.com/Duke1616/eflow/internal/service/event/mocks"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/Duke1616/eflow/internal/service/event/strategy/start"
	"github.com/gotomicro/ego/core/elog"
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
			CurrentNode: &model.Node{NodeID: "start_node"},
			Ticket: domain.Ticket{
				Id:      4001,
				Provide: domain.SYSTEM,
			},
		},
	}

	s.mockBase.EXPECT().IsGlobalNotify(gomock.Any()).Return(true)
	s.mockBase.EXPECT().GetNodeProperty(gomock.Any(), "start_node").Return([]easyflow.Node{}, map[string]interface{}{}, nil)
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
