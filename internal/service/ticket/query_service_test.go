package ticket

import (
	"context"
	"testing"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	usermocks "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1/mocks"
	"github.com/Duke1616/eflow/internal/domain"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
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
			engineSvc *enginemocks.MockService,
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
				engineSvc *enginemocks.MockService,
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
				engineSvc *enginemocks.MockService,
				userSvc *usermocks.MockUserServiceClient) {
				tickets := []domain.Ticket{
					{
						Id: 1, CreateBy: "alice", Status: domain.PROCESS,
						Process: domain.Process{InstanceId: 101},
					},
					{
						Id: 2, CreateBy: "alice", Status: domain.WITHDRAW,
						Process: domain.Process{InstanceId: 102},
					},
				}
				repo.EXPECT().ListTicket(gomock.Any(), "alice", gomock.Any(), int64(0), int64(10)).
					Return(tickets, nil)
				repo.EXPECT().CountTicket(gomock.Any(), "alice", gomock.Any()).
					Return(int64(2), nil)

				engineSvc.EXPECT().ListPendingStepsOfMyTask(gomock.Any(), []int{101, 102}, "alice").
					Return([]domain.Instance{
						{ProcInstID: 101, CurrentNodeName: "组长审批", ApprovedBy: "bob", TaskID: 999},
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
				assert.Equal(t, 999, views[0].TaskID)

				// 工单 2 已撤回，步骤展示为“已撤回”
				assert.Equal(t, int64(2), views[1].Ticket.Id)
				assert.Equal(t, "已撤回", views[1].CurrentStep)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repomocks.NewMockTicketRepository(ctrl)
			mockEngine := enginemocks.NewMockService(ctrl)
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
	mockEngine := enginemocks.NewMockService(ctrl)
	mockUser := usermocks.NewMockUserServiceClient(ctrl)

	// 模拟首屏待办包含启动失败工单置顶
	mockEngine.EXPECT().ListAllTodoTasks(gomock.Any(), "", "", false, 0, 10).
		Return([]domain.Instance{
			{ProcInstID: 201, CurrentNodeName: "财务复核", ApprovedBy: "charlie", TaskID: 888},
		}, int64(1), nil)

	mockUser.EXPECT().QueryByUsernames(gomock.Any(), &userv1.QueryByUsernamesReq{Usernames: []string{"charlie"}}).
		Return(&userv1.QueryUsersResp{
			Users: []*userv1.User{{Username: "charlie", DisplayName: "查理"}},
		}, nil)

	mockRepo.EXPECT().ListTicketByProcessInstanceIds(gomock.Any(), []int{201}).
		Return([]domain.Ticket{
			{Id: 10, Process: domain.Process{InstanceId: 201}},
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
