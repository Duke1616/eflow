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
	// NotifyTypeReject 审批驳回通知
	NotifyTypeReject NotifyType = "reject"
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

// RejectMode 审批驳回支持的模式常量
type RejectMode string

const (
	// RejectModeStarter 驳回至提单人（退回起点重新填报提审，仅 flowx 支持）
	RejectModeStarter RejectMode = "starter"
	// RejectModeHistory 驳回至历史审批节点（自由驳回至前序审批人，仅 flowx 支持）
	RejectModeHistory RejectMode = "history"
	// RejectModeTerminate 否决并直接终止流程（一票否决办结归档，所有引擎均支持）
	RejectModeTerminate RejectMode = "terminate"
)

// FlowPolicy 流程审批与流转治理策略配置
type FlowPolicy struct {
	AllowTransfer bool         `json:"allow_transfer"` // 是否允许转签工单给他人处理
	AllowReject   bool         `json:"allow_reject"`   // 是否允许驳回操作
	RejectModes   []RejectMode `json:"reject_modes"`   // 允许的驳回模式列表
}

// DefaultEasyflowPolicy 获取 easyflow 遗留引擎的默认受限策略（不支持转签与回退起点，仅允许否决终止）
func DefaultEasyflowPolicy() FlowPolicy {
	return FlowPolicy{
		AllowTransfer: false,
		AllowReject:   true,
		RejectModes:   []RejectMode{RejectModeTerminate},
	}
}

// DefaultFlowxPolicy 获取 flowx 现代图引擎的默认策略（转签默认关闭，驳回默认开启且限定为顺序驳回）
func DefaultFlowxPolicy() FlowPolicy {
	return FlowPolicy{
		AllowTransfer: false,
		AllowReject:   true,
		RejectModes:   []RejectMode{RejectModeHistory},
	}
}

// CanTransfer 是否允许转签
func (p FlowPolicy) CanTransfer() bool {
	return p.AllowTransfer
}

// CanReject 是否允许驳回
func (p FlowPolicy) CanReject() bool {
	return p.AllowReject
}

// AllowsMode 判断是否支持指定的驳回模式
func (p FlowPolicy) AllowsMode(mode RejectMode) bool {
	if !p.AllowReject {
		return false
	}
	for _, m := range p.RejectModes {
		if m == mode {
			return true
		}
	}
	return false
}

// CanTerminate 是否允许直接否决终止
func (p FlowPolicy) CanTerminate() bool {
	return p.AllowsMode(RejectModeTerminate)
}

// CanReturn 是否允许退回修改（无论是退回发起人还是退回历史环节）
func (p FlowPolicy) CanReturn() bool {
	return p.AllowsMode(RejectModeStarter) || p.AllowsMode(RejectModeHistory)
}

// ValidateReject 校验指定驳回操作是否符合当前策略约束
func (p FlowPolicy) ValidateReject(terminate bool) error {
	if !p.CanReject() {
		return fmt.Errorf("%w: 当前工作流配置不支持驳回操作", ErrForbidden)
	}
	if terminate {
		if !p.CanTerminate() {
			return fmt.Errorf("%w: 当前工作流配置不支持直接否决终止", ErrForbidden)
		}
		return nil
	}
	if !p.CanReturn() {
		return fmt.Errorf("%w: 当前工作流配置不支持退回修改操作", ErrForbidden)
	}
	return nil
}

// ValidateTransfer 校验转签操作是否符合当前策略约束
func (p FlowPolicy) ValidateTransfer() error {
	if !p.CanTransfer() {
		return fmt.Errorf("%w: 当前工作流配置不支持转签操作", ErrForbidden)
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
	FlowPolicy   FlowPolicy   // 流程审批与流转治理策略配置
}

// ResolveFlowPolicy 获取有效的流转策略配置（对历史未配置流程自动依据引擎自适应兜底）
func (w Workflow) ResolveFlowPolicy() FlowPolicy {
	engine := w.ResolveEngineType()
	policy := w.FlowPolicy

	// 若未配置任何策略（零值/历史空数据），依据底层引擎返回标准默认策略
	if !policy.AllowReject && !policy.AllowTransfer && len(policy.RejectModes) == 0 {
		if engine.IsFlowx() {
			return DefaultFlowxPolicy()
		}
		return DefaultEasyflowPolicy()
	}

	// 若开启了驳回但未配置任何驳回模式，采用引擎默认模式
	if len(policy.RejectModes) == 0 && policy.AllowReject {
		if engine.IsFlowx() {
			policy.RejectModes = []RejectMode{RejectModeHistory}
		} else {
			policy.RejectModes = []RejectMode{RejectModeTerminate}
		}
	}

	// 针对 easyflow 引擎施加强制降级保护：禁止转签，且驳回仅限直接否决终止
	if engine.IsEasyFlow() {
		policy.AllowTransfer = false
		if policy.AllowReject {
			policy.RejectModes = []RejectMode{RejectModeTerminate}
		} else {
			policy.RejectModes = nil
		}
	}

	return policy
}

// HasRejectMode 判断当前工作流是否允许指定的驳回模式
func (w Workflow) HasRejectMode(mode RejectMode) bool {
	return w.ResolveFlowPolicy().AllowsMode(mode)
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

// FormatFlowxProcessKey 生成标准的 FlowX 流程定义全局唯一 Key
func FormatFlowxProcessKey(workflowID int64) string {
	return fmt.Sprintf("wf_%d", workflowID)
}

// ParseWorkflowIDFromProcessKey 从 FlowX 流程定义 Key 中解析工作流 ID（若非标准格式则返回 0）
func ParseWorkflowIDFromProcessKey(procKey string) int64 {
	var id int64
	_, _ = fmt.Sscanf(procKey, "wf_%d", &id)
	return id
}
