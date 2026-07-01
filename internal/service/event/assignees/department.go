package assignees

import (
	"context"
	"fmt"
	"strconv"

	departmentv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/department/v1"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/samber/lo"
)

const departmentMemberPageSize int64 = 200

// DepartmentResolver 指定部门成员解析器
type DepartmentResolver struct {
	departmentSvc departmentv1.DepartmentServiceClient
}

func NewDepartmentResolver(departmentSvc departmentv1.DepartmentServiceClient) *DepartmentResolver {
	return &DepartmentResolver{departmentSvc: departmentSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *DepartmentResolver) Name() string {
	return string(easyflow.DEPARTMENT)
}

func (r *DepartmentResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, nil
	}

	ids, err := lo.MapErr(target.Values, func(v string, _ int) (int64, error) {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的部门 ID [%s]: %w", v, err)
		}
		return id, nil
	})
	if err != nil {
		return nil, err
	}

	return lo.FlatMapErr(ids, func(id int64, _ int) ([]domain.User, error) {
		return r.listMembers(ctx, id)
	})
}

func (r *DepartmentResolver) listMembers(ctx context.Context, departmentID int64) ([]domain.User, error) {
	var users []domain.User
	var offset int64

	for {
		resp, err := r.departmentSvc.ListMembers(ctx, &departmentv1.ListMembersReq{
			DepartmentId: departmentID,
			Offset:       offset,
			Limit:        departmentMemberPageSize,
		})
		if err != nil {
			return nil, fmt.Errorf("查询部门 [%d] 成员失败: %w", departmentID, err)
		}
		if resp.GetErrorCode() != departmentv1.ErrorCode_ERROR_CODE_UNSPECIFIED {
			return nil, fmt.Errorf("查询部门 [%d] 成员失败: %s", departmentID, departmentErrorMessage(resp.GetErrorCode(), resp.GetErrorMessage()))
		}

		members := resp.GetMembers()
		users = append(users, toDomainUsers(members)...)

		offset += int64(len(members))
		if len(members) == 0 || int64(len(members)) < departmentMemberPageSize {
			break
		}
		if total := resp.GetTotal(); total > 0 && offset >= total {
			break
		}
	}

	return users, nil
}

func departmentErrorMessage(code departmentv1.ErrorCode, msg string) string {
	if msg != "" {
		return msg
	}
	return code.String()
}

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
