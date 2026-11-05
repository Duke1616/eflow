package eiam

import (
	"context"
	"fmt"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
)

// IUserClient EIAM 用户查询与第三方账号映射客户端
type IUserClient interface {
	// FindByUsername 按用户名查询用户详情
	FindByUsername(ctx context.Context, username string) (*userv1.User, error)
	// FindByUsernames 批量查询用户，用于解析多审批人显示名称
	FindByUsernames(ctx context.Context, usernames []string) ([]*userv1.User, error)
	// FindByWechatUser 按企微 UserID 映射内部用户，未绑定时降级返回占位用户避免断流
	FindByWechatUser(ctx context.Context, wechatUserId string) (*userv1.User, error)
}

type userClientAdapter struct {
	client userv1.UserServiceClient
}

// NewUserClient 构造 EIAM 用户客户端
func NewUserClient(client userv1.UserServiceClient) IUserClient {
	return &userClientAdapter{client: client}
}

// FindByUsername 按用户名查询用户详情
func (a *userClientAdapter) FindByUsername(ctx context.Context, username string) (*userv1.User, error) {
	resp, err := a.client.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: []string{username},
	})
	if err != nil {
		return nil, fmt.Errorf("查询用户失败 [username: %s]: %w", username, err)
	}
	if len(resp.Users) == 0 {
		return nil, fmt.Errorf("用户不存在 [username: %s]", username)
	}
	return resp.Users[0], nil
}

// FindByUsernames 批量查询用户
func (a *userClientAdapter) FindByUsernames(ctx context.Context, usernames []string) ([]*userv1.User, error) {
	if len(usernames) == 0 {
		return []*userv1.User{}, nil
	}
	resp, err := a.client.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: usernames,
	})
	if err != nil {
		return nil, fmt.Errorf("批量查询用户失败: %w", err)
	}
	return resp.Users, nil
}

// FindByWechatUser 按企微 UserID 映射内部用户
func (a *userClientAdapter) FindByWechatUser(ctx context.Context, wechatUserId string) (*userv1.User, error) {
	user, err := a.FindByUsername(ctx, wechatUserId)
	if err != nil {
		return &userv1.User{
			Username:     wechatUserId,
			DisplayName:  wechatUserId,
			WechatUserId: wechatUserId,
		}, nil
	}
	return user, nil
}
