package domain

// NotifyMethod 通知渠道类型定义
type NotifyMethod uint8

// ToUint8 转换类型为基本 uint8 表达
func (s NotifyMethod) ToUint8() uint8 {
	return uint8(s)
}

const (
	// Feishu 通过飞书进行审批流转及事件通知
	Feishu NotifyMethod = 1
	// Wechat 通过企业微信进行审批流转及事件通知
	Wechat NotifyMethod = 2
)

// NotifyType 通知触发时机/类型定义
type NotifyType string

const (
	// NotifyTypeApproval 审批人处理提醒通知
	NotifyTypeApproval NotifyType = "approval"
	// NotifyTypeCC 抄送通知
	NotifyTypeCC NotifyType = "carbon-copy"
	// NotifyTypeChat IM 群机器人消息群通知
	NotifyTypeChat NotifyType = "chat"
	// NotifyTypeProgress 流程流转进度通知
	NotifyTypeProgress NotifyType = "progress"
	// NotifyTypeProgressImageResult 附带结果图表的流转进度通知
	NotifyTypeProgressImageResult NotifyType = "progress-image-result"
	// NotifyTypeRevoke 发起人撤销通知
	NotifyTypeRevoke NotifyType = "revoke"
)

// NotifyTemplateSetKey 生成 ealert 侧默认通知模板集的业务唯一标识。
func NotifyTemplateSetKey(notifyType NotifyType) string {
	return "eflow.workflow.notify." + string(notifyType)
}

// Workflow 工作流流程定义领域模型
type Workflow struct {
	Id           int64        // 工作流主键 ID
	TemplateId   int64        // 关联的工单页面模板 ID
	Name         string       // 流程定义名称
	Icon         string       // 流程图标
	Owner        string       // 流程设计/负责人邮箱
	Desc         string       // 流程设计说明
	IsNotify     bool         // 是否在节点流转时触发通知
	NotifyMethod NotifyMethod // 默认的主通知渠道类型
	FlowData     LogicFlow    // 前端 LogicFlow 设计器直接传递并保存的画布拓扑原始数据
	ProcessId    int          // 后端依赖的流程执行引擎所生成的底层定义实例 ProcessDefID
}

// FlowEdge 流程图中线条/连线的元属性描述
type FlowEdge map[string]interface{}

// FlowNode 流程图中节点(审批、网关、自动化任务等)的数据元属性
type FlowNode map[string]interface{}

// LogicFlow 承载前端 LogicFlow 画布输出的图结构数据
type LogicFlow struct {
	Edges []FlowEdge // 画布中的所有连线/路由信息
	Nodes []FlowNode // 画布中的所有审批/自动/开始/结束节点数据
}

// Edge 具体的 LogicFlow 连接线描述，供程序处理审批条件和流转路径时使用
type Edge struct {
	Type         string      // 线类型 (如 Polyline)
	SourceNodeId string      // 起点节点本地 ID
	TargetNodeId string      // 终点节点本地 ID
	Properties   interface{} // 线的渲染与跳转判断表达式等属性
	ID           string      // 连线唯一 ID
}

// Node 具体的 LogicFlow 节点描述，供引擎识别处理人、条件网关分支时使用
type Node struct {
	Type       string      // 节点类型 (如 userTask, gateway, autoTask)
	Properties interface{} // 节点属性 (如审批用户、处理逻辑、网关条件等)
	ID         string      // 节点唯一本地 ID
}

// EdgeProperty 连接线所绑定的属性
type EdgeProperty struct {
	Expression string // 跳转的跳转判断逻辑表达式
}

// UserProperty 审批节点（userTask）的内置业务属性
type UserProperty struct {
	Name     string // 属性名称
	Approved string // 审批处理人（比如多个候选人，或是指定表达式如 ${creator}）
}

// StartProperty 开始节点所绑定的额外属性
type StartProperty struct {
	Name string
}

// EndProperty 结束节点所绑定的额外属性
type EndProperty struct {
	Name string
}

// ConditionProperty 条件网关（gateway）节点所绑定的属性
type ConditionProperty struct {
	Name string
}
