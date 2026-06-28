package assignees

import (
	"context"
	"fmt"

	departmentv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/department/v1"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/samber/lo"
)

func queryFounderDepartment(ctx context.Context, userSvc userv1.UserServiceClient,
	departmentSvc departmentv1.DepartmentServiceClient, username string) (*departmentv1.Department, error) {
	resp, err := userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: []string{username},
	})
	if err != nil {
		return nil, fmt.Errorf("查询发起人失败: %w", err)
	}

	if len(resp.GetUsers()) == 0 {
		return nil, fmt.Errorf("发起人 [%s] 不存在", username)
	}

	founder := resp.GetUsers()[0]
	if founder.GetId() == 0 {
		return nil, fmt.Errorf("发起人 [%s] 用户 ID 为空", username)
	}

	departResp, err := departmentSvc.QueryByUserId(ctx, &departmentv1.QueryByUserIdReq{
		UserId: founder.GetId(),
	})
	if err != nil {
		return nil, fmt.Errorf("查询发起人 [%s] 所属部门失败: %w", username, err)
	}

	if departResp.GetErrorCode() != departmentv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
		return nil, fmt.Errorf("查询发起人 [%s] 所属部门失败: %s", username, departResp.GetErrorMessage())
	}

	depart, ok := lo.Find(departResp.GetDepartments(), func(depart *departmentv1.Department) bool {
		return depart != nil
	})
	if ok {
		return depart, nil
	}
	return nil, fmt.Errorf("发起人 [%s] 未分配部门", username)
}
