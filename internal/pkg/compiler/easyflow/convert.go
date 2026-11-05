package easyflow

import (
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
)

// 节点类型标识，映射统一领域标准 NodeType
const (
	NodeTypeStart     = flowdsl.NodeStart
	NodeTypeEnd       = flowdsl.NodeEnd
	NodeTypeUser      = flowdsl.NodeUserTask
	NodeTypeCondition = flowdsl.NodeGatewayCondition
	NodeTypeParallel  = flowdsl.NodeGatewayParallel
	NodeTypeInclusion = flowdsl.NodeGatewayInclusive
	NodeTypeSelective = flowdsl.NodeGatewayExclusive
	NodeTypeAuto      = flowdsl.NodeAutoTask
	NodeTypeChat      = flowdsl.NodeChatGroup
)

// 节点默认名称
const (
	DefaultNameStart     = "Start"
	DefaultNameEnd       = "End"
	DefaultNameUser      = "审批节点"
	DefaultNameParallel  = "并行网关"
	DefaultNameInclusion = "包容网关"
	DefaultNameSelective = "条件并行网关"
)

// 系统内置用户标识
const (
	UserStarter        = "$starter"   // 工单发起人
	AutomationApproval = "automation" // 自动化节点占位用户
	ChatGroupApproval  = "chat_group" // 群通知节点占位用户
	SysAutoUser        = "sys_auto"   // 系统代理节点自动审批用户
)

// SysProxyNodeName 系统代理节点名称
const SysProxyNodeName = "系统代理流转"

// 工作流引擎事件名称
const (
	// EventStart 开始节点触发事件
	EventStart = "EventStart"
	// EventNotify 通用通知事件（节点开始时通知审批人）
	EventNotify = "EventNotify"
	// EventCarbonCopy 抄送事件
	EventCarbonCopy = "EventCarbonCopy"
	// EventAutomation 自动化节点执行事件
	EventAutomation = "EventAutomation"
	// EventChatGroup 群通知节点事件
	EventChatGroup = "EventChatGroup"
	// EventSelectiveGatewaySplit 条件并行网关分裂事件
	EventSelectiveGatewaySplit = "EventSelectiveGatewaySplit"
	// EventRevoke 撤销事件
	EventRevoke = "EventRevoke"

	// EventTaskParallelNodePass 并行网关：分支通过事件
	EventTaskParallelNodePass = "EventTaskParallelNodePass"
	// EventTaskInclusionNodePass 包容网关：分支通过事件
	EventTaskInclusionNodePass = "EventTaskInclusionNodePass"
	// EventTaskSelectiveNodePass 条件并行网关：分支通过事件
	EventTaskSelectiveNodePass = "EventTaskSelectiveNodePass"

	// EventConcurrentRejectCleanup 并发拒绝时清理其他分支的事件（condition -> parallel/inclusion/selective）
	EventConcurrentRejectCleanup = "EventConcurrentRejectCleanup"
	// EventInclusionPassCleanup 包容网关通过时清理未完成分支的事件
	EventInclusionPassCleanup = "EventInclusionPassCleanup"
	// EventGatewayConditionReject 网关条件拒绝事件（gateway -> condition 场景下的拒绝）
	EventGatewayConditionReject = "EventGatewayConditionReject"
	// EventUserNodeRejectProxyCleanup 用户节点拒绝时清理代理节点的事件
	EventUserNodeRejectProxyCleanup = "EventUserNodeRejectProxyCleanup"
)

// NewConverter 创建包含所有标准节点处理器的 easy-workflow 转换器
func NewConverter() IConverter {
	return NewDefaultConverterWithHandlers()
}

// NewLogicFlowToEngineConvert 兼容旧构造函数命名
func NewLogicFlowToEngineConvert() IConverter {
	return NewConverter()
}
