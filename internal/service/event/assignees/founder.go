package assignees

import (
	"context"
	"fmt"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
)

// FounderResolver 发起人解析器
type FounderResolver struct {
	userSvc userv1.UserServiceClient
}

func NewFounderResolver(userSvc userv1.UserServiceClient) *FounderResolver {
	return &FounderResolver{userSvc: userSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *FounderResolver) Name() string {
	return string(easyflow.FOUNDER)
}

func (r *FounderResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, fmt.Errorf("缺少发起人信息")
	}

	resp, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: []string{target.Values[0]},
	})
	if err != nil {
		return nil, fmt.Errorf("查询发起人失败: %w", err)
	}

	if len(resp.Users) > 0 && resp.Users[0].Id != 0 {
		return toDomainUsers(resp.Users), nil
	}
	return nil, nil
}
