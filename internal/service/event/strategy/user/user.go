package user

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/gotomicro/ego/core/elog"
)

// Notification 审批人办理任务通知策略
type Notification struct {
	strategy.Service
}

func NewNotification(base strategy.Service) *Notification {
	return &Notification{
		Service: base,
	}
}

func (n *Notification) Send(ctx context.Context, info strategy.Info) (strategy.NotificationResponse, error) {
	// 1. 获取当前节点信息
	nodes, rawProps, err := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, fmt.Errorf("节点属性未配置: %v", err)
	}

	property, err := easyflow.ToNodeProperty[easyflow.UserProperty](easyflow.Node{Properties: rawProps})
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, fmt.Errorf("节点属性未配置: %v", err)
	}

	// 2. 解析审批人
	users, err := n.ResolveAssignees(ctx, &info, property.NormalizeAssignees())
	if err != nil {
		n.Logger().Error("解析审批人规则失败", elog.FieldErr(err), elog.String("node", info.CurrentNode.NodeID))
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	// 3. 构建通知元数据
	data, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	// 4. 异步处理消息发送逻辑 (以日志降级进行推送模拟)
	n.SafeGo(ctx, 3*time.Minute, func(sendCtx context.Context) {
		n.asyncSendNotification(sendCtx, info, property, data, strategy.NewRecipientMap(users, info.Channel))
	})

	return strategy.NotificationResponse{Msg: "success"}, nil
}

func (n *Notification) asyncSendNotification(ctx context.Context, info strategy.Info,
	property easyflow.UserProperty, data *strategy.NotificationData, userMap strategy.RecipientMap) {
	// 1. 尝试拉取任务，支持重试（引擎异步落库需时间）
	tasks, err := n.FetchTasksWithRetry(ctx, info)
	if err != nil {
		n.Logger().Warn("Notification 任务拉取失败，跳过通知发送", elog.FieldErr(err), elog.Int("instanceId", info.InstID))
		return
	}

	// 2. 准备通知元数据
	title := rule.GenerateTitle(data.StartUser.DisplayName, data.TName)

	// 3. 全局通知校验
	if ok := n.IsGlobalNotify(info.Workflow); !ok {
		return
	}

	// 4. 特殊处理：告警转工单通知
	if info.Order.Provide.IsAlert() {
		n.Logger().Info("[NOTIFICATION alert] 模拟发送告警工单通知给审批人",
			elog.Int("instanceId", info.InstID),
			elog.String("title", title),
			elog.Any("receivers", userMap.GetIDs()),
			elog.Any("channel", info.Channel))
		return
	}

	// 5. 模拟发送标准通知卡片
	for _, t := range tasks {
		n.Logger().Info("[NOTIFICATION user] 模拟发送办理通知给审批人",
			elog.Int("instanceId", info.InstID),
			elog.Int("taskId", t.TaskID),
			elog.String("title", title),
			elog.String("receiver", t.UserID),
			elog.Any("channel", info.Channel))
	}
}
