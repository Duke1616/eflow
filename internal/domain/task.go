package domain

import (
	"strings"

	"github.com/Duke1616/eflow/internal/errs"
)

var (
	ErrTaskCannotRetry                  = errs.BadRequest("TASK_CANNOT_RETRY", "只有失败、阻塞或已取消的自动化任务可以重试")
	ErrTaskNotTerminable                = errs.BadRequest("TASK_NOT_TERMINABLE", "当前自动化任务不能终止")
	ErrTaskAlreadySucceeded             = errs.Conflict("TASK_ALREADY_SUCCEEDED", "任务已经成功")
	ErrCompensationNotTerminable        = errs.Conflict("COMPENSATION_NOT_TERMINABLE", "撤回补偿任务必须成功才能完成撤回")
	ErrCancelledTaskRequiresManualRetry = errs.BadRequest("CANCELLED_TASK_MANUAL_RETRY", "已取消的自动化任务只支持人工重试")
	ErrTaskMaxAttemptsExceeded          = errs.Conflict("TASK_MAX_ATTEMPTS_EXCEEDED", "超过最大执行尝试次数")
)

// TaskStatus 表示自动化节点在流程编排中的状态。
type TaskStatus uint8

const (
	TaskStatusSuccess    TaskStatus = 1
	TaskStatusFailed     TaskStatus = 2
	TaskStatusRunning    TaskStatus = 3
	TaskStatusWaiting    TaskStatus = 4
	TaskStatusBlocked    TaskStatus = 5
	TaskStatusSubmitting TaskStatus = 6
	TaskStatusCancelled  TaskStatus = 7
)

// ToUint8 返回用于持久化的状态值。
func (s TaskStatus) ToUint8() uint8 { return uint8(s) }

// CanRetry 判断编排任务是否允许创建新的执行尝试。
func (s TaskStatus) CanRetry() bool {
	return s == TaskStatusFailed || s == TaskStatusBlocked || s == TaskStatusCancelled
}

// CanTerminate 判断编排任务是否允许进入强制终止状态。
func (s TaskStatus) CanTerminate() bool {
	switch s {
	case TaskStatusWaiting, TaskStatusBlocked, TaskStatusFailed,
		TaskStatusSubmitting, TaskStatusRunning:
		return true
	default:
		return false
	}
}

// ToPhase 根据当前任务状态派生阶段文案枚举（表现层与向下兼容）。
func (s TaskStatus) ToPhase() TaskPhase {
	switch s {
	case TaskStatusSuccess:
		return TaskPhaseSucceeded
	case TaskStatusFailed:
		return TaskPhaseFailed
	case TaskStatusRunning:
		return TaskPhaseRunning
	case TaskStatusWaiting:
		return TaskPhaseReady
	case TaskStatusBlocked:
		return TaskPhaseBlocked
	case TaskStatusSubmitting:
		return TaskPhaseSubmitting
	case TaskStatusCancelled:
		return TaskPhaseCancelled
	default:
		return TaskPhaseReady
	}
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
	TaskPhaseCancelled  TaskPhase = "CANCELLED"
)

// TaskExecutionKind 区分正常流程任务与流程撤回后激活的补偿任务。
type TaskExecutionKind string

const (
	TaskExecutionProcess      TaskExecutionKind = "PROCESS"
	TaskExecutionCompensation TaskExecutionKind = "COMPENSATION"
)

// AllowsStart 判断指定工单状态下是否允许创建新的执行尝试。
func (k TaskExecutionKind) AllowsStart(ticketStatus Status) bool {
	return (k == TaskExecutionProcess && ticketStatus == PROCESS) ||
		(k == TaskExecutionCompensation && ticketStatus == WITHDRAWING)
}

// IsTerminal 判断执行尝试是否已经结束。
func (s AttemptStatus) IsTerminal() bool {
	return s == AttemptStatusSuccess || s == AttemptStatusFailed || s == AttemptStatusCancelled
}

// TaskArgs 是一次执行尝试保存的业务输入快照。
type TaskArgs = Variables

// Task 表示一个流程实例中的自动化节点，不保存 etask 执行实现细节。
type Task struct {
	ID                  int64
	TenantID            int64
	TicketID            int64
	InstanceID          ID // 流程实例唯一标识，对齐全局多态标识规范 domain.ID
	NodeID              string
	NodeName            string
	ProcessVersion      int
	Status              TaskStatus
	Phase               TaskPhase
	ScheduledAt         int64
	OriginalScheduledAt int64
	ExecutionKind       TaskExecutionKind
	CompensationNodeID  string
	CurrentAttemptID    int64
	AdvancedAt          int64
	CancelledAt         int64
	LastError           string
	Output              string // 当前成功尝试的结构化输出，仅用于领域查询，不在任务表重复持久化。
	CTime               int64
	UTime               int64
}

