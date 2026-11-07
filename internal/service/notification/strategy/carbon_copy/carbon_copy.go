package carbon_copy

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/pkg/notification"
	"github.com/Duke1616/eflow/internal/pkg/notification/sender"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
)

type Notification struct {
	strategy.Service
	sender sender.NotificationSender
}

func NewNotification(base strategy.Service, sender sender.NotificationSender) *Notification {
	return &Notification{
		Service: base,
		sender:  sender,
	}
}

func (n *Notification) Send(ctx context.Context, info strategy.Info) (notification.Response, error) {
	// 1. 获取节点信息
	nodes, node, err := n.GetNode(info, info.CurrentNode.NodeID)
	if err != nil {
		return notification.NewErrorResponse(string(notification.ErrorCodeNodeNotConfigured), err.Error()), err
	}

	// 2. 加载基础数据
	data, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return notification.NewErrorResponse(string(notification.ErrorCodeFetchDataFailed), err.Error()), err
	}

	// 3. 解析抄送人员并自动同步到节点
	var assignees []flowdsl.Assignee
	if userCfg, isUser := node.AsUserTask(); isUser {
		assignees = userCfg.Assignees
	}
	users, err := n.ResolveAssignees(ctx, &info, assignees)
	if err != nil {
		return notification.NewErrorResponse(string(notification.ErrorCodeResolveRuleFailed), err.Error()), err
	}

	// 4. 异步处理
	n.SafeGo(ctx, 3*time.Minute, func(sendCtx context.Context) {
		n.asyncHandleCarbonCopy(sendCtx, info, data, strategy.NewRecipientMap(users, info.Channel))
	})

	return notification.NewSuccessResponse(0, "success"), nil
}

func (n *Notification) asyncHandleCarbonCopy(ctx context.Context, info strategy.Info, data *strategy.NotificationData, userMap strategy.RecipientMap) {
	// 1. 获取任务
	tasks, err := n.FetchTasksWithRetry(ctx, info)
	if err != nil {
		n.Logger().Error("CarbonCopy 获取任务失败", elog.FieldErr(err))
		return
	}

	// 2. 发送消息
	if n.IsGlobalNotify(info.Workflow) {
		title := rule.GenerateCCTitle(data.StartUser.DisplayName, data.TName)
		fields := n.PrepareCommonFields(info, data)

		ns := lo.Map(tasks, func(src strategy.Task, _ int) notification.Notification {
			receiver := userMap.GetID(src.UserID)
			return notification.Notification{
				Channel:    notification.Channel(info.Channel),
				WorkFlowID: info.Workflow.Id,
				Receiver:   receiver,
				Template: notification.Template{
					Name:     domain.NotifyTypeCC,
					Title:    title,
					Fields:   fields,
					Values:   notification.GenerateCallbackValues(info.Ticket.Id, src.TaskID.String(), info.Ticket.TenantID),
					HideForm: true,
				},
			}
		})

		if _, err = n.sender.BatchSend(ctx, ns); err != nil {
			n.Logger().Error("CarbonCopy 批量发送消息失败", elog.FieldErr(err))
		}
	}

	// 3. 自动通过任务流转
	for _, task := range tasks {
		if err = n.PassTask(ctx, task.TaskID, "抄送系统自动通过"); err != nil {
			n.Logger().Error("CarbonCopy 自动流转失败", elog.FieldErr(err), elog.String("taskId", task.TaskID.String()))
		}
	}
}
