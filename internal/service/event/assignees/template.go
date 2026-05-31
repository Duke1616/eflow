package assignees

import (
	"context"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
)

// TemplateResolver 模版字段解析器
type TemplateResolver struct {
	userSvc userv1.UserServiceClient
}

func NewTemplateResolver(userSvc userv1.UserServiceClient) *TemplateResolver {
	return &TemplateResolver{userSvc: userSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *TemplateResolver) Name() string {
	return string(easyflow.TEMPLATE)
}

func (r *TemplateResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, nil
	}

	resp, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: target.Values,
	})
	if err != nil {
		return nil, err
	}

	return toDomainUsers(resp.Users), nil
}
