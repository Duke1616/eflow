package domain

// TaskStatus 表示自动化节点在流程编排中的状态。
type TaskStatus uint8

const (
	TaskStatusSuccess    TaskStatus = 1
	TaskStatusFailed     TaskStatus = 2
	TaskStatusRunning    TaskStatus = 3
	TaskStatusWaiting    TaskStatus = 4
	TaskStatusBlocked    TaskStatus = 5
	TaskStatusSubmitting TaskStatus = 6
)

// ToUint8 返回用于持久化的状态值。
func (s TaskStatus) ToUint8() uint8 { return uint8(s) }

// CanRetry 判断编排任务是否允许创建新的执行尝试。
func (s TaskStatus) CanRetry() bool {
	return s == TaskStatusFailed || s == TaskStatusBlocked
}

// TaskPhase 描述最近一次编排动作，展示文案由表现层转换。
type TaskPhase string

const (
	TaskPhaseReady      TaskPhase = "READY"
	TaskPhaseSubmitting TaskPhase = "SUBMITTING"
	TaskPhaseRunning    TaskPhase = "RUNNING"
	TaskPhaseSucceeded  TaskPhase = "SUCCEEDED"
	TaskPhaseFailed     TaskPhase = "FAILED"
	TaskPhaseBlocked    TaskPhase = "BLOCKED"
	TaskPhaseRetrying   TaskPhase = "RETRYING"
)

// IsTerminal 判断执行尝试是否已经结束。
func (s AttemptStatus) IsTerminal() bool {
	return s == AttemptStatusSuccess || s == AttemptStatusFailed
}

// TaskArgs 是一次执行尝试保存的业务输入快照。
type TaskArgs map[string]any

// Task 表示一个流程实例中的自动化节点，不保存 etask 执行实现细节。
type Task struct {
	ID                int64
	TenantID          int64
	TicketID          int64
	ProcessInstanceID int
	NodeID            string
	NodeName          string
	ProcessVersion    int
	Status            TaskStatus
	Phase             TaskPhase
	ScheduledAt       int64
	CurrentAttemptID  int64
	AdvancedAt        int64
	LastError         string
	Output            string // 当前成功尝试的结构化输出，仅用于领域查询，不在任务表重复持久化。
	CTime             int64
	UTime             int64
}

// AttemptStatus 表示一次 etask 提交尝试的生命周期。
type AttemptStatus string

const (
	AttemptStatusSubmitting AttemptStatus = "SUBMITTING"
	AttemptStatusRunning    AttemptStatus = "RUNNING"
	AttemptStatusSuccess    AttemptStatus = "SUCCESS"
	AttemptStatusFailed     AttemptStatus = "FAILED"
)

// TaskAttempt 保存一次外部执行引用和 eflow 所需的业务快照。
type TaskAttempt struct {
	ID          int64
	TenantID    int64
	TaskID      int64
	AttemptNo   int
	RequestID   string
	RunnerID    int64
	ExecutionID int64
	Status      AttemptStatus
	Input       TaskArgs
	Output      string
	Error       string
	SubmittedAt int64
	CompletedAt int64
	CTime       int64
	UTime       int64
}
