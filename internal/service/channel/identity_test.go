package channel

import (
	"context"
	"testing"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	usermocks "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1/mocks"
	"github.com/Duke1616/eflow/internal/domain"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestChannelIdentityResolver_ResolveUser(t *testing.T) {
	testCases := []struct {
		name          string
		channel       domain.ChannelType
		channelUserID string
		taskID        domain.ID
		mockSetup     func(userMock *usermocks.MockUserServiceClient, engineMock *enginemocks.MockIEngine)
		wantErr       error
		wantUsername  string
	}{
		{
			name:          "通过待办任务上下文成功匹配飞书身份",
			channel:       domain.ChannelFeishu,
			channelUserID: "bcegag66",
			taskID:        domain.NewID(5242),
			mockSetup: func(userMock *usermocks.MockUserServiceClient, engineMock *enginemocks.MockIEngine) {
				engineMock.EXPECT().GetTaskContext(gomock.Any(), domain.NewID(5242)).Return(domain.TaskContext{
					Assignee: "luankz",
				}, nil)
				userMock.EXPECT().QueryByUsername(gomock.Any(), &userv1.QueryByUsernameReq{
					Username: "luankz",
				}).Return(&userv1.QueryUserResp{
					User: &userv1.User{
						Id:          1,
						Username:    "luankz",
						DisplayName: "栾凯朝",
						LarkUserId:  "bcegag66",
						IsAdmin:     true,
					},
				}, nil)
			},
			wantErr:      nil,
			wantUsername: "luankz",
		},
		{
			name:          "通过待办任务上下文成功匹配企微身份",
			channel:       domain.ChannelWechat,
			channelUserID: "LuanKaiZhao",
			taskID:        domain.NewID(5243),
			mockSetup: func(userMock *usermocks.MockUserServiceClient, engineMock *enginemocks.MockIEngine) {
				engineMock.EXPECT().GetTaskContext(gomock.Any(), domain.NewID(5243)).Return(domain.TaskContext{
					Assignee: "luankz",
				}, nil)
				userMock.EXPECT().QueryByUsername(gomock.Any(), &userv1.QueryByUsernameReq{
					Username: "luankz",
				}).Return(&userv1.QueryUserResp{
					User: &userv1.User{
						Id:           1,
						Username:     "luankz",
						DisplayName:  "栾凯朝",
						WechatUserId: "LuanKaiZhao",
					},
				}, nil)
			},
			wantErr:      nil,
			wantUsername: "luankz",
		},
		{
			name:          "通过FlowX字符串任务ID成功匹配飞书身份",
			channel:       domain.ChannelFeishu,
			channelUserID: "bcegag66",
			taskID:        domain.NewID("flowx_node_alice_1"),
			mockSetup: func(userMock *usermocks.MockUserServiceClient, engineMock *enginemocks.MockIEngine) {
				engineMock.EXPECT().GetTaskContext(gomock.Any(), domain.NewID("flowx_node_alice_1")).Return(domain.TaskContext{
					Assignee: "luankz",
				}, nil)
				userMock.EXPECT().QueryByUsername(gomock.Any(), &userv1.QueryByUsernameReq{
					Username: "luankz",
				}).Return(&userv1.QueryUserResp{
					User: &userv1.User{
						Id:          1,
						Username:    "luankz",
						DisplayName: "栾凯朝",
						LarkUserId:  "bcegag66",
						IsAdmin:     true,
					},
				}, nil)
			},
			wantErr:      nil,
			wantUsername: "luankz",
		},
		{
			name:          "任务指派人与操作者三方标识不匹配且无同名账号",
			channel:       domain.ChannelFeishu,
			channelUserID: "stranger_feishu_id",
			taskID:        domain.NewID(5244),
			mockSetup: func(userMock *usermocks.MockUserServiceClient, engineMock *enginemocks.MockIEngine) {
				engineMock.EXPECT().GetTaskContext(gomock.Any(), domain.NewID(5244)).Return(domain.TaskContext{
					Assignee: "luankz",
				}, nil)
				userMock.EXPECT().QueryByUsername(gomock.Any(), &userv1.QueryByUsernameReq{
					Username: "luankz",
				}).Return(&userv1.QueryUserResp{
					User: &userv1.User{
						Username:   "luankz",
						LarkUserId: "bcegag66",
					},
				}, nil)
				userMock.EXPECT().QueryByUsername(gomock.Any(), &userv1.QueryByUsernameReq{
					Username: "stranger_feishu_id",
				}).Return(nil, assert.AnError)
			},
			wantErr: ErrChannelUserNotFound,
		},
		{
			name:          "渠道用户标识为空直接拦截",
			channel:       domain.ChannelFeishu,
			channelUserID: "   ",
			taskID:        domain.NewID(5245),
			mockSetup: func(userMock *usermocks.MockUserServiceClient, engineMock *enginemocks.MockIEngine) {
			},
			wantErr: ErrChannelUserNotFound,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			userMock := usermocks.NewMockUserServiceClient(ctrl)
			engineMock := enginemocks.NewMockIEngine(ctrl)
			tc.mockSetup(userMock, engineMock)

			resolver := NewChannelIdentityResolver(userMock, engineMock)
			user, err := resolver.ResolveUser(context.Background(), tc.channel, tc.channelUserID, tc.taskID)

			if tc.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
				require.NotNil(t, user)
				assert.Equal(t, tc.wantUsername, user.Username)
			}
		})
	}
}
