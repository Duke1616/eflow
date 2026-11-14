package channel

import (
	"context"
	"errors"
	"testing"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	channelmocks "github.com/Duke1616/eflow/internal/service/channel/mocks"
	ticketmocks "github.com/Duke1616/eflow/internal/service/ticket/mocks"
	withdrawalmocks "github.com/Duke1616/eflow/internal/service/withdrawal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestChannelActionService_DispatchAction(t *testing.T) {
	testCases := []struct {
		name        string
		evt         domain.ChannelActionEvent
		mockSetup   func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService)
		wantSuccess bool
		wantRemark  string
	}{
		{
			name: "审批同意且流转成功",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "bcegag66",
				Action:        domain.ChannelActionPass,
				TaskID:        domain.NewID(5242),
				TicketID:      2735,
				Comment:       "准予发布",
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Process: domain.Process{EngineType: domain.EngineTypeFlowx},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "bcegag66", domain.EngineTypeFlowx, domain.NewID(5242)).
					Return(&userv1.User{Username: "zhangsan", IsAdmin: false}, nil)
				mockTicket.EXPECT().Pass(gomock.Any(), gomock.Any(), "5242", "zhangsan", false, "准予发布", gomock.Any()).Return(nil)
			},
			wantSuccess: true,
			wantRemark:  "你已同意该申请, 批注：准予发布",
		},
		{
			name: "身份无法解析直接强阻断并报错",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "unknown_user",
				Action:        domain.ChannelActionPass,
				TaskID:        domain.NewID(5242),
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "unknown_user", domain.EngineType(""), domain.NewID(5242)).
					Return(nil, ErrChannelUserNotFound)
				// 严禁调用底层任何 Pass 或 Reject
			},
			wantSuccess: false,
		},
		{
			name: "底层 Pass 执行失败返回阻断结果",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "bcegag66",
				Action:        domain.ChannelActionPass,
				TaskID:        domain.NewID(5242),
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "bcegag66", domain.EngineType(""), domain.NewID(5242)).
					Return(&userv1.User{Username: "zhangsan", IsAdmin: false}, nil)
				mockTicket.EXPECT().Pass(gomock.Any(), gomock.Any(), "5242", "zhangsan", false, "无", gomock.Any()).
					Return(errors.New("模拟网关无法计算条件"))
			},
			wantSuccess: false,
		},
		{
			name: "审批驳回成功",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelWechat,
				ChannelUserID: "ZhangSan",
				Action:        domain.ChannelActionReject,
				TaskID:        domain.NewID(5243),
				Comment:       "不符合发版要求",
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelWechat, "ZhangSan", domain.EngineType(""), domain.NewID(5243)).
					Return(&userv1.User{Username: "zhangsan", IsAdmin: false}, nil)
				mockTicket.EXPECT().Reject(gomock.Any(), gomock.Any(), "5243", "zhangsan", false, "不符合发版要求", "", false).Return(nil)
			},
			wantSuccess: true,
			wantRemark:  "你已驳回该申请, 批注：不符合发版要求",
		},
		{
			name: "FlowX字符串任务ID流转成功",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "bcegag66",
				Action:        domain.ChannelActionPass,
				TaskID:        domain.NewID("flowx_node_alice_1"),
				TicketID:      2735,
				Comment:       "FlowX准予发布",
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Process: domain.Process{EngineType: domain.EngineTypeFlowx},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "bcegag66", domain.EngineTypeFlowx, domain.NewID("flowx_node_alice_1")).
					Return(&userv1.User{Username: "zhangsan", IsAdmin: false}, nil)
				mockTicket.EXPECT().Pass(gomock.Any(), gomock.Any(), "flowx_node_alice_1", "zhangsan", false, "FlowX准予发布", gomock.Any()).Return(nil)
			},
			wantSuccess: true,
			wantRemark:  "你已同意该申请, 批注：FlowX准予发布",
		},
		{
			name: "查看进度即使任务上下文不存在也安全放行",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "observer_feishu_id",
				Action:        domain.ChannelActionProgress,
				TicketID:      2735,
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					CreateBy: "lisi",
					Process:  domain.Process{EngineType: domain.EngineTypeFlowx},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "observer_feishu_id", domain.EngineTypeFlowx, domain.NewID("")).
					Return(nil, ErrChannelUserNotFound)
				mockResolver.EXPECT().ResolveStarter(gomock.Any(), domain.ChannelFeishu, "observer_feishu_id", "lisi").
					Return(nil, errors.New("not starter"))
			},
			wantSuccess: true,
			wantRemark:  "查看工单流转进度",
		},
		{
			name: "发起人撤销卡片回退校验发起人成功",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "starter_feishu_id",
				Action:        domain.ChannelActionRevoke,
				TicketID:      2735,
				Comment:       "不再需要发布",
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Id:       2735,
					CreateBy: "lisi",
					Status:   domain.PROCESS,
					Process:  domain.Process{EngineType: domain.EngineTypeFlowx, InstanceID: domain.NewID("inst_123")},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "starter_feishu_id", domain.EngineTypeFlowx, domain.NewID("")).
					Return(nil, ErrChannelUserNotFound)
				mockResolver.EXPECT().ResolveStarter(gomock.Any(), domain.ChannelFeishu, "starter_feishu_id", "lisi").
					Return(&userv1.User{Username: "lisi"}, nil)
				mockWithdrawal.EXPECT().Revoke(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_123"), true, "lisi", "不再需要发布").Return(nil)
			},
			wantSuccess: true,
			wantRemark:  "你已撤销该申请, 原因：不再需要发布",
		},
		{
			name: "非发起人非管理员撤销他人工单被归属闸门拦截",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "mallory_feishu_id",
				Action:        domain.ChannelActionRevoke,
				TicketID:      2735,
				Comment:       "尝试撤他人的单",
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Id:       2735,
					CreateBy: "bob",
					Status:   domain.PROCESS,
					Process:  domain.Process{EngineType: domain.EngineTypeFlowx, InstanceID: domain.NewID("inst_123")},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "mallory_feishu_id", domain.EngineTypeFlowx, domain.NewID("")).
					Return(&userv1.User{Username: "mallory", IsAdmin: false}, nil)
				// 不声明 Revoke 期望：闸门必须在触达撤回服务之前拦截
			},
			wantSuccess: false,
			wantRemark:  "无权撤回他人发起的工单",
		},
		{
			name: "管理员可撤销任意发起人的运行中工单",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "admin_feishu_id",
				Action:        domain.ChannelActionRevoke,
				TicketID:      2735,
				Comment:       "管理员强撤",
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Id:       2735,
					CreateBy: "bob",
					Status:   domain.PROCESS,
					Process:  domain.Process{EngineType: domain.EngineTypeFlowx, InstanceID: domain.NewID("inst_123")},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "admin_feishu_id", domain.EngineTypeFlowx, domain.NewID("")).
					Return(&userv1.User{Username: "admin", IsAdmin: true}, nil)
				mockWithdrawal.EXPECT().Revoke(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("inst_123"), true, "admin", "管理员强撤").Return(nil)
			},
			wantSuccess: true,
			wantRemark:  "你已撤销该申请, 原因：管理员强撤",
		},
		{
			name: "撤回处理中的工单阻止重复撤回",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "starter_feishu_id",
				Action:        domain.ChannelActionRevoke,
				TicketID:      2735,
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Id:       2735,
					CreateBy: "alice",
					Status:   domain.WITHDRAWING,
					Process:  domain.Process{EngineType: domain.EngineTypeFlowx, InstanceID: domain.NewID("inst_123")},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "starter_feishu_id", domain.EngineTypeFlowx, domain.NewID("")).
					Return(nil, ErrChannelUserNotFound)
				mockResolver.EXPECT().ResolveStarter(gomock.Any(), domain.ChannelFeishu, "starter_feishu_id", "alice").
					Return(&userv1.User{Username: "alice"}, nil)
			},
			wantSuccess: false,
			wantRemark:  "工单撤回处理中，不可重复发起撤回",
		},
		{
			name: "已办结工单不支持撤回",
			evt: domain.ChannelActionEvent{
				Channel:       domain.ChannelFeishu,
				ChannelUserID: "starter_feishu_id",
				Action:        domain.ChannelActionRevoke,
				TicketID:      2735,
			},
			mockSetup: func(mockResolver *channelmocks.MockIChannelIdentityResolver, mockTicket *ticketmocks.MockService, mockWithdrawal *withdrawalmocks.MockService) {
				mockTicket.EXPECT().GetByID(gomock.Any(), int64(2735)).Return(domain.Ticket{
					Id:       2735,
					CreateBy: "alice",
					Status:   domain.END,
					Process:  domain.Process{EngineType: domain.EngineTypeFlowx, InstanceID: domain.NewID("inst_123")},
				}, nil).AnyTimes()
				mockResolver.EXPECT().ResolveUser(gomock.Any(), domain.ChannelFeishu, "starter_feishu_id", domain.EngineTypeFlowx, domain.NewID("")).
					Return(nil, ErrChannelUserNotFound)
				mockResolver.EXPECT().ResolveStarter(gomock.Any(), domain.ChannelFeishu, "starter_feishu_id", "alice").
					Return(&userv1.User{Username: "alice"}, nil)
			},
			wantSuccess: false,
			wantRemark:  "当前工单状态不支持撤回: 当前状态为 已结束",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockResolver := channelmocks.NewMockIChannelIdentityResolver(ctrl)
			mockTicket := ticketmocks.NewMockService(ctrl)
			mockWithdrawal := withdrawalmocks.NewMockService(ctrl)
			tc.mockSetup(mockResolver, mockTicket, mockWithdrawal)

			svc := NewChannelActionService(
				mockResolver,
				mockTicket,
				mockWithdrawal,
			)

			res, err := svc.DispatchAction(context.Background(), tc.evt)
			require.NoError(t, err)
			require.NotNil(t, res)
			assert.Equal(t, tc.wantSuccess, res.Success)
			if tc.wantRemark != "" {
				assert.Equal(t, tc.wantRemark, res.Remark)
			}
		})
	}
}
