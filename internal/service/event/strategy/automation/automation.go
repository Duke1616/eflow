package automation

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/rule"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
)

const (
	// ProcessEndSend 流程结束后发送
	ProcessEndSend = 1
	// ProcessNowSend 当前节点通过直接发送
	ProcessNowSend = 2
)

// Notification 自动化节点通知策略
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
	_, rawProps, err := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, fmt.Errorf("节点属性未配置: %v", err)
	}

	property, err := easyflow.ToNodeProperty[easyflow.AutomationProperty](easyflow.Node{Properties: rawProps})
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	// 2. 权限与触发校验
	if !n.IsGlobalNotify(info.Workflow) {
		return strategy.NotificationResponse{Msg: "全局通知已关闭"}, nil
	}

	if !property.IsNotify {
		return strategy.NotificationResponse{Msg: "【自动化节点】未开启消息通知"}, nil
	}

	if !containsAutoNotifyMethod(property.NotifyMethod, ProcessNowSend) {
		return strategy.NotificationResponse{Msg: "【自动化节点】节点未开启消息通知模式"}, nil
	}

	// 3. 获取元数据与自动化任务结果
	nodes, _, _ := n.GetNodeProperty(info, info.CurrentNode.NodeID)
	data, err := n.FetchRequiredData(ctx, info, nodes)
	if err != nil {
		return strategy.NotificationResponse{Msg: err.Error()}, err
	}

	title := rule.GenerateAutoTitle("你提交", data.TName)

	// 4. 模拟发送消息
	n.Logger().Info("[NOTIFICATION automation] 模拟发送自动化执行结果通知给发起人",
		elog.Int("instance_id", info.InstID),
		elog.String("title", title),
		elog.String("receiver", data.StartUser.Username),
		elog.Any("channel", info.Channel))

	return strategy.NotificationResponse{Msg: "success"}, nil
}

func containsAutoNotifyMethod(methods []int64, target int64) bool {
	return slice.Contains(methods, target)
}
