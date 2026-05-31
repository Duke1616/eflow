package ticket

import (
	"context"
	"fmt"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
)

// UserService 定义统一的业务用户查询接口，整合本系统与三方推送身份映射
type UserService interface {
	FindByWechatUser(ctx context.Context, wechatUserId string) (*userv1.User, error)
	FindByFeishuUserId(ctx context.Context, feishuUserId string) (*userv1.User, error)
	FindByUsername(ctx context.Context, username string) (*userv1.User, error)
	FindByUsernames(ctx context.Context, usernames []string) ([]*userv1.User, error)
}

type userServiceAdapter struct {
	client userv1.UserServiceClient
}

// NewUserServiceAdapter 实例化用户服务接口适配器
func NewUserServiceAdapter(client userv1.UserServiceClient) UserService {
	return &userServiceAdapter{client: client}
}

// FindByUsername 通过用户名查用户信息 (由 gRPC FindByUsernames 支撑实现)
func (a *userServiceAdapter) FindByUsername(ctx context.Context, username string) (*userv1.User, error) {
	resp, err := a.client.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: []string{username},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Users) == 0 {
		return nil, fmt.Errorf("user %s not found", username)
	}
	return resp.Users[0], nil
}

// FindByUsernames 批量通过用户名查询用户信息 (由 gRPC FindByUsernames 支撑实现)
func (a *userServiceAdapter) FindByUsernames(ctx context.Context, usernames []string) ([]*userv1.User, error) {
	resp, err := a.client.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: usernames,
	})
	if err != nil {
		return nil, err
	}
	return resp.Users, nil
}

// FindByWechatUser 通过企业微信 ID 查询本地用户
// HACK: 目前 gRPC pb 层尚未提供三方 ID 到本地用户反向查找的专属接口，为避免破裂，在此处利用企业微信 ID 降级作为用户名进行回退尝试
func (a *userServiceAdapter) FindByWechatUser(ctx context.Context, wechatUserId string) (*userv1.User, error) {
	// NOTE: 未来在 ecmdb pb 扩充 `FindByWechatUser` 接口后，应当在此直连 gRPC 对应实现
	user, err := a.FindByUsername(ctx, wechatUserId)
	if err != nil {
		// 返回降级构造的占位 User，防止三方回调链路由于找不到账号映射而彻底崩溃断流
		return &userv1.User{
			Username:     wechatUserId,
			DisplayName:  wechatUserId,
			WechatUserId: wechatUserId,
		}, nil
	}
	return user, nil
}

// FindByFeishuUserId 通过飞书用户 ID 查询本地用户
// HACK: 目前 gRPC pb 层尚未提供三方 ID 到本地用户反向查找的专属接口，为避免破裂，在此处将飞书 ID 降级作为用户名进行回退尝试
func (a *userServiceAdapter) FindByFeishuUserId(ctx context.Context, feishuUserId string) (*userv1.User, error) {
	// NOTE: 未来在 ecmdb pb 扩充 `FindByFeishuUserId` 接口后，应当在此直连 gRPC 对应实现
	user, err := a.FindByUsername(ctx, feishuUserId)
	if err != nil {
		// 返回降级构造的占位 User，防止三方回调链路由于找不到账号映射而彻底崩溃断流
		return &userv1.User{
			Username:   feishuUserId,
			DisplayName: feishuUserId,
			LarkUserId: feishuUserId,
		}, nil
	}
	return user, nil
}
