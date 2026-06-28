package assignees

import (
	"context"
	"fmt"
	"strconv"

	teamv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/team"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/samber/lo"
)

// TeamResolver 团队解析器
type TeamResolver struct {
	teamSvc teamv1.TeamServiceClient
	userSvc userv1.UserServiceClient
}

func NewTeamResolver(teamSvc teamv1.TeamServiceClient, userSvc userv1.UserServiceClient) *TeamResolver {
	return &TeamResolver{teamSvc: teamSvc, userSvc: userSvc}
}

// Name 返回该解析器覆盖的规则唯一标识
func (r *TeamResolver) Name() string {
	return string(easyflow.TEAM)
}

func (r *TeamResolver) Resolve(ctx context.Context, target resolve.Target) ([]domain.User, error) {
	if len(target.Values) == 0 {
		return nil, nil
	}

	// 将 string ID 列表转为 int64
	ids, err := lo.MapErr(target.Values, func(v string, _ int) (int64, error) {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("无效的团队 ID [%s]: %w", v, err)
		}
		return id, nil
	})
	if err != nil {
		return nil, err
	}

	// 批量查询团队信息
	resp, err := r.teamSvc.GetTeamByIds(ctx, &teamv1.GetTeamByIdsRequest{
		Ids: ids,
	})
	if err != nil {
		return nil, fmt.Errorf("查询团队信息失败: %w", err)
	}

	// 合并所有团队的成员名单并去重
	usernames := lo.Uniq(lo.FlatMap(resp.GetTeams(), func(team *teamv1.Team, _ int) []string {
		return team.GetMembers()
	}))

	if len(usernames) == 0 {
		return nil, nil
	}

	// 根据用户名批量查询人员详情
	respUsers, err := r.userSvc.QueryByUsernames(ctx, &userv1.QueryByUsernamesReq{
		Usernames: usernames,
	})
	if err != nil {
		return nil, fmt.Errorf("查询团队成员详情失败: %w", err)
	}

	return toDomainUsers(respUsers.Users), nil
}
