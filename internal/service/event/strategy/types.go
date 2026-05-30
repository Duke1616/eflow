package strategy

import (
	"context"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
)

type NodeName string

const (
	Start      NodeName = "START"       // 开始节点
	Automation NodeName = "AUTOMATION"  // 自动化节点
	User       NodeName = "USER"        // 用户审批节点
	CarbonCopy NodeName = "CARBON_COPY" // 抄送节点 (Carbon Copy)
	ChatGroup  NodeName = "CHAT_GROUP"  // 群通知节点
)

// 通知渠道定义
type Channel string

const (
	ChannelLarkCard Channel = "LARK_CARD"
	ChannelWechat   Channel = "WECHAT"
	ChannelEmail    Channel = "EMAIL"
	ChannelInApp    Channel = "IN_APP"
)

// NotificationResponse 模拟通知发送结果
type NotificationResponse struct {
	Msg string
}

type FlowContext struct {
	InstID      int             // 流程实例 ID
	Order       domain.Ticket   // 工单实体
	Workflow    domain.Workflow // 流程定义快照
	Instance    domain.Instance // 引擎实例状态
	CurrentNode *model.Node     // 当前触发事件的节点
	Nodes       []easyflow.Node // 解析后的流程节点
}

type Info struct {
	NodeName    NodeName `json:"node_name"` // 节点名称
	Channel     Channel  `json:"channel"`   // 通知渠道
	FlowContext          // 嵌入流程上下文
}

// SendStrategy 针对不同节点的策略接口
type SendStrategy interface {
	Send(ctx context.Context, info Info) (NotificationResponse, error)
}

type Dispatcher struct {
	userStrategy       SendStrategy
	autoStrategy       SendStrategy
	startStrategy      SendStrategy
	chatStrategy       SendStrategy
	carbonCopyStrategy SendStrategy
	base               Service
}

func NewDispatcher(user SendStrategy, auto SendStrategy,
	start SendStrategy, chat SendStrategy, carbonCopy SendStrategy, base Service) *Dispatcher {
	return &Dispatcher{
		userStrategy:       user,
		autoStrategy:       auto,
		startStrategy:      start,
		chatStrategy:       chat,
		carbonCopyStrategy: carbonCopy,
		base:               base,
	}
}

func (d *Dispatcher) Send(ctx context.Context, info Info) (NotificationResponse, error) {
	strategy := d.selectStrategy(info)

	// 1. 预解析流程节点，避免策略内重复解析
	if nodes, err := UnmarshalNodes(info.Workflow); err == nil {
		info.Nodes = nodes
	}

	// 2. 分发器根据流程属性注入通知渠道类型
	if info.Channel == "" {
		info.Channel = getChannel(info.Workflow)
	}

	return strategy.Send(ctx, info)
}

// GetChannel 根据流程配置获取通知渠道
func getChannel(wf domain.Workflow) Channel {
	// 这里 wf.NotifyMethod 为 2 则为微信，1 为飞书
	switch wf.NotifyMethod {
	case 1:
		return ChannelLarkCard
	case 2:
		return ChannelWechat
	default:
		return ChannelLarkCard
	}
}

func (d *Dispatcher) selectStrategy(not Info) SendStrategy {
	switch not.NodeName {
	case Start:
		return d.startStrategy
	case Automation:
		return d.autoStrategy
	case User:
		return d.userStrategy
	case CarbonCopy:
		return d.carbonCopyStrategy
	case ChatGroup:
		return d.chatStrategy
	default:
		return d.userStrategy
	}
}
