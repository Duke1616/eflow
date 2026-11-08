package channel

import (
	"context"
	"fmt"
	"strings"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	"github.com/gotomicro/ego/core/elog"
)

type channelIdentityResolver struct {
	userClient userv1.UserServiceClient
	engine     engineSvc.IEngine
	logger     *elog.Component
}

// NewChannelIdentityResolver 构造跨渠道统一身份解析器
func NewChannelIdentityResolver(
	userClient userv1.UserServiceClient,
	engine engineSvc.IEngine,
) IChannelIdentityResolver {
	return &channelIdentityResolver{
		userClient: userClient,
		engine:     engine,
		logger:     elog.DefaultLogger.With(elog.FieldComponentName("ChannelIdentityResolver")),
	}
}

// ResolveUser 基于多级防御策略，将第三方渠道平台的用户标识解析为系统内部用户实体
func (r *channelIdentityResolver) ResolveUser(ctx context.Context, channel domain.ChannelType, channelUserID string, engineType domain.EngineType, taskID domain.ID) (*userv1.User, error) {
	trimmedID := strings.TrimSpace(channelUserID)
	if trimmedID == "" {
		return nil, fmt.Errorf("%w: 渠道用户标识不能为空", ErrChannelUserNotFound)
	}

	// 策略 1（核心高命中）：基于待审批任务上下文的指派人精准推导
	// 审批卡片交互必然具备 taskID，通过直接核对该节点 Assignee 的三方绑定标识，能够以极低开销实现 100% 鉴权闭环
	if !taskID.IsZero() && r.engine != nil {
		if matchedUser, err := r.resolveByTaskAssignee(ctx, channel, trimmedID, engineType, taskID); err == nil && matchedUser != nil {
			r.logger.Info("通过任务上下文指派人成功解析第三方身份",
				elog.String("channel", string(channel)),
				elog.String("channelUserID", trimmedID),
				elog.String("username", matchedUser.Username),
				elog.String("taskID", taskID.String()),
			)
			return matchedUser, nil
		}
	}

	// 策略 2：直接尝试以三方标识作为系统用户名匹配（兼容账号同名场景，如企微工号与用户名一致）
	if directUser, err := r.findUserByUsername(ctx, trimmedID); err == nil && directUser != nil {
		if r.isChannelBindingMatch(directUser, channel, trimmedID) {
			r.logger.Info("通过同名账号绑定关系成功解析第三方身份",
				elog.String("channel", string(channel)),
				elog.String("channelUserID", trimmedID),
				elog.String("username", directUser.Username),
			)
			return directUser, nil
		}
	}

	r.logger.Warn("无法解析第三方渠道用户身份",
		elog.String("channel", string(channel)),
		elog.String("channelUserID", trimmedID),
		elog.String("taskID", taskID.String()),
	)

	return nil, fmt.Errorf("%w: channel=%s, id=%s", ErrChannelUserNotFound, channel, trimmedID)
}

// resolveByTaskAssignee 通过任务上下文的 Assignee 提取用户信息并校验三方渠道绑定关系
func (r *channelIdentityResolver) resolveByTaskAssignee(ctx context.Context, channel domain.ChannelType, channelUserID string, engineType domain.EngineType, taskID domain.ID) (*userv1.User, error) {
	taskCtx, err := r.engine.GetTaskContext(ctx, engineType, taskID)
	if err != nil {
		return nil, fmt.Errorf("查询任务上下文失败: %w", err)
	}

	assignee := strings.TrimSpace(taskCtx.Assignee)
	if assignee == "" {
		return nil, fmt.Errorf("当前任务尚未指派明确处理人")
	}

	user, err := r.findUserByUsername(ctx, assignee)
	if err != nil {
		return nil, fmt.Errorf("查询指派人账户详情失败: %w", err)
	}

	if r.isChannelBindingMatch(user, channel, channelUserID) {
		return user, nil
	}

	return nil, fmt.Errorf("任务指派人绑定的第三方标识与当前操作者不一致")
}

// findUserByUsername 调用 EIAM gRPC 接口通过用户名检索用户实体
func (r *channelIdentityResolver) findUserByUsername(ctx context.Context, username string) (*userv1.User, error) {
	resp, err := r.userClient.QueryByUsername(ctx, &userv1.QueryByUsernameReq{
		Username: username,
	})
	if err != nil {
		return nil, err
	}
	if resp == nil || resp.User == nil {
		return nil, fmt.Errorf("user %s not found", username)
	}
	return resp.User, nil
}

// isChannelBindingMatch 校验内部用户实体中记录的渠道标识是否与回调传入的三方标识一致
func (r *channelIdentityResolver) isChannelBindingMatch(u *userv1.User, channel domain.ChannelType, channelUserID string) bool {
	if u == nil {
		return false
	}
	switch channel {
	case domain.ChannelFeishu:
		return u.LarkUserId != "" && u.LarkUserId == channelUserID
	case domain.ChannelWechat:
		return u.WechatUserId != "" && u.WechatUserId == channelUserID
	default:
		return false
	}
}
