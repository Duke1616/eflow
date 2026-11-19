package channel

import (
	"context"
	"errors"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"github.com/Duke1616/eflow/internal/domain"
)

var (
	// ErrChannelUserNotFound 当无法将三方平台用户标识解析为可信系统用户时返回
	ErrChannelUserNotFound = errors.New("无法定位第三方渠道标识对应的系统账号")
	// ErrUnsupportedAction 收到未支持的渠道交互动作时返回
	ErrUnsupportedAction = errors.New("不支持的渠道审批动作类型")
	// ErrTicketRevoked 当工单已被发起人撤回时返回
	ErrTicketRevoked = errors.New("工单已撤回")
	// ErrTicketRevoking 当工单处于撤单处理中时返回
	ErrTicketRevoking = errors.New("工单撤回处理中")
	// ErrTicketEnded 当工单已办结不可继续流转时返回
	ErrTicketEnded = errors.New("工单已办结")
)

// IChannelIdentityResolver 统一多渠道第三方身份与内部系统用户映射解析接口
type IChannelIdentityResolver interface {
	// ResolveUser 基于渠道类型、第三方平台用户标识及任务上下文精准定位可信的内部用户
	ResolveUser(ctx context.Context, channel domain.ChannelType, channelUserID string, engineType domain.EngineType, taskID domain.ID) (*userv1.User, error)
	// ResolveStarter 校验第三方渠道用户是否为工单的合法发起人（用于撤销卡片等工单级操作）
	ResolveStarter(ctx context.Context, channel domain.ChannelType, channelUserID string, starter string) (*userv1.User, error)
}

// IChannelActionService 跨渠道工单审批流转统一业务编排接口
type IChannelActionService interface {
	// DispatchAction 统筹调度并执行来自外部渠道（飞书、企微、钉钉等）的审批流转行为
	DispatchAction(ctx context.Context, evt domain.ChannelActionEvent) (*domain.ChannelActionResult, error)
}
