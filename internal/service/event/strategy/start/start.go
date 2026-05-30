package start

import (
	"context"

	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/gotomicro/ego/core/elog"
)

type Notification struct {
	strategy.Service
}

// NewNotification 构造流程启动通知策略
func NewNotification(base strategy.Service) *Notification {
	return &Notification{
		Service: base,
	}
}

func (s *Notification) Send(ctx context.Context, info strategy.Info) (strategy.NotificationResponse, error) {
	// 1. 全局通知校验
	if !s.IsGlobalNotify(info.Workflow) {
		return strategy.NotificationResponse{Msg: "全局通知已关闭"}, nil
	}

	s.Logger().Debug("开始节点发送通知",
		elog.Int("instance_id", info.InstID),
		elog.String("node_id", info.CurrentNode.NodeID))

	// 2. 加载基础通知元数据
	nodes, _, err := s.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	data, err := s.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	title := rule.GenerateTitle(data.StartUser.DisplayName, data.TName)

	// 3. 模拟发送通知给发起人
	s.Logger().Info("[NOTIFICATION start] 模拟发送流程启动通知给发起人",
		elog.Int("instance_id", info.InstID),
		elog.String("title", title),
		elog.String("starter", data.StartUser.Username),
		elog.Any("channel", info.Channel))

	return strategy.NotificationResponse{Msg: "success"}, nil
}
