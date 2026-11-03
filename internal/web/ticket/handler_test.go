package ticket

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeUserService struct {
	userv1.UserServiceClient
	user *userv1.User
	err  error
}

func (f *fakeUserService) QueryById(_ context.Context, _ *userv1.QueryByIdReq, _ ...grpc.CallOption) (*userv1.QueryUserResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &userv1.QueryUserResp{User: f.user}, nil
}

func (f *fakeUserService) QueryByUsernames(_ context.Context, _ *userv1.QueryByUsernamesReq, _ ...grpc.CallOption) (*userv1.QueryUsersResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	var users []*userv1.User
	if f.user != nil {
		users = append(users, f.user)
	}
	return &userv1.QueryUsersResp{Users: users}, nil
}

type fakeTicketService struct {
	ticketSvc.Service
	ticket  domain.Ticket
	tickets []domain.Ticket
	total   int64
	err     error
}

func (f *fakeTicketService) GetByProcessInstanceID(_ context.Context, _ int) (domain.Ticket, error) {
	return f.ticket, f.err
}

func (f *fakeTicketService) ListByUser(_ context.Context, _ string, _, _ int64) ([]domain.Ticket, int64, error) {
	return f.tickets, f.total, f.err
}

type fakeEngineService struct {
	engineSvc.Service
	taskInfo model.Task
	tasks    []domain.Instance
	err      error
}

func (f *fakeEngineService) TaskInfo(_ context.Context, _ int) (model.Task, error) {
	return f.taskInfo, f.err
}

func (f *fakeEngineService) Transfer(_ context.Context, _ int, _ []string) ([]model.Task, error) {
	return nil, f.err
}

func (f *fakeEngineService) ListPendingStepsOfMyTask(_ context.Context, _ []int, _ string) ([]domain.Instance, error) {
	return f.tasks, f.err
}

type fakeWithdrawalService struct {
	withdrawalSvc.Service
	revokeCalled bool
	err          error
}

func (f *fakeWithdrawalService) Revoke(_ context.Context, _ int, _ bool, _, _ string) error {
	f.revokeCalled = true
	return f.err
}

func newTestContext(userID int64) *ginx.Context {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	req, _ := http.NewRequest(http.MethodPost, "/", nil)
	if userID > 0 {
		req = req.WithContext(ctxutil.WithUserID(req.Context(), userID))
		c.Set(ctxutil.UserIDKey, userID)
	}
	c.Request = req
	return &ginx.Context{Context: c}
}

func TestHandler_Transfer_Security(t *testing.T) {
	testCases := []struct {
		name       string
		req        TransferReq
		userID     int64
		fakeUser   *userv1.User
		taskInfo   model.Task
		wantCode   int
		wantErrMsg string
	}{
		{
			name:       "非法任务ID",
			req:        TransferReq{TaskId: 0, Usernames: []string{"bob"}},
			userID:     1,
			fakeUser:   &userv1.User{Id: 1, Username: "alice"},
			wantCode:   400,
			wantErrMsg: "任务 ID 非法",
		},
		{
			name:       "转签目标人为空",
			req:        TransferReq{TaskId: 10, Usernames: []string{}},
			userID:     1,
			fakeUser:   &userv1.User{Id: 1, Username: "alice"},
			wantCode:   400,
			wantErrMsg: "转签目标用户不能为空",
		},
		{
			name:       "非当前指派人且非管理员被越权拦截",
			req:        TransferReq{TaskId: 10, Usernames: []string{"charlie"}},
			userID:     1,
			fakeUser:   &userv1.User{Id: 1, Username: "alice", IsAdmin: false},
			taskInfo:   model.Task{TaskID: 10, UserID: "bob"}, // 归属 bob
			wantCode:   500,
			wantErrMsg: "与您账号不一致",
		},
		{
			name:       "当前指派人允许转签",
			req:        TransferReq{TaskId: 10, Usernames: []string{"charlie"}},
			userID:     1,
			fakeUser:   &userv1.User{Id: 1, Username: "alice", IsAdmin: false},
			taskInfo:   model.Task{TaskID: 10, UserID: "alice"},
			wantCode:   200,
			wantErrMsg: "",
		},
		{
			name:       "管理员允许代办转签他人任务",
			req:        TransferReq{TaskId: 10, Usernames: []string{"charlie"}},
			userID:     99,
			fakeUser:   &userv1.User{Id: 99, Username: "admin", IsAdmin: true},
			taskInfo:   model.Task{TaskID: 10, UserID: "bob"},
			wantCode:   200,
			wantErrMsg: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{
				logger:    elog.DefaultLogger,
				userSvc:   &fakeUserService{user: tc.fakeUser},
				engineSvc: &fakeEngineService{taskInfo: tc.taskInfo},
			}
			ctx := newTestContext(tc.userID)
			res, err := h.Transfer(ctx, tc.req)
			if tc.wantErrMsg != "" {
				if err != nil {
					assert.Contains(t, err.Error(), tc.wantErrMsg)
				} else {
					assert.Contains(t, res.Msg, tc.wantErrMsg)
				}
			} else {
				require.NoError(t, err)
				assert.Equal(t, "转签成功", res.Msg)
			}
		})
	}
}

