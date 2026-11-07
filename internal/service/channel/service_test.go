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
				mockTicket.EXPECT().Pass(gomock.Any(), domain.EngineTypeFlowx, "5242", "zhangsan", false, "准予发布", gomock.Any()).Return(nil)
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
				mockTicket.EXPECT().Pass(gomock.Any(), domain.EngineType(""), "5242", "zhangsan", false, "无", gomock.Any()).
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
				mockTicket.EXPECT().Reject(gomock.Any(), domain.EngineType(""), "5243", "zhangsan", false, "不符合发版要求").Return(nil)
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
				mockTicket.EXPECT().Pass(gomock.Any(), domain.EngineTypeFlowx, "flowx_node_alice_1", "zhangsan", false, "FlowX准予发布", gomock.Any()).Return(nil)
			},
			wantSuccess: true,
			wantRemark:  "你已同意该申请, 批注：FlowX准予发布",
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
