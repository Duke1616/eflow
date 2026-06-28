package assignees

import (
	"context"
	"fmt"
	"strconv"

	oncallv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/oncall/v1"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/samber/lo"
)

// OnCallResolver 值班解析器
type OnCallResolver struct {
	onCallSvc oncallv1.OnCallServiceClient
	userSvc   userv1.UserServiceClient
}

func NewOnCallResolver(onCallSvc oncallv1.OnCallServiceClient, userSvc userv1.UserServiceClient) *OnCallResolver {
	return &OnCallResolver{onCallSvc: onCallSvc, userSvc: userSvc}
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
	resp, err := r.onCallSvc.GetCurrentSchedulesByIDs(ctx, &oncallv1.GetCurrentSchedulesByIDsRequest{
		Ids: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("查询值班排班失败: %w", err)
	}

	// 合并所有排班组的值班人员并去重
	members := lo.Uniq(lo.FlatMap(resp.GetSchedules(), func(sc *oncallv1.OnCallSchedule, _ int) []string {
		group := sc.GetOncallGroup()
		if group == nil {
			return nil
		}
		return group.GetMembers()
	}))

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
	return lo.MapErr(values, func(v string, _ int) (int64, error) {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的 ID [%s]: %w", v, err)
		}
		return id, nil
	})
}
