package assignees

import (
	"context"
	"fmt"

	userv1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/Duke1616/eflow/internal/service/department"
)

// LeaderResolver 部门领导解析器
type LeaderResolver struct {
	userSvc       userv1.UserServiceClient
	departmentSvc department.Service
}

func NewLeaderResolver(userSvc userv1.UserServiceClient, departmentSvc department.Service) *LeaderResolver {
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

	resp, err := r.userSvc.FindByUsernames(ctx, &userv1.FindByUsernamesReq{
		Usernames: []string{target.Values[0]},
	})
	if err != nil {
		return nil, fmt.Errorf("查询发起人失败: %w", err)
	}

	if len(resp.Users) == 0 {
		return nil, fmt.Errorf("发起人 [%s] 不存在", target.Values[0])
	}

	startUser := resp.Users[0]
	if startUser.DepartmentId == 0 {
		return nil, fmt.Errorf("发起人 [%s] 未分配部门", target.Values[0])
	}

	depart, err := r.departmentSvc.FindById(ctx, startUser.DepartmentId)
	if err != nil {
		return nil, fmt.Errorf("查询部门 [%d] 失败: %w", startUser.DepartmentId, err)
	}

	if len(depart.Leaders) == 0 {
		return nil, nil
	}

	respLeaders, err := r.userSvc.FindByUsernames(ctx, &userv1.FindByUsernamesReq{
		Usernames: depart.Leaders,
	})
	if err != nil {
		return nil, fmt.Errorf("批量查询部门主管信息失败: %w", err)
	}

	return toDomainUsers(respLeaders.Users), nil
}
