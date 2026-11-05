package domain

import (
	"fmt"
)

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

// EngineType 流程底层执行引擎类型定义（DDD 领域值对象）
type EngineType string

// String 转换为基础 string 字符串
func (e EngineType) String() string {
	return string(e)
}

const (
	// EngineTypeEasyFlow 遗留基于第三方 easy-workflow 的图执行引擎
	EngineTypeEasyFlow EngineType = "easy-workflow"
	// EngineTypeFlowx 现代自研的高性能事件驱动 flowx 流程引擎
	EngineTypeFlowx EngineType = "flowx"
)

// IsValid 校验引擎类型是否合法有效
func (e EngineType) IsValid() bool {
	return e == EngineTypeEasyFlow || e == EngineTypeFlowx
}

// IsFlowx 判断是否为 flowx 引擎
func (e EngineType) IsFlowx() bool {
	return e == EngineTypeFlowx
}

// IsEasyFlow 判断是否为 easy-workflow 引擎
func (e EngineType) IsEasyFlow() bool {
	return e == EngineTypeEasyFlow
}

// CanvasData 流程图画布通用原始数据（底层为 map[string]any，支持 LogicFlow、AntV X6、BPMN 等任意前端结构）
type CanvasData map[string]any

// Nodes 辅助方法：以切片形式获取画布中的节点原始数据（若存在）
func (c CanvasData) Nodes() []any {
	if raw, ok := c["nodes"].([]any); ok {
		return raw
	}
	return nil
}

// Edges 辅助方法：以切片形式获取画布中的连线原始数据（若存在）
func (c CanvasData) Edges() []any {
	if raw, ok := c["edges"].([]any); ok {
		return raw
	}
	return nil
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
	FlowData     CanvasData   // 流程图画布原始数据（中立结构，支持 LogicFlow / AntV X6 / BPMN 等）
	ProcessId    int          // 后端依赖的流程执行引擎所生成的底层定义实例 ProcessDefID
	EngineType   EngineType   // 关联的流程执行引擎类型（值对象：easy-workflow, flowx）
}

// ResolveEngineType 获取有效引擎类型（为空或未指定时默认兜底为 EngineTypeEasyFlow 保障老数据兼容）
func (w Workflow) ResolveEngineType() EngineType {
	if w.EngineType.IsValid() {
		return w.EngineType
	}
	return EngineTypeEasyFlow
}

// ValidateEngineType 校验流程引擎类型的合法性（空值允许，视为默认历史引擎）
func (w Workflow) ValidateEngineType() error {
	if w.EngineType == "" || w.EngineType.IsValid() {
		return nil
	}
	return fmt.Errorf("不支持的流程引擎类型: %s", w.EngineType)
}

// AutomationNodeRef 流程自动化节点描述引用
type AutomationNodeRef struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	CodebookID int64  `json:"codebook_id"`
	RunnerID   int64  `json:"runner_id"`
}
