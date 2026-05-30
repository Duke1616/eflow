package assignees

import (
	"context"

	userv1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
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

	resp, err := r.userSvc.FindByUsernames(ctx, &userv1.FindByUsernamesReq{
		Usernames: target.Values,
	})
	if err != nil {
		return nil, err
	}

	return toDomainUsers(resp.Users), nil
}

func toDomainUsers(src []*userv1.User) []domain.User {
	res := make([]domain.User, 0, len(src))
	for _, u := range src {
		if u == nil {
			continue
		}
		res = append(res, domain.User{
			Id:           u.Id,
			Username:     u.Username,
			DisplayName:  u.DisplayName,
			Email:        u.Email,
			Phone:        u.Phone,
			LarkUserId:   u.LarkUserId,
			WechatUserId: u.WechatUserId,
		})
	}
	return res
}
