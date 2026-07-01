package assignees

import (
	"context"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/samber/lo"
)

// AppointResolver 指定内部人员解析器
type AppointResolver struct {
	userSvc userv1.UserServiceClient
}

func NewAppointResolver(userSvc userv1.UserServiceClient) *AppointResolver {
	return &AppointResolver{userSvc: userSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *AppointResolver) Name() string {
	return string(easyflow.APPOINT)
}

func (r *AppointResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
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

func toDomainUsers(src []*userv1.User) []domain.User {
	return lo.FilterMap(src, func(u *userv1.User, _ int) (domain.User, bool) {
		if u == nil {
			return domain.User{}, false
		}
		return domain.User{
			Id:           u.Id,
			DepartmentId: u.DepartmentId,
			Username:     u.Username,
			DisplayName:  u.DisplayName,
			Email:        u.Email,
			Phone:        u.Phone,
			LarkUserId:   u.LarkUserId,
			WechatUserId: u.WechatUserId,
		}, true
	})
}
