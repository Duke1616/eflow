package notification

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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
	assert.Contains(t, events, flowx.EventTaskRejected)
	assert.Contains(t, events, flowx.EventTaskPassed)
}

func TestFlowxNotificationListener_OnEvent(t *testing.T) {
	testCases := []struct {
		name      string
		event     flowx.Event
		setupMock func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService)
		assertFn  func(t *testing.T, startMock, userMock, ccMock, chatMock *mockSendStrategy)
	}{
		{
			name: "空实例或无实例ID直接跳过",
			event: flowx.Event{
				Type:     flowx.EventProcessStarted,
				Instance: nil,
			},
			setupMock: func(_ *repomocks.MockTicketRepository, _ *wfmocks.MockService) {},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
				assert.Equal(t, 0, s.calls)
				assert.Equal(t, 0, u.calls)
				assert.Equal(t, 0, c.calls)
				assert.Equal(t, 0, ch.calls)
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
				repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_non_exist")).
					Return(domain.Ticket{}, errors.New("ticket not found"))
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
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
				repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
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
				repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
				assert.Equal(t, 1, u.calls)
				assert.Equal(t, strategy.User, u.lastInfo.NodeName)
				assert.Equal(t, "leader_audit", u.lastInfo.CurrentNode.NodeID)
				assert.Equal(t, []string{"bob"}, u.lastInfo.CurrentNode.UserIDs)
			},
		},
		{
			name: "处理任务驳回事件触发驳回通知并携带最新驳回人与原因",
			event: flowx.Event{
				Type: flowx.EventTaskRejected,
				Instance: &flowxModel.Instance{
					ID:          "inst_reject_001",
					BusinessKey: "1003",
				},
				Task: &flowxModel.Task{
					NodeID:     "node_mgr",
					NodeName:   "主管审批",
					Assignee:   "bob_mgr",
					Comment:    "预算超支，请削减",
					FinishTime: time.Date(2026, 11, 11, 10, 0, 0, 0, time.UTC),
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService) {
				repo.EXPECT().Detail(gomock.Any(), int64(1003)).
					Return(domain.Ticket{
						Id:         1003,
						WorkflowId: 20,
						CreateBy:   "alice",
					}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(20)).
					Return(domain.Workflow{Id: 20, Name: "采购流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
				assert.Equal(t, 1, s.calls)
				assert.Equal(t, "node_mgr", s.lastInfo.CurrentNode.NodeID)
				require.NotNil(t, s.lastInfo.Ticket.LatestReject)
				assert.Equal(t, "bob_mgr", s.lastInfo.Ticket.LatestReject.RejectedBy)
				assert.Equal(t, "预算超支，请削减", s.lastInfo.Ticket.LatestReject.Reason)
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
				repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
				assert.Equal(t, 1, c.calls)
				assert.Equal(t, strategy.CarbonCopy, c.lastInfo.NodeName)
				assert.Equal(t, "cc_node", c.lastInfo.CurrentNode.NodeID)
				assert.Equal(t, []string{"pm_zhang"}, c.lastInfo.CurrentNode.UserIDs)
			},
		},
		{
			name: "群聊通知节点触发群通知",
			event: flowx.Event{
				Type: flowx.EventNodeEntered,
				Instance: &flowxModel.Instance{
					ID: "inst_1001",
				},
				Node: &flowxModel.NodeSpec{
					ID:        "chat_node",
					Name:      "研发告警群通知",
					Type:      flowxModel.NodeChatGroup,
					Assignees: []string{},
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService) {
				repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_1001")).
					Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
				assert.Equal(t, 1, ch.calls)
				assert.Equal(t, strategy.ChatGroup, ch.lastInfo.NodeName)
				assert.Equal(t, "chat_node", ch.lastInfo.CurrentNode.NodeID)
			},
		},
		{
			name: "融合实例变量时隔离系统内置变量不污染表单数据",
			event: flowx.Event{
				Type: flowx.EventNodeEntered,
				Instance: &flowxModel.Instance{
					ID: "inst_1001",
					Variables: map[string]any{
						"starter":          "luankz",
						"template_name":    "产线环境发版",
						"ticket_id":        int64(1130),
						"operator":         "admin",
						"comment":          "同意",
						"deploy_branch":    "main",
						"custom_parameter": "prod-v1",
					},
				},
				Node: &flowxModel.NodeSpec{
					ID:        "cc_node",
					Name:      "抄送节点",
					Type:      flowxModel.NodeCarbonCopy,
					Assignees: []string{"auditor"},
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository, wf *wfmocks.MockService) {
				repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_1001")).
					Return(domain.Ticket{
						Id:         1130,
						WorkflowId: 10,
						CreateBy:   "luankz",
						TenantID:   1,
						Data: domain.TicketData{
							"base_field": "initial_value",
						},
					}, nil)
				wf.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)
			},
			assertFn: func(t *testing.T, s, u, c, ch *mockSendStrategy) {
				assert.Equal(t, 1, c.calls)
				ticketData := c.lastInfo.Ticket.Data
				// 业务字段正常融合
				assert.Equal(t, "initial_value", ticketData["base_field"])
				assert.Equal(t, "main", ticketData["deploy_branch"])
				assert.Equal(t, "prod-v1", ticketData["custom_parameter"])
				// 系统内置流程变量严格被排除，不污染业务表单
				assert.NotContains(t, ticketData, "starter")
				assert.NotContains(t, ticketData, "template_name")
				assert.NotContains(t, ticketData, "ticket_id")
				assert.NotContains(t, ticketData, "operator")
				assert.NotContains(t, ticketData, "comment")
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
			chatMock := &mockSendStrategy{}
			dispatcher := strategy.NewDispatcher(userMock, nil, startMock, chatMock, ccMock, nil)

			listener := NewFlowxNotificationListener(repo, wf, dispatcher, WithSyncMode(true))
			err := listener.OnEvent(context.Background(), tc.event)
			require.NoError(t, err)

			tc.assertFn(t, startMock, userMock, ccMock, chatMock)
		})
	}
}

// TestFlowxNotificationListener_AutomationTaskPassed 覆盖 TASK_PASSED 的节点类型收窄矩阵：
// 该事件由自动化/审批/抄送/群聊四类节点共同广播，只有自动化节点才允许给发起人推执行结果卡片。
func TestFlowxNotificationListener_AutomationTaskPassed(t *testing.T) {
	const instanceKey = "7395001234567890123"

	testCases := []struct {
		name     string
		node     *flowxModel.NodeSpec
		wantCall int
	}{
		{
			name:     "自动化节点通过推送执行结果",
			node:     &flowxModel.NodeSpec{ID: "auto_deploy", Name: "自动发布", Type: flowxModel.NodeTask},
			wantCall: 1,
		},
		{
			name:     "审批节点通过不推自动化卡片",
			node:     &flowxModel.NodeSpec{ID: "leader_audit", Name: "主管审批", Type: flowxModel.NodeApproval},
			wantCall: 0,
		},
		{
			name:     "抄送节点通过不推自动化卡片",
			node:     &flowxModel.NodeSpec{ID: "cc_node", Name: "抄送知会", Type: flowxModel.NodeCarbonCopy},
			wantCall: 0,
		},
		{
			name:     "群聊节点通过不推自动化卡片",
			node:     &flowxModel.NodeSpec{ID: "chat_node", Name: "群通知", Type: flowxModel.NodeChatGroup},
			wantCall: 0,
		},
		{
			name:     "缺失节点信息平滑跳过",
			node:     nil,
			wantCall: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := repomocks.NewMockTicketRepository(ctrl)
			wf := wfmocks.NewMockService(ctrl)
			repo.EXPECT().DetailByEngineInstance(gomock.Any(), domain.EngineTypeFlowx, domain.NewID(instanceKey)).
				Return(domain.Ticket{Id: 88, WorkflowId: 10, CreateBy: "alice", TenantID: 1}, nil)
			wf.EXPECT().Find(gomock.Any(), int64(10)).
				Return(domain.Workflow{Id: 10, Name: "发布流程"}, nil)

			startMock := &mockSendStrategy{}
			userMock := &mockSendStrategy{}
			autoMock := &mockSendStrategy{}
			ccMock := &mockSendStrategy{}
			chatMock := &mockSendStrategy{}
			dispatcher := strategy.NewDispatcher(userMock, autoMock, startMock, chatMock, ccMock, nil)

			listener := NewFlowxNotificationListener(repo, wf, dispatcher, WithSyncMode(true))
			require.NoError(t, listener.OnEvent(context.Background(), flowx.Event{
				Type:     flowx.EventTaskPassed,
				Instance: &flowxModel.Instance{ID: instanceKey},
				Node:     tc.node,
				Task:     &flowxModel.Task{ID: "task_auto", NodeID: "auto_deploy", Assignee: "system"},
			}))

			require.Equal(t, tc.wantCall, autoMock.calls)
			assert.Equal(t, 0, startMock.calls)
			assert.Equal(t, 0, userMock.calls)
			assert.Equal(t, 0, ccMock.calls)
			assert.Equal(t, 0, chatMock.calls)

			if tc.wantCall == 0 {
				return
			}
			assert.Equal(t, strategy.Automation, autoMock.lastInfo.NodeName)
			assert.Equal(t, "auto_deploy", autoMock.lastInfo.CurrentNode.NodeID)
			assert.Equal(t, "自动发布", autoMock.lastInfo.CurrentNode.NodeName)
			assert.Equal(t, []string{"alice"}, autoMock.lastInfo.CurrentNode.UserIDs)
			assert.Equal(t, int64(88), autoMock.lastInfo.Ticket.Id)
			// flowx 实例标识是雪花字符串，必须原样透传给策略，否则自动化结果按 instance_key 查不到
			assert.Equal(t, int64(7395001234567890123), autoMock.lastInfo.ResolveInstanceID().Int64())
			assert.Equal(t, domain.EngineTypeFlowx, autoMock.lastInfo.Ticket.Process.EngineType)
		})
	}
}

func TestFlowxNotificationListener_AsyncSerialOrdering(t *testing.T) {
	listenerObj := NewFlowxNotificationListener(nil, nil, nil)
	listener, ok := listenerObj.(*FlowxNotificationListener)
	require.True(t, ok)

	const taskCount = 10
	results := make([]int, 0, taskCount)
	var mu sync.Mutex
	done := make(chan struct{})

	for i := 0; i < taskCount; i++ {
		val := i
		listener.dispatchAsync("inst_test_serial", func() {
			mu.Lock()
			results = append(results, val)
			if len(results) == taskCount {
				close(done)
			}
			mu.Unlock()
		})
	}

	select {
	case <-done:
	case <-time.After(time.Second * 3):
		t.Fatal("等待异步串行队列执行超时")
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, results, taskCount)
	for i := 0; i < taskCount; i++ {
		assert.Equal(t, i, results[i], "基于 ekit 阻塞队列的消费必须严格遵循 FIFO 保序")
	}
}
