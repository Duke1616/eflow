package ticket

import (
	"net/http"
	"net/http/httptest"
	"testing"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	usermocks "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1/mocks"
	"github.com/Duke1616/eflow/internal/domain"
	ticketmocks "github.com/Duke1616/eflow/internal/service/ticket/mocks"
	withdrawalmocks "github.com/Duke1616/eflow/internal/service/withdrawal/mocks"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/ecodeclub/ginx"
	"github.com/gin-gonic/gin"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

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
		mockSetup  func(ctrl *gomock.Controller, mockUser *usermocks.MockUserServiceClient, mockTicket *ticketmocks.MockService)
		wantCode   int
		wantErrMsg string
	}{
		{
			name:   "非法任务ID",
			req:    TransferReq{TaskId: 0, Usernames: []string{"bob"}},
			userID: 1,
			mockSetup: func(ctrl *gomock.Controller, mockUser *usermocks.MockUserServiceClient, mockTicket *ticketmocks.MockService) {
			},
			wantCode:   400,
			wantErrMsg: "任务 ID 非法",
		},
		{
			name:   "转签目标人为空",
			req:    TransferReq{TaskId: 10, Usernames: []string{}},
			userID: 1,
			mockSetup: func(ctrl *gomock.Controller, mockUser *usermocks.MockUserServiceClient, mockTicket *ticketmocks.MockService) {
			},
			wantCode:   400,
			wantErrMsg: "转签目标用户不能为空",
		},
		{
			name:   "非当前指派人且非管理员被越权拦截",
			req:    TransferReq{TaskId: 10, Usernames: []string{"charlie"}},
			userID: 1,
			mockSetup: func(ctrl *gomock.Controller, mockUser *usermocks.MockUserServiceClient, mockTicket *ticketmocks.MockService) {
				mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 1}).
					Return(&userv1.QueryUserResp{User: &userv1.User{Id: 1, Username: "alice", IsAdmin: false}}, nil)
				mockTicket.EXPECT().Transfer(gomock.Any(), 10, "alice", false, []string{"charlie"}).
					Return(domain.ErrTaskUnauthorized)
			},
			wantCode:   403,
			wantErrMsg: "与您账号不一致",
		},
		{
			name:   "当前指派人允许转签",
			req:    TransferReq{TaskId: 10, Usernames: []string{"charlie"}},
			userID: 1,
			mockSetup: func(ctrl *gomock.Controller, mockUser *usermocks.MockUserServiceClient, mockTicket *ticketmocks.MockService) {
				mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 1}).
					Return(&userv1.QueryUserResp{User: &userv1.User{Id: 1, Username: "alice", IsAdmin: false}}, nil)
				mockTicket.EXPECT().Transfer(gomock.Any(), 10, "alice", false, []string{"charlie"}).
					Return(nil)
			},
			wantCode:   200,
			wantErrMsg: "",
		},
		{
			name:   "管理员允许代办转签他人任务",
			req:    TransferReq{TaskId: 10, Usernames: []string{"charlie"}},
			userID: 99,
			mockSetup: func(ctrl *gomock.Controller, mockUser *usermocks.MockUserServiceClient, mockTicket *ticketmocks.MockService) {
				mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 99}).
					Return(&userv1.QueryUserResp{User: &userv1.User{Id: 99, Username: "admin", IsAdmin: true}}, nil)
				mockTicket.EXPECT().Transfer(gomock.Any(), 10, "admin", true, []string{"charlie"}).
					Return(nil)
			},
			wantCode:   200,
			wantErrMsg: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockUser := usermocks.NewMockUserServiceClient(ctrl)
			mockTicket := ticketmocks.NewMockService(ctrl)
			tc.mockSetup(ctrl, mockUser, mockTicket)

			h := &Handler{
				logger:  elog.DefaultLogger,
				userSvc: mockUser,
				svc:     mockTicket,
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
		name      string
		req       RevokeOrderReq
		userID    int64
		mockSetup func(ctrl *gomock.Controller,
			mockUser *usermocks.MockUserServiceClient,
			mockTicket *ticketmocks.MockService,
			mockWithdrawal *withdrawalmocks.MockService)
		wantCode int
	}{
		{
			name:   "非发起人且非管理员尝试撤回被403拦截",
			req:    RevokeOrderReq{InstanceId: 200, Force: true, Reason: "测试撤回"},
			userID: 1,
			mockSetup: func(ctrl *gomock.Controller,
				mockUser *usermocks.MockUserServiceClient,
				mockTicket *ticketmocks.MockService,
				mockWithdrawal *withdrawalmocks.MockService) {
				mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 1}).
					Return(&userv1.QueryUserResp{User: &userv1.User{Id: 1, Username: "alice", IsAdmin: false}}, nil)
				mockTicket.EXPECT().GetByProcessInstanceID(gomock.Any(), 200).
					Return(domain.Ticket{CreateBy: "bob", Status: domain.PROCESS}, nil)
			},
			wantCode: 403,
		},
		{
			name:   "工单发起人允许撤销自己的工单",
			req:    RevokeOrderReq{InstanceId: 200, Force: true, Reason: "自测撤销"},
			userID: 1,
			mockSetup: func(ctrl *gomock.Controller,
				mockUser *usermocks.MockUserServiceClient,
				mockTicket *ticketmocks.MockService,
				mockWithdrawal *withdrawalmocks.MockService) {
				mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 1}).
					Return(&userv1.QueryUserResp{User: &userv1.User{Id: 1, Username: "alice", IsAdmin: false}}, nil)
				mockTicket.EXPECT().GetByProcessInstanceID(gomock.Any(), 200).
					Return(domain.Ticket{CreateBy: "alice", Status: domain.PROCESS}, nil)
				mockWithdrawal.EXPECT().Revoke(gomock.Any(), 200, true, "alice", "自测撤销").
					Return(nil)
			},
			wantCode: 0,
		},
		{
			name:   "管理员允许撤回任意工单",
			req:    RevokeOrderReq{InstanceId: 200, Force: true, Reason: "管理员强撤"},
			userID: 99,
			mockSetup: func(ctrl *gomock.Controller,
				mockUser *usermocks.MockUserServiceClient,
				mockTicket *ticketmocks.MockService,
				mockWithdrawal *withdrawalmocks.MockService) {
				mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 99}).
					Return(&userv1.QueryUserResp{User: &userv1.User{Id: 99, Username: "admin", IsAdmin: true}}, nil)
				mockTicket.EXPECT().GetByProcessInstanceID(gomock.Any(), 200).
					Return(domain.Ticket{CreateBy: "bob", Status: domain.PROCESS}, nil)
				mockWithdrawal.EXPECT().Revoke(gomock.Any(), 200, true, "admin", "管理员强撤").
					Return(nil)
			},
			wantCode: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockUser := usermocks.NewMockUserServiceClient(ctrl)
			mockTicket := ticketmocks.NewMockService(ctrl)
			mockWithdrawal := withdrawalmocks.NewMockService(ctrl)
			tc.mockSetup(ctrl, mockUser, mockTicket, mockWithdrawal)

			h := &Handler{
				logger:        elog.DefaultLogger,
				userSvc:       mockUser,
				svc:           mockTicket,
				withdrawalSvc: mockWithdrawal,
			}
			ctx := newTestContext(tc.userID)
			res, err := h.Revoke(ctx, tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCode, res.Code)
		})
	}
}

