package ticket

import (
	"context"
	"testing"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	usermocks "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1/mocks"
	"github.com/Duke1616/eflow/internal/domain"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/internal/service/engine"
	easyflowDrv "github.com/Duke1616/eflow/internal/service/engine/easyflow"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
	flowxDrv "github.com/Duke1616/eflow/internal/service/engine/flowx"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	flowxMem "github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestQueryService_ListUserInitiated(t *testing.T) {
	testCases := []struct {
		name      string
		username  string
		offset    int64
		limit     int64
		mockSetup func(repo *repomocks.MockTicketRepository,
			engineSvc *enginemocks.MockIEngine,
			userSvc *usermocks.MockUserServiceClient)
		wantCount int
		wantTotal int64
		check     func(t *testing.T, views []domain.TicketTaskView)
	}{
		{
			name:     "空列表直接返回",
			username: "alice",
			offset:   0,
			limit:    10,
			mockSetup: func(repo *repomocks.MockTicketRepository,
				engineSvc *enginemocks.MockIEngine,
				userSvc *usermocks.MockUserServiceClient) {
				repo.EXPECT().ListTicket(gomock.Any(), "alice", gomock.Any(), int64(0), int64(10)).
					Return([]domain.Ticket{}, nil)
				repo.EXPECT().CountTicket(gomock.Any(), "alice", gomock.Any()).
					Return(int64(0), nil)
			},
			wantCount: 0,
			wantTotal: 0,
			check:     func(t *testing.T, views []domain.TicketTaskView) {},
		},
		{
			name:     "聚合流转中与已撤回工单并展示处理人姓名",
			username: "alice",
			offset:   0,
			limit:    10,
			mockSetup: func(repo *repomocks.MockTicketRepository,
				engineSvc *enginemocks.MockIEngine,
				userSvc *usermocks.MockUserServiceClient) {
				tickets := []domain.Ticket{
					{
						Id: 1, CreateBy: "alice", Status: domain.PROCESS,
						Process: domain.Process{EngineType: domain.EngineTypeEasyFlow, InstanceID: domain.NewID(101)},
					},
					{
						Id: 2, CreateBy: "alice", Status: domain.WITHDRAW,
						Process: domain.Process{EngineType: domain.EngineTypeEasyFlow, InstanceID: domain.NewID(102)},
					},
				}
				repo.EXPECT().ListTicket(gomock.Any(), "alice", gomock.Any(), int64(0), int64(10)).
					Return(tickets, nil)
				repo.EXPECT().CountTicket(gomock.Any(), "alice", gomock.Any()).
					Return(int64(2), nil)

				engineSvc.EXPECT().ListPendingSteps(gomock.Any(), []domain.ID{domain.NewID(101), domain.NewID(102)}).
					Return([]domain.TodoTask{
						{InstanceID: domain.NewID(101), NodeName: "组长审批", ApprovedBy: "bob", TaskID: domain.NewID(999)},
					}, nil)

				userSvc.EXPECT().QueryByUsernames(gomock.Any(), &userv1.QueryByUsernamesReq{Usernames: []string{"bob"}}).
					Return(&userv1.QueryUsersResp{
						Users: []*userv1.User{{Username: "bob", DisplayName: "鲍勃"}},
					}, nil)
			},
			wantCount: 2,
			wantTotal: 2,
			check: func(t *testing.T, views []domain.TicketTaskView) {
				// 工单 1 聚合了当前步骤与处理人真实姓名
				assert.Equal(t, int64(1), views[0].Ticket.Id)
				assert.Equal(t, "组长审批", views[0].CurrentStep)
				assert.Equal(t, "鲍勃", views[0].ApprovedBy)
				assert.Equal(t, "999", views[0].TaskID)

				// 工单 2 已撤回，步骤展示为“已撤回”
				assert.Equal(t, int64(2), views[1].Ticket.Id)
				assert.Equal(t, "已撤回", views[1].CurrentStep)
			},
		},
		{
			name:     "工单存在网关并行分支审批时展开为多条记录以支持前端表格跨行合并",
			username: "luankz",
			offset:   0,
			limit:    10,
			mockSetup: func(repo *repomocks.MockTicketRepository,
				engineSvc *enginemocks.MockIEngine,
				userSvc *usermocks.MockUserServiceClient) {
				tickets := []domain.Ticket{
					{
						Id: 2735, CreateBy: "luankz", Status: domain.PROCESS,
						Process: domain.Process{EngineType: domain.EngineTypeEasyFlow, InstanceID: domain.NewID(2001)},
					},
				}
				repo.EXPECT().ListTicket(gomock.Any(), "luankz", gomock.Any(), int64(0), int64(10)).
					Return(tickets, nil)
				repo.EXPECT().CountTicket(gomock.Any(), "luankz", gomock.Any()).
					Return(int64(1), nil)

				// 流程引擎返回两个处于等待审批中的并行节点：后端与前端
				engineSvc.EXPECT().ListPendingSteps(gomock.Any(), []domain.ID{domain.NewID(2001)}).
					Return([]domain.TodoTask{
						{InstanceID: domain.NewID(2001), NodeName: "后端", ApprovedBy: "peicy", TaskID: domain.NewID(5242)},
						{InstanceID: domain.NewID(2001), NodeName: "前端", ApprovedBy: "luankz", TaskID: domain.NewID(5243)},
					}, nil)

				userSvc.EXPECT().QueryByUsernames(gomock.Any(), gomock.Any()).
					Return(&userv1.QueryUsersResp{
						Users: []*userv1.User{
							{Username: "peicy", DisplayName: "裴承运"},
							{Username: "luankz", DisplayName: "栾凯朝"},
						},
					}, nil)
			},
			wantCount: 2, // 同一工单按步骤展开为 2 条视图记录
			wantTotal: 1, // 工单基础分页总数维持 1
			check: func(t *testing.T, views []domain.TicketTaskView) {
				// 第一条视图对应“后端”分支
				assert.Equal(t, int64(2735), views[0].Ticket.Id)
				assert.Equal(t, "后端", views[0].CurrentStep)
				assert.Equal(t, "裴承运", views[0].ApprovedBy)
				assert.Equal(t, "5242", views[0].TaskID)

				// 第二条视图对应“前端”分支，工单主体数据完全一致，供前端根据 ID 合并行
				assert.Equal(t, int64(2735), views[1].Ticket.Id)
				assert.Equal(t, "前端", views[1].CurrentStep)
				assert.Equal(t, "栾凯朝", views[1].ApprovedBy)
				assert.Equal(t, "5243", views[1].TaskID)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repomocks.NewMockTicketRepository(ctrl)
			mockEngine := enginemocks.NewMockIEngine(ctrl)
			mockUser := usermocks.NewMockUserServiceClient(ctrl)

			tc.mockSetup(mockRepo, mockEngine, mockUser)

			svc := NewQueryService(mockRepo, mockEngine, mockUser, nil)
			views, total, err := svc.ListUserInitiated(context.Background(), tc.username, tc.offset, tc.limit)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total)
			assert.Len(t, views, tc.wantCount)
			tc.check(t, views)
		})
	}
}

