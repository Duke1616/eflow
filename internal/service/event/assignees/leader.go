package assignees

import (
	"context"
	"fmt"

	departmentv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/department/v1"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
)

// LeaderResolver 部门领导解析器
type LeaderResolver struct {
	userSvc       userv1.UserServiceClient
	departmentSvc departmentv1.DepartmentServiceClient
}

func NewLeaderResolver(userSvc userv1.UserServiceClient, departmentSvc departmentv1.DepartmentServiceClient) *LeaderResolver {
	return &LeaderResolver{userSvc: userSvc, departmentSvc: departmentSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *LeaderResolver) Name() string {
	return string(easyflow.LEADER)
}

func (r *LeaderResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, fmt.Errorf("缺少发起人信息")
	}

	depart, err := queryFounderDepartment(ctx, r.userSvc, r.departmentSvc, target.Values[0])
	if err != nil {
		return nil, err
	}

	if len(depart.GetLeaders()) == 0 {
		return nil, nil
	}

	respLeaders, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: depart.GetLeaders(),
	})
	if err != nil {
		return nil, fmt.Errorf("批量查询部门主管信息失败: %w", err)
	}

	return toDomainUsers(respLeaders.Users), nil
}
