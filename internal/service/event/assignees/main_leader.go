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

// MainLeaderResolver 分管领导解析器
type MainLeaderResolver struct {
	userSvc       userv1.UserServiceClient
	departmentSvc departmentv1.DepartmentServiceClient
}

func NewMainLeaderResolver(userSvc userv1.UserServiceClient, departmentSvc departmentv1.DepartmentServiceClient) *MainLeaderResolver {
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

	depart, err := queryFounderDepartment(ctx, r.userSvc, r.departmentSvc, target.Values[0])
	if err != nil {
		return nil, err
	}

	if depart.GetMainLeader() == "" {
		return nil, nil
	}

	respMain, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: []string{depart.GetMainLeader()},
	})
	if err != nil {
		return nil, fmt.Errorf("查询分管领导信息失败: %w", err)
	}

	if len(respMain.Users) > 0 && respMain.Users[0].Id != 0 {
		return toDomainUsers(respMain.Users), nil
	}
	return nil, nil
}