func TestQueryService_ListAllTodo(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomocks.NewMockTicketRepository(ctrl)
	mockEngine := enginemocks.NewMockIEngine(ctrl)
	mockUser := usermocks.NewMockUserServiceClient(ctrl)

	// 模拟首屏待办包含启动失败工单置顶
	mockEngine.EXPECT().ListTodoTasks(gomock.Any(), domain.TodoQueryReq{
		Username:    "",
		ProcessName: "",
		SortByAsc:   false,
		Offset:      0,
		Limit:       10,
		IsAdmin:     true,
	}).Return([]domain.TodoTask{
		{
			EngineType: domain.EngineTypeEasyFlow,
			InstanceID: domain.NewID(201),
			TaskID:     domain.NewID(888),
			NodeName:   "财务复核",
			ApprovedBy: "charlie",
		},
	}, int64(1), nil)

	mockUser.EXPECT().QueryByUsernames(gomock.Any(), &userv1.QueryByUsernamesReq{Usernames: []string{"charlie"}}).
		Return(&userv1.QueryUsersResp{
			Users: []*userv1.User{{Username: "charlie", DisplayName: "查理"}},
		}, nil)

	mockRepo.EXPECT().ListTicketByProcessInstanceIds(gomock.Any(), []int{201}).
		Return([]domain.Ticket{
			{Id: 10, Process: domain.Process{EngineType: domain.EngineTypeEasyFlow, InstanceID: domain.NewID(201)}},
		}, nil)

	mockRepo.EXPECT().ListProcessStartFailed(gomock.Any(), "", int64(0), int64(10)).
		Return([]domain.Ticket{
			{Id: 99, Status: domain.START_FAILED},
		}, nil)

	mockRepo.EXPECT().CountProcessStartFailed(gomock.Any(), "").
		Return(int64(1), nil)

	svc := NewQueryService(mockRepo, mockEngine, mockUser, nil)
	views, total, err := svc.ListAllTodo(context.Background(), "", "", false, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, views, 2)

	// 首个应当是启动失败置顶
	assert.Equal(t, int64(99), views[0].Ticket.Id)
	assert.Equal(t, "流程启动失败", views[0].CurrentStep)

	// 第二个是正常引擎任务
	assert.Equal(t, int64(10), views[1].Ticket.Id)
	assert.Equal(t, "财务复核", views[1].CurrentStep)
	assert.Equal(t, "查理", views[1].ApprovedBy)
}

