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