// NewProcessTask 创建流程编排自动化任务实体。
func NewProcessTask(tenantID, ticketID int64, instanceID ID, nodeID, nodeName string, version int) Task {
	return Task{
		TenantID:       tenantID,
		TicketID:       ticketID,
		InstanceID:     instanceID,
		NodeID:         nodeID,
		NodeName:       nodeName,
		ProcessVersion: version,
		Status:         TaskStatusWaiting,
		ExecutionKind:  TaskExecutionProcess,
	}
}

// Block 将任务置为阻塞状态。
func (t *Task) Block(reason string) {
	t.Status = TaskStatusBlocked
	t.LastError = reason
}

// Schedule 设置自动化任务计划执行时间与补偿节点。
func (t *Task) Schedule(scheduledAt int64, compensationNodeID string) {
	t.ScheduledAt = scheduledAt
	t.CompensationNodeID = compensationNodeID
}

// IsWaiting 判断任务是否处于等待启动状态。
func (t *Task) IsWaiting() bool {
	return t.Status == TaskStatusWaiting
}

// IsPendingSubmission 判断任务当前是否处于提交或执行中等待绑定状态。
func (t *Task) IsPendingSubmission() bool {
	return t.CurrentAttemptID > 0 && (t.Status == TaskStatusSubmitting || t.Status == TaskStatusRunning)
}

// ShouldStartImmediately 判断新创建的任务是否达到立即执行条件。
func (t *Task) ShouldStartImmediately(nowMillis int64, ticketStatus Status) bool {
	return t.ScheduledAt <= nowMillis && ticketStatus == PROCESS
}

// ValidateTerminable 校验当前任务是否允许强制终止。
func (t *Task) ValidateTerminable() error {
	if t.ExecutionKind == TaskExecutionCompensation {
		return ErrCompensationNotTerminable
	}
	if t.Status == TaskStatusSuccess {
		return ErrTaskAlreadySucceeded
	}
	// 已取消任务允许发起终止以幂等补发底层终结信号；其余状态必须符合可终止约束
	if t.Status != TaskStatusCancelled && !t.Status.CanTerminate() {
		return ErrTaskNotTerminable
	}
	return nil
}

// ValidateRetry 校验当前任务是否满足重试条件。
func (t *Task) ValidateRetry(automatic bool) error {
	if !t.Status.CanRetry() {
		return ErrTaskCannotRetry
	}
	if t.Status == TaskStatusCancelled && automatic {
		return ErrCancelledTaskRequiresManualRetry
	}
	return nil
}

// HasExecution 判断当前执行尝试是否已成功关联外部执行标识。
func (a TaskAttempt) HasExecution() bool {
	return a.ExecutionID > 0
}

// HasIdentifiableRequest 判断尝试是否存在有效的外部幂等标识。
func (a TaskAttempt) HasIdentifiableRequest() bool {
	return a.ExecutionID > 0 || strings.TrimSpace(a.RequestID) != ""
}

// NeedsRemoteTermination 判断执行尝试在终止时是否需要向外部执行引擎补发物理取消信号。
func (a TaskAttempt) NeedsRemoteTermination() bool {
	if a.ID <= 0 {
		return false
	}
	return !a.Status.IsTerminal() || a.Status == AttemptStatusCancelled
}

type AttemptStatus string

const (
	AttemptStatusSubmitting AttemptStatus = "SUBMITTING"
	AttemptStatusRunning    AttemptStatus = "RUNNING"
	AttemptStatusSuccess    AttemptStatus = "SUCCESS"
	AttemptStatusFailed     AttemptStatus = "FAILED"
	AttemptStatusCancelled  AttemptStatus = "CANCELLED"
)

// TaskAttempt 保存一次外部执行引用和 eflow 所需的业务快照。
type TaskAttempt struct {
	ID              int64
	TenantID        int64
	TaskID          int64
	AttemptNo       int
	RequestID       string
	DefaultRunnerID int64
	RunnerID        int64
	RouteRuleID         int64
	ExecutionID         int64
	MaxExecutionSeconds int64
	Status              AttemptStatus
	Input           TaskArgs
	Output          string
	Error           string
	SubmittedAt     int64
	CompletedAt     int64
	CTime           int64
	UTime           int64
}
