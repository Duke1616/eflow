package dao

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskAttempt 是一次提交到 etask 的执行尝试。
type TaskAttempt struct {
	ID          int64                           `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'执行尝试主键'"`
	TenantID    int64                           `gorm:"column:tenant_id;type:bigint;not null;uniqueIndex:uk_attempt_request,priority:1;index;comment:'租户 ID'"`
	TaskID      int64                           `gorm:"column:task_id;type:bigint;not null;uniqueIndex:uk_task_attempt,priority:1;index;comment:'自动化任务 ID'"`
	AttemptNo   int                             `gorm:"column:attempt_no;type:int;not null;uniqueIndex:uk_task_attempt,priority:2;comment:'任务内尝试序号'"`
	RequestID   string                          `gorm:"column:request_id;type:varchar(128);not null;uniqueIndex:uk_attempt_request,priority:2;comment:'etask 幂等请求标识'"`
	RunnerID    int64                           `gorm:"column:runner_id;type:bigint;not null;index;comment:'本次选择的 Runner ID'"`
	ExecutionID sql.NullInt64                   `gorm:"column:execution_id;type:bigint;uniqueIndex;comment:'etask 执行 ID'"`
	Status      string                          `gorm:"column:status;type:varchar(24);not null;index;comment:'执行尝试状态'"`
	Input       sqlx.JsonField[domain.TaskArgs] `gorm:"column:input;type:json;comment:'业务输入快照'"`
	Output      string                          `gorm:"column:output;type:mediumtext;comment:'结构化执行输出'"`
	Error       string                          `gorm:"column:error_message;type:text;comment:'提交或执行错误'"`
	SubmittedAt int64                           `gorm:"column:submitted_at;type:bigint;not null;default:0;comment:'提交成功时间'"`
	CompletedAt int64                           `gorm:"column:completed_at;type:bigint;not null;default:0;comment:'执行完成时间'"`
	CTime       int64                           `gorm:"column:ctime;type:bigint;comment:'创建时间'"`
	UTime       int64                           `gorm:"column:utime;type:bigint;comment:'更新时间'"`
}

// TableName 返回执行尝试表名。
func (TaskAttempt) TableName() string { return "automation_task_attempts" }

// TaskAttemptDAO 定义执行尝试的持久化和状态迁移能力。
type TaskAttemptDAO interface {
	// Begin 在任务行锁内创建下一次尝试，或返回尚未完成提交的当前尝试。
	Begin(ctx context.Context, taskID, runnerID int64, input domain.TaskArgs) (TaskAttempt, error)
	// BindExecution 绑定 etask 执行 ID，并将任务和尝试置为运行中。
	BindExecution(ctx context.Context, attemptID, executionID int64) error
	// RecordSubmissionError 记录结果不确定的提交错误，等待使用相同请求标识重试。
	RecordSubmissionError(ctx context.Context, attemptID int64, reason string) error
	// RejectSubmission 记录 etask 明确拒绝的提交并阻塞当前任务。
	RejectSubmission(ctx context.Context, attemptID int64, reason string) error
	// Complete 根据请求标识幂等完成执行尝试。
	Complete(ctx context.Context, requestID, status, output, reason string) (TaskAttempt, error)
	// FindByID 根据主键查询执行尝试。
	FindByID(ctx context.Context, id int64) (TaskAttempt, error)
	// ListByTaskID 查询任务的全部执行尝试。
	ListByTaskID(ctx context.Context, taskID int64) ([]TaskAttempt, error)
}

type gormTaskAttemptDAO struct{ db *gorm.DB }

// NewTaskAttemptDAO 创建执行尝试 DAO。
func NewTaskAttemptDAO(db *gorm.DB) TaskAttemptDAO { return &gormTaskAttemptDAO{db: db} }

func (g *gormTaskAttemptDAO) Begin(ctx context.Context, taskID, runnerID int64,
	input domain.TaskArgs) (TaskAttempt, error) {
	var attempt TaskAttempt
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var task Task
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", taskID).First(&task).Error; err != nil {
			return err
		}
		if task.CurrentAttemptID > 0 &&
			(task.Status == domain.TaskStatusSubmitting.ToUint8() || task.Status == domain.TaskStatusRunning.ToUint8()) {
			return tx.Where("id = ?", task.CurrentAttemptID).First(&attempt).Error
		}
		var last TaskAttempt
		attemptNo := 1
		if err := tx.Where("task_id = ?", taskID).Order("attempt_no DESC").First(&last).Error; err == nil {
			attemptNo = last.AttemptNo + 1
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		now := time.Now().UnixMilli()
		attempt = TaskAttempt{
			TaskID: taskID, AttemptNo: attemptNo,
			RequestID: fmt.Sprintf("eflow:%d:%d", taskID, attemptNo),
			RunnerID:  runnerID, Status: string(domain.AttemptStatusSubmitting),
			Input: sqlx.JsonField[domain.TaskArgs]{Val: input, Valid: true}, CTime: now, UTime: now,
		}
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).Where("id = ?", taskID).Updates(map[string]any{
			"status": domain.TaskStatusSubmitting.ToUint8(), "phase": domain.TaskPhaseSubmitting,
			"current_attempt_id": attempt.ID, "last_error": "", "utime": now,
		}).Error
	})
	return attempt, err
}

