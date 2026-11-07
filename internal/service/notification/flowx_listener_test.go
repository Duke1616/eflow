package notification

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	pkgNotification "github.com/Duke1616/eflow/internal/pkg/notification"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	wfmocks "github.com/Duke1616/eflow/internal/service/workflow/mocks"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

type mockSendStrategy struct {
	lastInfo strategy.Info
	calls    int
	err      error
}

func (m *mockSendStrategy) Send(_ context.Context, info strategy.Info) (pkgNotification.Response, error) {
	m.calls++
	m.lastInfo = info
	return pkgNotification.NewSuccessResponse(0, "success"), m.err
}

func TestFlowxNotificationListener_InterestIn(t *testing.T) {
	listener := NewFlowxNotificationListener(nil, nil, nil)
	events := listener.InterestIn()
	assert.Contains(t, events, flowx.EventProcessStarted)
	assert.Contains(t, events, flowx.EventTaskCreated)
	assert.Contains(t, events, flowx.EventNodeEntered)
}

func TestFlowxNotificationListener_OnEvent(t *testing.T) {
	testCases := []struct {
		name      string
		event     flowx.Event
		setupMock func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService)
		assertFn  func(t *testing.T, startMock, userMock, ccMock *mockSendStrategy)
	}{
		{
			name: "空实例或无实例ID直接跳过",
			event: flowx.Event{
				Type:     flowx.EventProcessStarted,
				Instance: nil,
			},
			setupMock: func(_ *repomocks.MockTicketRepository, _ *wfmocks.MockService) {},
			assertFn: func(t *testing.T, s, u, c *mockSendStrategy) {
				assert.Equal(t, 0, s.calls)
				assert.Equal(t, 0, u.calls)
				assert.Equal(t, 0, c.calls)
			},
		},
		{
			name: "工单不存在时平滑容错跳过",
			event: flowx.Event{
				Type: flowx.EventProcessStarted,
				Instance: &flowxModel.Instance{
					ID: "inst_non_exist",
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, _ *wfmocks.MockService) {
				repo.EXPECT().DetailByProcessInstance(gomock.Any(), domain.NewID("inst_non_exist")).
					Return(domain.Ticket{}, errors.New("ticket not found"))
			},
			assertFn: func(t *testing.T, s, u, c *mockSendStrategy) {
				assert.Equal(t, 0, s.calls)
			},
		},
		{
			name: "流程启动触发提单通知",
			event: flowx.Event{
				Type: flowx.EventProcessStarted,
				Instance: &flowxModel.Instance{
					ID: "inst_1001",
				},
				Node: &flowxModel.NodeSpec{ID: "start_node", Name: "Start"},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService) {
				repo.EXPECT().DetailByProcessInstance(gomock.Any(), domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c *mockSendStrategy) {
				assert.Equal(t, 1, s.calls)
				assert.Equal(t, strategy.Start, s.lastInfo.NodeName)
				assert.Equal(t, int64(88), s.lastInfo.Ticket.Id)
				assert.Equal(t, "start_node", s.lastInfo.CurrentNode.NodeID)
			},
		},
		{
			name: "任务生成触发审批待办通知",
			event: flowx.Event{
				Type: flowx.EventTaskCreated,
				Instance: &flowxModel.Instance{
					ID: "inst_1001",
				},
				Task: &flowxModel.Task{
					ID:       "task_123",
					NodeID:   "leader_audit",
					NodeName: "主管审批",
					Assignee: "bob",
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService) {
				repo.EXPECT().DetailByProcessInstance(gomock.Any(), domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c *mockSendStrategy) {
				assert.Equal(t, 1, u.calls)
				assert.Equal(t, strategy.User, u.lastInfo.NodeName)
				assert.Equal(t, "leader_audit", u.lastInfo.CurrentNode.NodeID)
				assert.Equal(t, []string{"bob"}, u.lastInfo.CurrentNode.UserIDs)
			},
		},
		{
			name: "抄送节点触发抄送通知",
			event: flowx.Event{
				Type: flowx.EventNodeEntered,
				Instance: &flowxModel.Instance{
					ID: "inst_1001",
				},
				Node: &flowxModel.NodeSpec{
					ID:        "cc_node",
					Name:      "抄送项目经理",
					Type:      flowxModel.NodeCarbonCopy,
					Assignees: []string{"pm_zhang"},
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService) {
				repo.EXPECT().DetailByProcessInstance(gomock.Any(), domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c *mockSendStrategy) {
				assert.Equal(t, 1, c.calls)
				assert.Equal(t, strategy.CarbonCopy, c.lastInfo.NodeName)
				assert.Equal(t, "cc_node", c.lastInfo.CurrentNode.NodeID)
				assert.Equal(t, []string{"pm_zhang"}, c.lastInfo.CurrentNode.UserIDs)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := repomocks.NewMockTicketRepository(ctrl)
			wf := wfmocks.NewMockService(ctrl)
			tc.setupMock(repo, wf)

			startMock := &mockSendStrategy{}
			userMock := &mockSendStrategy{}
			ccMock := &mockSendStrategy{}
			dispatcher := strategy.NewDispatcher(userMock, nil, startMock, nil, ccMock, nil)

			listener := NewFlowxNotificationListener(repo, wf, dispatcher)
			err := listener.OnEvent(context.Background(), tc.event)
			require.NoError(t, err)

			tc.assertFn(t, startMock, userMock, ccMock)
		})
	}
}
