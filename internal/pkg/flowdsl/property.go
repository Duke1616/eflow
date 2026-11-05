package flowdsl

import "github.com/samber/lo"

// ─────────────────────────────────────────────────────────────────────────────
// 节点类型值对象
// ─────────────────────────────────────────────────────────────────────────────

// NodeType 节点标准类型（值对象）
type NodeType string

func (t NodeType) ToString() string { return string(t) }

const (
	NodeStart            NodeType = "start"
	NodeEnd              NodeType = "end"
	NodeUserTask         NodeType = "userTask"
	NodeAutoTask         NodeType = "autoTask"
	NodeGatewayParallel  NodeType = "parallel"  // 并行网关 (AND)
	NodeGatewayInclusive NodeType = "inclusion" // 包容网关 (OR)
	NodeGatewayExclusive NodeType = "selective" // 排他网关 (XOR)
	NodeGatewayCondition NodeType = "condition" // 条件分支网关 (LogicFlow 级联排他分支)
	NodeGateway          NodeType = "gateway"   // 默认通用网关 (作为排他网关语义)
	NodeChatGroup        NodeType = "chatGroup" // IM 群聊广播通知
)

// NormalizeNodeType 统一归一化各类前端画布的节点类型别名，保证系统内部类型唯一标准
func NormalizeNodeType(rawType string) NodeType {
	switch rawType {
	case "user", "userTask", "user-task":
		return NodeUserTask
	case "automation", "autoTask", "auto-task":
		return NodeAutoTask
	case "start", "startNode", "start-node":
		return NodeStart
	case "end", "endNode", "end-node":
		return NodeEnd
	case "parallel", "parallelGateway", "parallel-gateway":
		return NodeGatewayParallel
	case "inclusion", "inclusiveGateway", "inclusive-gateway":
		return NodeGatewayInclusive
	case "condition", "conditionGateway", "condition-gateway":
		return NodeGatewayCondition
	case "selective", "exclusiveGateway", "exclusive-gateway":
		return NodeGatewayExclusive
	case "gateway":
		return NodeGateway
	case "chat", "chatGroup", "chat-group":
		return NodeChatGroup
	default:
		return NodeType(rawType)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 审批与表单值对象
// ─────────────────────────────────────────────────────────────────────────────

// Rule 审批分配匹配策略（值对象）
type Rule string

func (r Rule) ToString() string { return string(r) }

const (
	APPOINT    Rule = "appoint"     // 指定内部用户工号
	FOUNDER    Rule = "founder"     // 工单发起人
	TEMPLATE   Rule = "template"    // 依据表单/模板字段动态提取用户
	LEADER     Rule = "leaders"     // 申请人直属领导
	MainLeader Rule = "main_leader" // 申请人分管领导
	DEPARTMENT Rule = "department"  // 指定部门全员
	OnCall     Rule = "on_call"     // 排班/值班人员
	TEAM       Rule = "team"        // 所属团队成员
)

// Assignee 审批人员分配规则
type Assignee struct {
	Rule   Rule     `json:"rule"`   // 分配策略
	Values []string `json:"values"` // 目标参数值（工号、角色、部门标识等）
}

// ContainsValue 判断分配规则是否包含指定值
func (a Assignee) ContainsValue(val string) bool {
	return lo.Contains(a.Values, val)
}

// FieldType 动态表单字段类型
type FieldType string

func (f FieldType) ToString() string { return string(f) }

const (
	FieldInput       FieldType = "input"        // 单行输入
	FieldTextarea    FieldType = "textarea"     // 多行文本
	FieldNumber      FieldType = "number"       // 数值类型
	FieldDate        FieldType = "date"         // 日期类型
	FieldSelect      FieldType = "select"       // 单选下拉
	FieldMultiSelect FieldType = "multi_select" // 多选下拉
	FieldTips        FieldType = "tips"         // 静态说明文本
)

// Option 单选/多选下拉选项键值对
type Option struct {
	Label string `json:"label"` // 选项显示名
	Value string `json:"value"` // 选项实际值
}

// Field 动态表单字段项控制规格
type Field struct {
	Name     string            `json:"name"`               // 字段展示标签
	Key      string            `json:"key"`                // 工单持久化映射 Key
	Type     FieldType         `json:"type"`               // 组件渲染控件类型
	Required bool              `json:"required"`           // 是否必填
	Options  []Option          `json:"options,omitempty"`  // 下拉候选字典集
	Props    map[string]string `json:"props,omitempty"`    // 组件额外展示属性（如 placeholder）
	Merge    bool              `json:"merge,omitempty"`    // 是否在后续节点合并推进展出
	Validate string            `json:"validate,omitempty"` // 校验正则表达式
	Hidden   bool              `json:"hidden,omitempty"`   // 是否在审批表单中隐匿
	Value    string            `json:"value,omitempty"`    // 表单预填默认值
	ReadOnly bool              `json:"readonly,omitempty"` // 是否只读展示
}

// UserTaskConfig 人工审批/填写任务专属配置规格
type UserTaskConfig struct {
	Assignees  []Assignee `json:"assignees,omitempty"`   // 审批人分配规则列表
	Fields     []Field    `json:"fields,omitempty"`      // 关联的动态表单权限配置
	IsCosigned bool       `json:"is_cosigned,omitempty"` // 是否会签
	IsCC       bool       `json:"is_cc,omitempty"`       // 是否抄送节点
}

// ─────────────────────────────────────────────────────────────────────────────
// 自动化任务与调度配置
// ─────────────────────────────────────────────────────────────────────────────

// ScheduleType 调度方式
type ScheduleType string

const (
	ScheduleImmediate ScheduleType = "immediate" // 立即执行
	ScheduleDelay     ScheduleType = "delay"     // 相对延迟执行
	ScheduleAt        ScheduleType = "at"        // 指定绝对时间点
)

// ScheduleSourceType 调度数值获取来源
type ScheduleSourceType string

const (
	ScheduleSourceFixed         ScheduleSourceType = "fixed"          // 固定静态时长/时间
	ScheduleSourceTemplateField ScheduleSourceType = "template_field" // 提取表单动态字段
)

// ScheduleUnit 延迟时间单位
type ScheduleUnit string

const (
	ScheduleUnitMinute ScheduleUnit = "minute"
	ScheduleUnitHour   ScheduleUnit = "hour"
	ScheduleUnitDay    ScheduleUnit = "day"
)

// ScheduleSource 调度源取值描述
type ScheduleSource struct {
	Type       ScheduleSourceType `json:"type"`
	Value      int64              `json:"value,omitempty"`
	TemplateID int64              `json:"template_id,omitempty"`
	Field      string             `json:"field,omitempty"`
	TimeField  string             `json:"time_field,omitempty"`
}

// ScheduleConfig 自动化执行计划
type ScheduleConfig struct {
	Type     ScheduleType   `json:"type"`
	Source   ScheduleSource `json:"source,omitempty"`
	Unit     ScheduleUnit   `json:"unit,omitempty"`
	Timezone string         `json:"timezone,omitempty"`
}

// AutoTaskConfig 自动化节点专属执行规格（彻底移除废弃的 is_timing 等历史字段）
type AutoTaskConfig struct {
	CodebookID         int64           `json:"codebook_id"`                    // 绑定的自动化执行脚本 ID
	RunnerID           int64           `json:"runner_id"`                      // 指定的执行节点 Runner ID
	NotifyMethod       []int64         `json:"notify_method,omitempty"`        // 执行后通知渠道
	IsNotify           bool            `json:"is_notify"`                      // 是否开启消息提醒
	Schedule           *ScheduleConfig `json:"schedule,omitempty"`             // 任务延期或计划调度配置
	CompensationNodeID string          `json:"compensation_node_id,omitempty"` // 撤回时的补偿冲正节点 ID
}

// ─────────────────────────────────────────────────────────────────────────────
// 群聊与开始节点配置
// ─────────────────────────────────────────────────────────────────────────────

// StartConfig 开始节点专属配置
type StartConfig struct {
	IsNotify bool `json:"is_notify,omitempty"`
}

// ChatGroupMode 建群模式
type ChatGroupMode string

const (
	ChatGroupUseExisting ChatGroupMode = "existing"
	ChatGroupCreate      ChatGroupMode = "create"
)

// OutputMode 消息卡片支持展示的输出数据类型
type OutputMode string

const (
	OutputTicketData OutputMode = "ticket_data" // 工单提交信息
	OutputAutoTask   OutputMode = "auto_task"   // 自动化任务返回结果
	OutputUserInput  OutputMode = "user_input"  // 用户节点提交信息
)

// CreateChatGroup create 模式下的群创建配置
type CreateChatGroup struct {
	Name    string `json:"name"`    // 创建群名称
	Channel string `json:"channel"` // 通知渠道
}

// ChatGroupConfig 群聊通知节点专属配置
type ChatGroupConfig struct {
	Title        string           `json:"title"`                    // 消息卡片标题
	Mode         ChatGroupMode    `json:"mode"`                     // existing / create
	ChatGroupIDs []int64          `json:"chat_group_ids,omitempty"` // existing 模式群组 ID 列表
	Create       *CreateChatGroup `json:"create,omitempty"`         // create 模式配置
	Assignees    []Assignee       `json:"assignees,omitempty"`      // 成员规则
	OutputMode   []OutputMode     `json:"is_auto"`                  // 支持的输出数据
}

// ─────────────────────────────────────────────────────────────────────────────
// 有向连线模型
// ─────────────────────────────────────────────────────────────────────────────

// Edge 统一有向连线拓扑模型
type Edge struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	Expression string `json:"expression,omitempty"` // 条件网关跳转表达式（如 "${amount} > 100"）
	IsPass     bool   `json:"is_pass,omitempty"`    // 详情回显点亮标记：当前连线是否已流转通过
	IsSkipped  bool   `json:"is_skipped,omitempty"` // 详情回显跳过标记：当前连线分支是否因条件未满足跳过
}