func (g *gormTaskAttemptDAO) BindExecution(ctx context.Context, attemptID, executionID int64) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt TaskAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", attemptID).First(&attempt).Error; err != nil {
			return err
		}
		if attempt.ExecutionID.Valid && attempt.ExecutionID.Int64 != executionID {
			return fmt.Errorf("执行尝试 %d 已绑定其他 etask execution: %d", attemptID, attempt.ExecutionID.Int64)
		}
		updates := map[string]any{"execution_id": executionID, "error_message": "", "utime": now}
		if attempt.SubmittedAt == 0 {
			updates["submitted_at"] = now
		}
		if !domain.AttemptStatus(attempt.Status).IsTerminal() {
			updates["status"] = domain.AttemptStatusRunning
		}
		if err := tx.Model(&TaskAttempt{}).Where("id = ?", attemptID).Updates(updates).Error; err != nil {
			return err
		}
		// 极快执行可能先通过完成事件进入终态，此时这里只补齐 execution ID。
		if domain.AttemptStatus(attempt.Status).IsTerminal() {
			return nil
		}
		return tx.Model(&Task{}).
			Where("id = ? AND current_attempt_id = ?", attempt.TaskID, attempt.ID).Updates(map[string]any{
			"status": domain.TaskStatusRunning.ToUint8(), "phase": domain.TaskPhaseRunning,
			"last_error": "", "utime": now,
		}).Error
	})
}

func (g *gormTaskAttemptDAO) RecordSubmissionError(ctx context.Context, attemptID int64,
	reason string) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt TaskAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", attemptID).First(&attempt).Error; err != nil {
			return err
		}
		if domain.AttemptStatus(attempt.Status).IsTerminal() {
			return nil
		}
		if err := tx.Model(&TaskAttempt{}).Where("id = ?", attemptID).Updates(map[string]any{
			"error_message": reason, "utime": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).
			Where("id = ? AND current_attempt_id = ?", attempt.TaskID, attempt.ID).Updates(map[string]any{
			"last_error": reason, "utime": now,
		}).Error
	})
}

func (g *gormTaskAttemptDAO) RejectSubmission(ctx context.Context, attemptID int64, reason string) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var attempt TaskAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", attemptID).First(&attempt).Error; err != nil {
			return err
		}
		if domain.AttemptStatus(attempt.Status).IsTerminal() {
			return nil
		}
		if err := tx.Model(&TaskAttempt{}).Where("id = ?", attemptID).Updates(map[string]any{
			"status": domain.AttemptStatusFailed, "error_message": reason,
			"completed_at": now, "utime": now,
		}).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).
			Where("id = ? AND current_attempt_id = ?", attempt.TaskID, attempt.ID).Updates(map[string]any{
			"status": domain.TaskStatusBlocked.ToUint8(), "phase": domain.TaskPhaseBlocked,
			"last_error": reason, "utime": now,
		}).Error
	})
}

func (g *gormTaskAttemptDAO) Complete(ctx context.Context, requestID, status, output,
	reason string) (TaskAttempt, error) {
	var attempt TaskAttempt
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("request_id = ?", requestID).First(&attempt).Error; err != nil {
			return err
		}
		if domain.AttemptStatus(attempt.Status).IsTerminal() {
			return nil
		}
		now := time.Now().UnixMilli()
		attemptStatus := domain.AttemptStatusFailed
		taskStatus := domain.TaskStatusFailed
		phase := domain.TaskPhaseFailed
		if status == string(domain.AttemptStatusSuccess) {
			attemptStatus = domain.AttemptStatusSuccess
			taskStatus = domain.TaskStatusSuccess
			phase = domain.TaskPhaseSucceeded
		}
		if err := tx.Model(&TaskAttempt{}).Where("id = ?", attempt.ID).Updates(map[string]any{
			"status": attemptStatus, "output": output, "error_message": reason,
			"completed_at": now, "utime": now,
		}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Task{}).
			Where("id = ? AND current_attempt_id = ?", attempt.TaskID, attempt.ID).Updates(map[string]any{
			"status": taskStatus.ToUint8(), "phase": phase, "last_error": reason, "utime": now,
		}).Error; err != nil {
			return err
		}
		attempt.Status = string(attemptStatus)
		attempt.Output = output
		attempt.Error = reason
		attempt.CompletedAt = now
		return nil
	})
	return attempt, err
}

func (g *gormTaskAttemptDAO) FindByID(ctx context.Context, id int64) (TaskAttempt, error) {
	var attempt TaskAttempt
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&attempt).Error
	return attempt, err
}

func (g *gormTaskAttemptDAO) ListByTaskID(ctx context.Context, taskID int64) ([]TaskAttempt, error) {
	var attempts []TaskAttempt
	err := g.db.WithContext(ctx).Where("task_id = ?", taskID).Order("attempt_no DESC").Find(&attempts).Error
	return attempts, err
}