func TestFetchPagedParallel(t *testing.T) {
	t.Run("成功并发返回明细与总数", func(t *testing.T) {
		items, total, err := fetchPagedParallel(
			func() ([]string, error) { return []string{"item1", "item2"}, nil },
			func() (int64, error) { return 2, nil },
		)
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.Equal(t, []string{"item1", "item2"}, items)
	})

	t.Run("列表查询失败返回错误", func(t *testing.T) {
		_, _, err := fetchPagedParallel(
			func() ([]string, error) { return nil, assert.AnError },
			func() (int64, error) { return 10, nil },
		)
		require.ErrorIs(t, err, assert.AnError)
	})
}

func TestQueryService_ListUserTodo_CrossEngine(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := repomocks.NewMockTicketRepository(ctrl)
	mockUser := usermocks.NewMockUserServiceClient(ctrl)

	// 1. 创建基于内存存储的真实 FlowX 引擎
	store := flowxMem.NewStorage()
	flowxEng := flowx.NewEngine(store)

	// 部署并启动一条 FlowX 流程实例，生成 Alice 的待办任务
	def, err := flowxEng.Deploy(context.Background(), flowxModel.ProcessDefinition{
		Key:  "wf_test_todo",
		Name: "FlowX待办测试流程",
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "audit_node", Name: "主管审批", Type: flowxModel.NodeApproval, Assignees: []string{"alice"}},
			{ID: "end", Type: flowxModel.NodeEnd},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "audit_node"},
			{Source: "audit_node", Target: "end"},
		},
	})
	require.NoError(t, err)

	flowxInstID, err := flowxEng.Start(context.Background(), flowx.StartReq{
		Key:     def.Key,
		Version: def.Version,
		Starter: "bob",
	})
	require.NoError(t, err)

	// 2. 组装多态统一工作流引擎 (EngineDispatcher) 驱动
	mockEngineRepo := repomocks.NewMockIEngineRepository(ctrl)
	easyflowEngine := easyflowDrv.NewEngine(mockEngineRepo)
	flowxEngine := flowxDrv.NewEngine(flowxEng)
	unifiedEngine := engine.NewEngineDispatcher(easyflowEngine, flowxEngine)

	// Mock EasyFlow 产生一条待办
	mockEngineRepo.EXPECT().TodoList(gomock.Any(), "alice", "", false, 0, 10).
		Return([]domain.Instance{
			{ProcInstID: 101, CurrentNodeName: "组长初审", ApprovedBy: "alice", TaskID: 501},
		}, nil)
	mockEngineRepo.EXPECT().CountTodo(gomock.Any(), "alice", "").
		Return(int64(1), nil)

	// 3. Mock 用户中心解析中文名
	mockUser.EXPECT().QueryByUsernames(gomock.Any(), gomock.Any()).
		Return(&userv1.QueryUsersResp{
			Users: []*userv1.User{{Username: "alice", DisplayName: "爱丽丝"}},
		}, nil)

	// 4. Mock 批量反查工单：EasyFlow 按 ID 查，FlowX 按 Key 查
	mockRepo.EXPECT().ListTicketByProcessInstanceIds(gomock.Any(), []int{101}).
		Return([]domain.Ticket{
			{Id: 1, Process: domain.Process{EngineType: domain.EngineTypeEasyFlow, InstanceID: domain.NewID(101)}},
		}, nil)

	mockRepo.EXPECT().ListTicketByProcessInstanceKeys(gomock.Any(), []string{flowxInstID}).
		Return([]domain.Ticket{
			{Id: 2, Process: domain.Process{EngineType: domain.EngineTypeFlowx, InstanceID: domain.NewID(flowxInstID)}},
		}, nil)

	// 5. 执行跨引擎多态待办查询
	svc := NewQueryService(mockRepo, unifiedEngine, mockUser, nil)
	views, total, err := svc.ListUserTodo(context.Background(), "alice", "", false, 0, 10)
	require.NoError(t, err)

	assert.Equal(t, int64(2), total)
	require.Len(t, views, 2)

	// 验证包含 EasyFlow 待办
	easyView, foundEasy := lo.Find(views, func(v domain.TicketTaskView) bool { return v.Ticket.Id == 1 })
	assert.True(t, foundEasy)
	assert.Equal(t, "501", easyView.TaskID)
	assert.Equal(t, "组长初审", easyView.CurrentStep)
	assert.Equal(t, "爱丽丝", easyView.ApprovedBy)

	// 验证包含 FlowX 待办
	flowxView, foundFlowx := lo.Find(views, func(v domain.TicketTaskView) bool { return v.Ticket.Id == 2 })
	assert.True(t, foundFlowx)
	assert.NotEmpty(t, flowxView.TaskID)
	assert.Equal(t, "主管审批", flowxView.CurrentStep)
	assert.Equal(t, "爱丽丝", flowxView.ApprovedBy)
}
