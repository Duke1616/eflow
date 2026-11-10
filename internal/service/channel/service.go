package channel

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/gotomicro/ego/core/elog"
)

type channelActionService struct {
	identityResolver IChannelIdentityResolver
	ticketSvc        ticketSvc.Service
	withdrawalSvc    withdrawalSvc.Service
	logger           *elog.Component
}

// NewChannelActionService 构造跨渠道统一工单业务调度服务
func NewChannelActionService(
	identityResolver IChannelIdentityResolver,
	ticketSvc ticketSvc.Service,
	withdrawalSvc withdrawalSvc.Service,
) IChannelActionService {
	return &channelActionService{
		identityResolver: identityResolver,
		ticketSvc:        ticketSvc,
		withdrawalSvc:    withdrawalSvc,
		logger:           elog.DefaultLogger.With(elog.FieldComponentName("ChannelActionService")),
	}
}

// DispatchAction 统一接收跨渠道交互，执行身份解析、安全断言、状态机推进与结果决策
func (s *channelActionService) DispatchAction(ctx context.Context, evt domain.ChannelActionEvent) (*domain.ChannelActionResult, error) {
	// 1. 注入多租户上下文，防止被 GORM 租户安全插件拦截
	if evt.TenantID > 0 {
		ctx = ctxutil.WithTenantID(ctx, evt.TenantID)
	}

	comment := strings.TrimSpace(evt.Comment)
	if comment == "" {
		comment = "无"
	}

	var (
		engineType domain.EngineType
		createBy   string
	)
	if evt.TicketID > 0 {
		if ticket, err := s.ticketSvc.GetByID(ctx, evt.TicketID); err == nil {
			engineType = ticket.Process.EngineType
			createBy = ticket.CreateBy
		}
	}

	// 2. 特权动作与只读交互优化：
	// “查看进度”为只读查询，绝不修改工单或任务状态。优先解析操作人，解析不到也安全放行
	if evt.Action == domain.ChannelActionProgress {
		operator := evt.ChannelUserID
		if user, err := s.identityResolver.ResolveUser(ctx, evt.Channel, evt.ChannelUserID, engineType, evt.TaskID); err == nil && user != nil {
			operator = user.Username
		} else if createBy != "" {
			if startUser, sErr := s.identityResolver.ResolveStarter(ctx, evt.Channel, evt.ChannelUserID, createBy); sErr == nil && startUser != nil {
				operator = startUser.Username
			}
		}
		return &domain.ChannelActionResult{
			Success:  true,
			Action:   domain.ChannelActionProgress,
			Operator: operator,
			Remark:   "查看工单流转进度",
		}, nil
	}

	// 3. 身份解析（针对审批与撤销等写操作的核心鉴权防线）
	user, err := s.identityResolver.ResolveUser(ctx, evt.Channel, evt.ChannelUserID, engineType, evt.TaskID)
	// 若任务上下文未解析到（如发起人撤销卡片无具体审批任务），且当前动作是撤销，回退校验工单发起人身份
	if err != nil && evt.Action == domain.ChannelActionRevoke && createBy != "" {
		if startUser, sErr := s.identityResolver.ResolveStarter(ctx, evt.Channel, evt.ChannelUserID, createBy); sErr == nil && startUser != nil {
			user = startUser
			err = nil
		}
	}

	if err != nil {
		s.logger.Error("跨渠道审批身份解析失败，强行阻断流转",
			elog.String("channel", string(evt.Channel)),
			elog.String("channelUserID", evt.ChannelUserID),
			elog.String("taskID", evt.TaskID.String()),
			elog.FieldErr(err),
		)
		return &domain.ChannelActionResult{
			Success:  false,
			Action:   evt.Action,
			Operator: evt.ChannelUserID,
			Remark:   fmt.Sprintf("审批操作失败: 您的第三方账号尚未绑定内部系统账号 (渠道标识: %s)", evt.ChannelUserID),
			Error:    err,
		}, nil
	}

	operator := user.Username
	isAdmin := user.IsAdmin

	s.logger.Info("开始执行渠道审批业务分发",
		elog.String("channel", string(evt.Channel)),
		elog.String("action", string(evt.Action)),
		elog.String("operator", operator),
		elog.String("taskID", evt.TaskID.String()),
		elog.Int64("ticketID", evt.TicketID),
	)

	// 3. 按照动作分发业务执行
	switch evt.Action {
	case domain.ChannelActionPass:
		return s.handlePass(ctx, evt, engineType, operator, isAdmin, comment)
	case domain.ChannelActionReject:
		return s.handleReject(ctx, evt, engineType, operator, isAdmin, comment)
	case domain.ChannelActionRevoke:
		return s.handleRevoke(ctx, evt, operator, comment)
	default:
		s.logger.Warn("收到不支持的渠道动作指令", elog.String("action", string(evt.Action)))
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedAction, evt.Action)
	}
}

