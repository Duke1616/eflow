package assignees

import (
	"context"
	"fmt"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/Duke1616/eflow/internal/service/department"
)

// MainLeaderResolver 分管领导解析器
type MainLeaderResolver struct {
	userSvc       userv1.UserServiceClient
	departmentSvc department.Service
}

func NewMainLeaderResolver(userSvc userv1.UserServiceClient, departmentSvc department.Service) *MainLeaderResolver {
	return &MainLeaderResolver{userSvc: userSvc, departmentSvc: departmentSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *MainLeaderResolver) Name() string {
	return string(easyflow.MAIN_LEADER)
}

func (r *MainLeaderResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, fmt.Errorf("缺少发起人信息")
	}

	resp, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
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

	if depart.MainLeader == "" {
		return nil, nil
	}

	respMain, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: []string{depart.MainLeader},
	})
	if err != nil {
		return nil, fmt.Errorf("查询分管领导信息失败: %w", err)
	}

	if len(respMain.Users) > 0 && respMain.Users[0].Id != 0 {
		return toDomainUsers(respMain.Users), nil
	}
	return nil, nil
}