func TestHandler_StartUser_CompleteData(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockUser := usermocks.NewMockUserServiceClient(ctrl)
	mockQuery := ticketmocks.NewMockQueryService(ctrl)

	views := []domain.TicketTaskView{
		{
			Ticket:      domain.Ticket{Id: 1, CreateBy: "alice", Status: domain.PROCESS, Ctime: 1716999999000},
			CurrentStep: "领导审批",
			ApprovedBy:  "李领导",
			TaskID:      501,
		},
		{
			Ticket:      domain.Ticket{Id: 2, CreateBy: "alice", Status: domain.WITHDRAWING, Ctime: 1716999999000},
			CurrentStep: "撤回处理中",
		},
		{
			Ticket:      domain.Ticket{Id: 3, CreateBy: "alice", Status: domain.END, Ctime: 1716999999000},
			CurrentStep: "已结束",
		},
	}

	mockUser.EXPECT().QueryById(gomock.Any(), &userv1.QueryByIdReq{Id: 1}).
		Return(&userv1.QueryUserResp{User: &userv1.User{Id: 1, Username: "alice"}}, nil)

	mockQuery.EXPECT().ListUserInitiated(gomock.Any(), "alice", int64(0), int64(10)).
		Return(views, int64(3), nil)

	h := &Handler{
		logger:   elog.DefaultLogger,
		userSvc:  mockUser,
		querySvc: mockQuery,
	}

	ctx := newTestContext(1)
	res, err := h.StartUser(ctx, StartUserReq{Offset: 0, Limit: 10})
	require.NoError(t, err)

	data, ok := res.Data.(RetrieveTickets)
	require.True(t, ok)
	require.Equal(t, int64(3), data.Total)
	require.Len(t, data.Tasks, 3)

	// 工单 1 正常映射节点名和处理人真实姓名，并正确填充流程提交时间
	assert.Equal(t, "领导审批", data.Tasks[0].CurrentStep)
	assert.Equal(t, "李领导", data.Tasks[0].ApprovedBy)
	assert.Equal(t, 501, data.Tasks[0].TaskId)
	assert.NotEmpty(t, data.Tasks[0].ProcInstCreateTime)
	assert.Equal(t, data.Tasks[0].Ctime, data.Tasks[0].ProcInstCreateTime)

	// 工单 2 撤回中，步骤展示为“撤回处理中”
	assert.Equal(t, "撤回处理中", data.Tasks[1].CurrentStep)

	// 工单 3 已完结，步骤展示为“已结束”
	assert.Equal(t, "已结束", data.Tasks[2].CurrentStep)
}
