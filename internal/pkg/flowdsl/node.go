package flowdsl

import (
	"strings"

	"github.com/samber/lo"
)

// 节点在画布未显式命名时的缺省兜底展示名称
const (
	DefaultNodeNameStart     = "开始"
	DefaultNodeNameEnd       = "结束"
	DefaultNodeNameUser      = "人工审批"
	DefaultNodeNameParallel  = "并行网关"
	DefaultNodeNameInclusion = "包容网关"
	DefaultNodeNameSelective = "排他网关"
)

// Node 统一标准工作流拓扑节点领域模型。
// 拓扑属性与各类型业务规格严格对称正交，消灭上帝对象与类型偏好。
type Node struct {
	ID   string   `json:"id"`
	Name string   `json:"name"`
	Type NodeType `json:"type"`

	// 强类型专属节点规格（按需装载，结构完全对称）
	UserTask  *UserTaskConfig  `json:"user_task,omitempty"`  // 人工审批/抄送任务规格
	AutoTask  *AutoTaskConfig  `json:"auto_task,omitempty"`  // 自动化任务规格
	ChatGroup *ChatGroupConfig `json:"chat_group,omitempty"` // IM 群聊广播通知规格
	Start     *StartConfig     `json:"start,omitempty"`      // 流程起始节点规格

	// 原始扩展属性（存放画布位置、坐标、非业务元数据）
	Props map[string]any `json:"props,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// 类型安全的守卫方法（Safe Guards）：杜绝空指针与类型不匹配
// ─────────────────────────────────────────────────────────────────────────────

// AsUserTask 安全转换为人工审批任务规格
func (n *Node) AsUserTask() (*UserTaskConfig, bool) {
	if n == nil || n.Type != NodeUserTask || n.UserTask == nil {
		return nil, false
	}
	return n.UserTask, true
}

// AsAutoTask 安全转换为自动化任务规格
func (n *Node) AsAutoTask() (*AutoTaskConfig, bool) {
	if n == nil || n.Type != NodeAutoTask || n.AutoTask == nil {
		return nil, false
	}
	return n.AutoTask, true
}

// AsChatGroup 安全转换为群聊广播规格
func (n *Node) AsChatGroup() (*ChatGroupConfig, bool) {
	if n == nil || n.Type != NodeChatGroup || n.ChatGroup == nil {
		return nil, false
	}
	return n.ChatGroup, true
}

// AsStart 安全转换为开始节点规格
func (n *Node) AsStart() (*StartConfig, bool) {
	if n == nil || n.Type != NodeStart || n.Start == nil {
		return nil, false
	}
	return n.Start, true
}

// ─────────────────────────────────────────────────────────────────────────────
// 基础类型语义判断
// ─────────────────────────────────────────────────────────────────────────────

func (n *Node) IsUserTask() bool  { return n != nil && n.Type == NodeUserTask }
func (n *Node) IsAutoTask() bool  { return n != nil && n.Type == NodeAutoTask }
func (n *Node) IsStart() bool     { return n != nil && n.Type == NodeStart }
func (n *Node) IsEnd() bool       { return n != nil && n.Type == NodeEnd }
func (n *Node) IsChatGroup() bool { return n != nil && n.Type == NodeChatGroup }

func (n *Node) IsGateway() bool {
	return n != nil && lo.Contains([]NodeType{
		NodeGateway,
		NodeGatewayParallel,
		NodeGatewayInclusive,
		NodeGatewayExclusive,
	}, n.Type)
}

// DisplayName 获取节点显示名称，若未显式命名则自适应返回该节点类型的缺省名称
func (n *Node) DisplayName() string {
	if n != nil && strings.TrimSpace(n.Name) != "" {
		return n.Name
	}
	if n != nil {
		return n.Type.DefaultName()
	}
	return ""
}

// DefaultName 返回该节点类型的缺省中文展示名称
func (t NodeType) DefaultName() string {
	switch t {
	case NodeStart:
		return DefaultNodeNameStart
	case NodeEnd:
		return DefaultNodeNameEnd
	case NodeUserTask:
		return DefaultNodeNameUser
	case NodeGatewayParallel:
		return DefaultNodeNameParallel
	case NodeGatewayInclusive:
		return DefaultNodeNameInclusion
	case NodeGatewayExclusive, NodeGatewayCondition, NodeGateway:
		return DefaultNodeNameSelective
	default:
		return ""
	}
}
