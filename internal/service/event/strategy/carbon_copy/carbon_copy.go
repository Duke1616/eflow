package carbon_copy

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/gotomicro/ego/core/elog"
)

// Notification 抄送节点通知策略
type Notification struct {
	strategy.Service
}

func NewNotification(base strategy.Service) *Notification {
	return &Notification{
		Service: base,
	}
}

func (n *Notification) Send(ctx context.Context, info strategy.Info) (strategy.NotificationResponse, error) {
	// 1. 获取节点属性
	nodes, rawProps, err := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}
	property, err := easyflow.ToNodeProperty[easyflow.UserProperty](easyflow.Node{Properties: rawProps})
	if err != nil {
		return strategy.NotificationResponse{Msg: fmt.Sprintf("节点属性解析失败: %v", err)}, err
	}

	// 2. 加载基础数据
	data, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	// 3. 解析抄送人员并自动同步到节点
	users, err := n.ResolveAssignees(ctx, &info, property.NormalizeAssignees())
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	// 4. 异步处理
	n.SafeGo(ctx, 3*time.Minute, func(sendCtx context.Context) {
		n.asyncHandleCarbonCopy(sendCtx, info, data, strategy.NewRecipientMap(users, info.Channel))
	})

	return strategy.NotificationResponse{Msg: "success"}, nil
}

func (n *Notification) asyncHandleCarbonCopy(ctx context.Context, info strategy.Info, data *strategy.NotificationData, userMap strategy.RecipientMap) {
	// 1. 获取任务
	tasks, err := n.FetchTasksWithRetry(ctx, info)
	if err != nil {
		n.Logger().Error("CarbonCopy 获取任务失败", elog.FieldErr(err))
		return
	}

	// 2. 模拟发送通知
	if n.IsGlobalNotify(info.Workflow) {
		title := rule.GenerateCCTitle(data.StartUser.DisplayName, data.TName)

		for _, t := range tasks {
			n.Logger().Info("[NOTIFICATION CC] 模拟发送抄送通知",
				elog.Int("instance_id", info.InstID),
				elog.Int("taskId", t.TaskID),
				elog.String("title", title),
				elog.String("receiver", t.UserID),
				elog.Any("channel", info.Channel))
		}
	}

	// 3. 立即自动通过抄送任务，防止流程在此节点阻塞
	for _, t := range tasks {
		if err = n.PassTask(ctx, t.TaskID, "抄送节点自动通过"); err != nil {
			n.Logger().Error("抄送节点自动通过失败", elog.FieldErr(err), elog.Any("taskId", t.TaskID))
		}

		if t.IsCosigned != 1 {
			return
		}
	}
}
