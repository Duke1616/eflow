package domain

import (
	"errors"
	"time"

	"github.com/Duke1616/eflow/pkg/idx"
)

// ID 引擎实体（任务、实例等）强类型自适应标识，对齐基础设施通用包 pkg/idx.ID
type ID = idx.ID

// TaskID 流程审批任务领域强类型唯一标识（自适应整型自增与现代字符串形态）
type TaskID = idx.ID

// InstanceID 流程实例领域强类型唯一标识（自适应整型主键与现代字符串形态）
type InstanceID = idx.ID

// NewID 从任意整型或字符串创建强类型 ID
func NewID[T ~int | ~int64 | ~int32 | ~string](val T) ID {
	return idx.New(val)
}

// NewTaskID 从任意整型或字符串创建强类型任务标识
func NewTaskID[T ~int | ~int64 | ~int32 | ~string](val T) TaskID {
	return idx.New(val)
}

// NewInstanceID 从任意整型或字符串创建强类型流程实例标识
func NewInstanceID[T ~int | ~int64 | ~int32 | ~string](val T) InstanceID {
	return idx.New(val)
}

// DeployInfo 流程引擎部署完成返回的元信息
type DeployInfo struct {
	ProcessID int // 底层引擎的标识（若引擎无自增ID则为 0）
	Version   int // 部署发布的版本号
}

// ProcessLaunchReq 工单发起流程实例的强类型入参结构（单一事实来源 SSOT）
type ProcessLaunchReq struct {
	EngineType   EngineType // 流程底层引擎类型（可选，未指定时由调度器自动判定或默认兜底）
	TicketId     int64      // 业务工单唯一主键 ID（作为流程实例对齐的基石）
	WorkflowId   int64      // 工作流定义主键 ID
	ProcessId    int        // 兼容底层引擎的定义 ID（easy-workflow 为自增ID，flowx 为 wf.Id）
	Version      int        // 流程定义版本号（未指定或 <=0 时由底层引擎自动对齐最新已发布版本）
	WorkflowName string     // 流程名称
	Starter      string     // 流程发起人用户名/工号
	Variables    Variables  // 表单初始变量快照字典（权威唯一结构）
}

// Validate 编译期与运行时防呆自卫校验（Fail-Fast）
func (r ProcessLaunchReq) Validate() error {
	if r.TicketId <= 0 {
		return errors.New("工单 TicketId 非法")
	}
	if r.WorkflowId <= 0 {
		return errors.New("流程定义 WorkflowId 非法")
	}
	if r.Starter == "" {
		return errors.New("流程发起人 Starter 不能为空")
	}
	return nil
}

// ProcessLaunchResult 流程实例启动成功的返回结果
// 包含驱动该实例的引擎类型与自适应强类型实例标识（类型 + 标识 SSOT）
type ProcessLaunchResult struct {
	EngineType EngineType // 流程底层引擎类型
	InstanceID ID         // 实例全局唯一标识 (支持 Int() 与 String())
}

// TaskPassReq 审批通过请求结构
type TaskPassReq struct {
	EngineType EngineType // 所属引擎类型
	TaskId     ID         // 强类型任务唯一标识
	Operator   string     // 审批操作人
	Comment    string     // 审批意见
	Variables  Variables  // 增量变量合并
}

// Validate 审批通过入参自卫校验
func (r TaskPassReq) Validate() error {
	if r.TaskId.IsZero() {
		return errors.New("任务 ID 不能为空")
	}
	if r.Operator == "" {
		return errors.New("审批操作人 Operator 不能为空")
	}
	return nil
}

// TaskRejectReq 审批驳回请求结构
type TaskRejectReq struct {
	EngineType   EngineType // 所属引擎类型
	TaskId       ID         // 强类型任务唯一标识
	Operator     string     // 审批操作人
	Comment      string     // 驳回原因（必填）
	TargetNodeId string     // 回退目标节点 ID（可选）
}

// Validate 审批驳回入参自卫校验
func (r TaskRejectReq) Validate() error {
	if r.TaskId.IsZero() {
		return errors.New("任务 ID 不能为空")
	}
	if r.Operator == "" {
		return errors.New("审批操作人 Operator 不能为空")
	}
	if r.Comment == "" {
		return errors.New("驳回原因 Comment 不能为空")
	}
	return nil
}

// TaskTransferReq 任务转签请求结构（封装转签操作要素，避免多参数传递）
type TaskTransferReq struct {
	EngineType  EngineType `json:"engine_type"`
	TaskId      ID         `json:"task_id"`
	TargetUsers []string   `json:"target_users"`
}

// Validate 任务转签入参自卫校验
func (r TaskTransferReq) Validate() error {
	if r.TaskId.IsZero() {
		return errors.New("任务 ID 不能为空")
	}
	if len(r.TargetUsers) == 0 {
		return errors.New("目标转签人列表不能为空")
	}
	return nil
}

// RevokeReq 流程实例撤销请求结构（消除裸布尔值传递，保证调用意图显式化）
type RevokeReq struct {
	EngineType EngineType `json:"engine_type"`
	InstanceID ID         `json:"instance_id"`
	Force      bool       `json:"force"`
	Operator   string     `json:"operator"`
}

// Validate 流程撤销入参自卫校验
func (r RevokeReq) Validate() error {
	if r.InstanceID.IsZero() {
		return errors.New("流程实例 ID 不能为空")
	}
	if r.Operator == "" {
		return errors.New("撤销操作人 Operator 不能为空")
	}
	return nil
}

