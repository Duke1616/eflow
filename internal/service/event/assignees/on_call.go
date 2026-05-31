package assignees

import (
	"context"
	"fmt"
	"strconv"

	rotav1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/rota/v1"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/ecodeclub/ekit/slice"
)

// OnCallResolver 值班解析器
type OnCallResolver struct {
	rotaSvc rotav1.OnCallServiceClient
	userSvc userv1.UserServiceClient
}

func NewOnCallResolver(rotaSvc rotav1.OnCallServiceClient, userSvc userv1.UserServiceClient) *OnCallResolver {
	return &OnCallResolver{rotaSvc: rotaSvc, userSvc: userSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *OnCallResolver) Name() string {
	return string(easyflow.ON_CALL)
}

func (r *OnCallResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, nil
	}

	// 将 string ID 列表转为 int64
	ids, err := parseIDs(target.Values)
	if err != nil {
		return nil, fmt.Errorf("解析排班组 ID 失败: %w", err)
	}

	// 批量查询当前时段的值班排班
	resp, err := r.rotaSvc.GetCurrentSchedulesByIDs(ctx, &rotav1.GetCurrentSchedulesByIDsRequest{
		Ids: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("查询值班排班失败: %w", err)
	}

	// 合并所有排班组的值班人员并去重
	var members []string
	for _, sc := range resp.Schedules {
		if sc.RotaGroup != nil {
			members = slice.UnionSet(members, sc.RotaGroup.Members)
		}
	}

	if len(members) == 0 {
		return nil, nil
	}

	respUsers, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: members,
	})
	if err != nil {
		return nil, fmt.Errorf("查询值班人员信息失败: %w", err)
	}

	return toDomainUsers(respUsers.Users), nil
}

// parseIDs 将 []string 转换为 []int64
func parseIDs(values []string) ([]int64, error) {
	ids := make([]int64, 0, len(values))
	for _, v := range values {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("无效的 ID [%s]: %w", v, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}