// handlePass 统一处理审批同意逻辑
func (s *channelActionService) handlePass(ctx context.Context, evt domain.ChannelActionEvent, _ domain.EngineType, operator string, isAdmin bool, comment string) (*domain.ChannelActionResult, error) {
	err := s.ticketSvc.Pass(ctx, evt.TicketID, evt.TaskID.String(), operator, isAdmin, comment, evt.FormValues)
	if err != nil {
		s.logger.Error("渠道审批[同意]执行失败",
			elog.String("taskID", evt.TaskID.String()),
			elog.String("operator", operator),
			elog.FieldErr(err),
		)

		return &domain.ChannelActionResult{
			Success:  false,
			Action:   domain.ChannelActionPass,
			Operator: operator,
			Remark:   formatTaskActionError(domain.ChannelActionPass, err),
			Error:    err,
		}, nil
	}

	return &domain.ChannelActionResult{
		Success:  true,
		Action:   domain.ChannelActionPass,
		Operator: operator,
		Remark:   fmt.Sprintf("你已同意该申请, 批注：%s", comment),
	}, nil
}

// handleReject 统一处理审批驳回逻辑
func (s *channelActionService) handleReject(ctx context.Context, evt domain.ChannelActionEvent, _ domain.EngineType, operator string, isAdmin bool, comment string) (*domain.ChannelActionResult, error) {
	err := s.ticketSvc.Reject(ctx, evt.TicketID, evt.TaskID.String(), operator, isAdmin, comment)
	if err != nil {
		s.logger.Error("渠道审批[驳回]执行失败",
			elog.String("taskID", evt.TaskID.String()),
			elog.String("operator", operator),
			elog.FieldErr(err),
		)

		return &domain.ChannelActionResult{
			Success:  false,
			Action:   domain.ChannelActionReject,
			Operator: operator,
			Remark:   formatTaskActionError(domain.ChannelActionReject, err),
			Error:    err,
		}, nil
	}

	return &domain.ChannelActionResult{
		Success:  true,
		Action:   domain.ChannelActionReject,
		Operator: operator,
		Remark:   fmt.Sprintf("你已驳回该申请, 批注：%s", comment),
	}, nil
}

// formatTaskActionError 将底层的任务执行错误格式化为面向用户的友好中文提示
// 解释设计原因：Pass 与 Reject 拥有对称的节点生命周期与权限校验，统一错误映射可避免多处维护导致文案不一致或漏测
func formatTaskActionError(action domain.ChannelActionType, err error) string {
	actionDesc := "审批"
	if action == domain.ChannelActionReject {
		actionDesc = "驳回"
	}

	switch {
	case errors.Is(err, ticketSvc.ErrTaskAlreadyFinished):
		return fmt.Sprintf("当前节点任务已经结束，无法进行%s", actionDesc)
	case errors.Is(err, domain.ErrTaskUnauthorized):
		return fmt.Sprintf("您无权%s此任务（当前任务指派人与操作者不符）", actionDesc)
	default:
		return fmt.Sprintf("%s操作失败: %v", actionDesc, err)
	}
}

// handleRevoke 统一处理工单撤单逻辑
func (s *channelActionService) handleRevoke(ctx context.Context, evt domain.ChannelActionEvent, operator string, reason string) (*domain.ChannelActionResult, error) {
	ticket, err := s.ticketSvc.GetByID(ctx, evt.TicketID)
	if err != nil {
		return &domain.ChannelActionResult{
			Success:  false,
			Action:   domain.ChannelActionRevoke,
			Operator: operator,
			Remark:   "撤回失败: 工单实体不存在",
			Error:    err,
		}, nil
	}

	err = s.withdrawalSvc.Revoke(ctx, ticket.Process.EngineType, ticket.Process.InstanceID, true, operator, reason)
	if err != nil {
		s.logger.Error("渠道操作[撤销]执行失败",
			elog.Int64("ticketID", evt.TicketID),
			elog.String("operator", operator),
			elog.FieldErr(err),
		)

		var remark string
		if errors.Is(err, withdrawalSvc.ErrAutomationRunning) {
			remark = "自动化任务正在执行，暂时无法撤回，请稍后重试"
		} else {
			remark = fmt.Sprintf("撤销工单失败: %v", err)
		}

		return &domain.ChannelActionResult{
			Success:  false,
			Action:   domain.ChannelActionRevoke,
			Operator: operator,
			Remark:   remark,
			Error:    err,
		}, nil
	}

	return &domain.ChannelActionResult{
		Success:  true,
		Action:   domain.ChannelActionRevoke,
		Operator: operator,
		Remark:   fmt.Sprintf("你已撤销该申请, 原因：%s", reason),
	}, nil
}
