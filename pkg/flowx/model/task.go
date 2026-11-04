package model

import (
	"time"

	"github.com/samber/lo"
)

// TaskStatus 任务流转执行状态 (与 ecmdb-web 前端状态码 0, 1, 2, 3, 4, 5 严格绝对对齐)
type TaskStatus uint8

const (
	TaskPending        TaskStatus = 0 // 等待处理 (is_finished=0 待办处理中; is_finished=1 节点或签联动处理)
	TaskPassed         TaskStatus = 1 // 确认通过 (人工审核或自动化正常审批通过)
	TaskRejected       TaskStatus = 2 // 手动驳回 (审批人主动点击驳回)
	TaskSystemPassed   TaskStatus = 3 // 系统通过 (包容网关竞争或系统策略自动放行)
	TaskSystemRejected TaskStatus = 4 // 系统驳回 (会签有人驳回后，联动将兄弟待办标记为系统驳回取消)
	TaskSkipped        TaskStatus = 5 // 系统跳过 (条件网关未命中分支自动跳过)

	// TaskCancelled 别名映射为系统驳回 (Status = 4)，语义保持一致
	TaskCancelled = TaskSystemRejected
)

// Task 流转中的任务明细 (每个指派人一条记录)
type Task struct {
	ID         string     `json:"id"`
	InstanceID string     `json:"instance_id"`
	NodeID     string     `json:"node_id"`
	NodeName   string     `json:"node_name"`
	BatchCode  int        `json:"batch_code"` // 流转批次号 (驳回重走自增，历史不可变)
	Assignee   string     `json:"assignee"`   // 处理人
	SignRule   SignRule   `json:"sign_rule"`
	Status     TaskStatus `json:"status"`
	Comment    string     `json:"comment"`
	CreateTime time.Time  `json:"create_time"`
	FinishTime *time.Time `json:"finish_time,omitempty"`
}

// NewTask 构建标准任务实体（承载节点+实例+批次绑定关系的骨架）
func NewTask(id, instanceID string, node NodeSpec, batchCode int, assignee string, status TaskStatus) *Task {
	return &Task{
		ID: id, InstanceID: instanceID,
		NodeID: node.ID, NodeName: node.Name,
		BatchCode: batchCode, Assignee: assignee,
		SignRule: node.SignRule,
		Status: status, CreateTime: time.Now(),
	}
}

// Pass 审批人同意通过当前任务
func (t *Task) Pass(comment string) {
	t.Status = TaskPassed
	t.Comment = comment
	t.FinishTime = lo.ToPtr(time.Now())
}

// Reject 审批人驳回当前任务
func (t *Task) Reject(comment string) {
	t.Status = TaskRejected
	t.Comment = comment
	t.FinishTime = lo.ToPtr(time.Now())
}

// Skip 系统自动跳过当前任务 (分支条件未满足或或签竞争通过联动)
func (t *Task) Skip(comment string) {
	t.Status = TaskSkipped
	t.Comment = comment
	t.FinishTime = lo.ToPtr(time.Now())
}

// Cancel 系统或发起人取消废弃当前任务
func (t *Task) Cancel(comment string) {
	t.Status = TaskCancelled
	t.Comment = comment
	t.FinishTime = lo.ToPtr(time.Now())
}

// IsPending 判断任务是否处于等待处理态
func (t *Task) IsPending() bool {
	return t.Status == TaskPending
}

// IsPassed 判断任务是否已通过 (包含人工确认通过与系统自动通过)
func (t *Task) IsPassed() bool {
	return t.Status == TaskPassed || t.Status == TaskSystemPassed
}

// IsRejected 判断任务是否已被驳回 (包含手动驳回与系统驳回)
func (t *Task) IsRejected() bool {
	return t.Status == TaskRejected || t.Status == TaskSystemRejected
}

// IsTerminal 判断任务是否已处于终态办结 (非 Pending 即为定态)
func (t *Task) IsTerminal() bool {
	return t.Status != TaskPending
}

// TimelineEvent 面向前端展示的聚合流转时间线事件
type TimelineEvent struct {
	NodeID     string    `json:"node_id"`
	NodeName   string    `json:"node_name"`
	BatchCode  int       `json:"batch_code"`
	OccurredAt time.Time `json:"occurred_at"`
	Tasks      []Task    `json:"tasks"` // 本批次该节点的所有处理人记录
	IsFinished bool      `json:"is_finished"`
}