func TestHandler_Revoke_Security(t *testing.T) {
	testCases := []struct {
		name           string
		req            RevokeOrderReq
		userID         int64
		fakeUser       *userv1.User
		ticket         domain.Ticket
		wantCode       int
		wantRevokeCall bool
	}{
		{
			name:           "非发起人且非管理员尝试撤回被403拦截",
			req:            RevokeOrderReq{InstanceId: 200, Force: true, Reason: "测试撤回"},
			userID:         1,
			fakeUser:       &userv1.User{Id: 1, Username: "alice", IsAdmin: false},
			ticket:         domain.Ticket{CreateBy: "bob"}, // bob 发起的
			wantCode:       403,
			wantRevokeCall: false,
		},
		{
			name:           "工单发起人允许撤销自己的工单",
			req:            RevokeOrderReq{InstanceId: 200, Force: true, Reason: "自测撤销"},
			userID:         1,
			fakeUser:       &userv1.User{Id: 1, Username: "alice", IsAdmin: false},
			ticket:         domain.Ticket{CreateBy: "alice"},
			wantCode:       0,
			wantRevokeCall: true,
		},
		{
			name:           "管理员允许撤回任意工单",
			req:            RevokeOrderReq{InstanceId: 200, Force: true, Reason: "管理员强撤"},
			userID:         99,
			fakeUser:       &userv1.User{Id: 99, Username: "admin", IsAdmin: true},
			ticket:         domain.Ticket{CreateBy: "bob"},
			wantCode:       0,
			wantRevokeCall: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fakeWithdrawal := &fakeWithdrawalService{}
			h := &Handler{
				logger:        elog.DefaultLogger,
				userSvc:       &fakeUserService{user: tc.fakeUser},
				svc:           &fakeTicketService{ticket: tc.ticket},
				withdrawalSvc: fakeWithdrawal,
			}
			ctx := newTestContext(tc.userID)
			res, err := h.Revoke(ctx, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, res.Code)
			assert.Equal(t, tc.wantRevokeCall, fakeWithdrawal.revokeCalled)
		})
	}
}

func TestHandler_StartUser_CompleteData(t *testing.T) {
	// 验证 StartUser 不会丢失已撤销、流转中但无待办任务的工单
	tickets := []domain.Ticket{
		{
			Id: 1, CreateBy: "alice", Status: domain.PROCESS,
			Process: domain.Process{InstanceId: 101},
		},
		{
			Id: 2, CreateBy: "alice", Status: domain.WITHDRAWING,
			Process: domain.Process{InstanceId: 102},
		},
		{
			Id: 3, CreateBy: "alice", Status: domain.END,
			Process: domain.Process{InstanceId: 103},
		},
	}

	// 只有 ticket 1 有未完结的人工待办任务；ticket 2 和 3 没有人工待办
	pendingSteps := []domain.Instance{
		{ProcInstID: 101, CurrentNodeName: "领导审批", ApprovedBy: "leader1", TaskID: 501},
	}

	h := &Handler{
		logger:  elog.DefaultLogger,
		userSvc: &fakeUserService{user: &userv1.User{Id: 1, Username: "alice"}},
		svc: &fakeTicketService{
			tickets: tickets,
			total:   3,
		},
		engineSvc: &fakeEngineService{tasks: pendingSteps},
	}

	ctx := newTestContext(1)
	res, err := h.StartUser(ctx, StartUserReq{Offset: 0, Limit: 10})
	require.NoError(t, err)

	data, ok := res.Data.(RetrieveTickets)
	require.True(t, ok)
	require.Equal(t, int64(3), data.Total)
	require.Len(t, data.Tasks, 3)

	// 工单 1 正常映射节点名和处理人
	assert.Equal(t, "领导审批", data.Tasks[0].CurrentStep)
	assert.Equal(t, "leader1", data.Tasks[0].ApprovedBy)
	assert.Equal(t, 501, data.Tasks[0].TaskId)

	// 工单 2 撤回中，步骤展示为“撤回处理中”，而不是被丢弃
	assert.Equal(t, "撤回处理中", data.Tasks[1].CurrentStep)

	// 工单 3 已完结，步骤展示为“已结束”，而不是被丢弃
	assert.Equal(t, "已结束", data.Tasks[2].CurrentStep)
}