// TimelineNode 跨引擎统一流转时间线节点视图
type TimelineNode struct {
	NodeID       string         // 流程节点全局唯一标识
	NodeName     string         // 流程节点展示名称
	NodeType     string         // 节点类型（如 START, APPROVAL, AUTOMATION, END）
	Status       string         // 节点流转状态（PENDING, RUNNING, COMPLETED, REJECTED, TERMINATED, SKIPPED）
	Batch        int            // 执行批次号（用于回退场景区分多次流转）
	Operator     string         // 处理人账号/工号
	OperatorName string         // 处理人中文真名
	Comment      string         // 审批意见或流转批注
	StartTime    string         // 节点激活/到达时间
	EndTime      string         // 节点完结/流出时间
	Extra        map[string]any // 扩展信息（如增量变量、执行器详情）
}

// TaskContext 跨引擎统一的任务流转运行时上下文快照
// 供工单业务层执行审批鉴权、节点表单查找、状态判断使用
type TaskContext struct {
	EngineType        EngineType // 所属工作流底层引擎类型
	TaskID            ID         // 任务强类型 ID
	ProcessInstanceID ID         // 流程实例强类型 ID
	WorkflowID        int64      // 关联的工作流定义主键 ID
	ProcessKey        string     // 流程全局唯一标识 Key（FlowX 对应 procKey 如 wf_12）
	ProcessID         int        // Deprecated: 历史流程定义 ID（仅用于兼容 EasyFlow 引擎存量实例）
	ProcessVersion    int        // 流程定义版本号
	NodeID            string     // 节点物理标识
	NodeName          string     // 节点名称
	Assignee          string     // 当前指派处理人账号
	IsFinished        bool       // 是否已完结
	Comment           string     // 审批批注
}

// ValidateOperable 断言当前任务是否允许指定操作人执行流转（已办结拦截与操作人权限隔离）
func (t TaskContext) ValidateOperable(operator string, isAdmin bool) error {
	return ValidateTaskOperable(t.Assignee, t.IsFinished, operator, isAdmin)
}

// InstanceExecutionView 流程实例运行时轨迹与点亮视图
type InstanceExecutionView struct {
	EngineType     EngineType          // 产生该执行视图的底层引擎类型 (EasyFlow / FlowX)
	WorkflowID     int64               // 关联的工作流定义主键 ID
	ProcessKey     string              // 流程全局唯一标识 Key（FlowX 对应 procKey）
	ProcessID      int                 // Deprecated: 历史流程定义 ID（仅用于兼容 EasyFlow 引擎存量实例）
	ProcessVersion int                 // 流程定义版本号
	NodeStatusMap  map[string]int      // 节点状态映射 [NodeID] -> 状态编码
	TraversedEdges map[string][]string // 已激活流转的拓扑边映射 [SourceID] -> []TargetID
	PendingUsers   []string            // 当前等待审批的处理人列表
}

// Instance 流程在引擎中运行流转的实例领域模型（兼容历史待办连表视图）
type Instance struct {
	TaskID          int        // 任务 ID
	ProcInstID      int        // 流程实例 ID
	ProcID          int        // 流程 ID
	ProcName        string     // 流程名称
	ProcVersion     int        // 流程版本号
	BusinessID      string     // 关联的工单系统生成的业务唯一单号
	Starter         string     // 流程发起人显示名称或用户 ID
	CurrentNodeID   string     // 流程当前正处在的节点物理 ID
	CurrentNodeName string     // 流程当前正处在的节点展示名称
	CreateTime      *time.Time // 实例整体的发起/创建时间
	ApprovedBy      string     // 当前处理人
	Status          int        // 实例状态（0: 未完成审批中, 1: 已完成通过, 2: 撤销）
}

// TaskTimelineGroup 是面向工单流转时间线的节点执行汇总（跨引擎通用批次聚合视图）
type TaskTimelineGroup struct {
	NodeID              string       // 流程节点 ID
	NodeName            string       // 节点展示名称
	BatchCode           string       // 节点执行批次；驳回重走会产生新批次
	IsCosigned          int          // 0: 或签，1: 会签
	TaskCount           int          // 该节点批次中的任务总数
	PassedCount         int          // 人工通过任务数
	RejectedCount       int          // 人工驳回任务数
	SystemPassedCount   int          // 因联动被系统通过的任务数
	SystemRejectedCount int          // 因联动被系统驳回的任务数
	SkippedCount        int          // 条件不满足而跳过的任务数
	LinkedCount         int          // 节点或签联动结束、但未改变原始状态的任务数
	PendingCount        int          // 尚未完成的任务数
	OccurredAt          string       // 本次节点执行的最新活动时间
	Members             []TaskRecord // 该批次下的具体处理人任务明细
}

// TodoTask 跨引擎统一的待办任务领域模型（Value Semantics 优先，消灭底层异构差异）
type TodoTask struct {
	EngineType EngineType `json:"engine_type"`
	TaskID     ID         `json:"task_id"`
	InstanceID ID         `json:"instance_id"`
	NodeID     string     `json:"node_id"`
	NodeName   string     `json:"node_name"`
	ApprovedBy string     `json:"approved_by"`
	CreateTime time.Time  `json:"create_time"`
}

// TodoQueryReq 跨引擎待办任务检索条件请求（单一事实来源 SSOT）
type TodoQueryReq struct {
	Username    string `json:"username"`
	ProcessName string `json:"process_name"`
	SortByAsc   bool   `json:"sort_by_asc"`
	Offset      int    `json:"offset"`
	Limit       int    `json:"limit"`
	IsAdmin     bool   `json:"is_admin"` // 是否查询全量待办（管理员视图，不过滤当前处理人）
}
