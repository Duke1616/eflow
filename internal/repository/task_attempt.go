package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
)

// TaskAttemptRepository 定义自动化任务执行尝试仓储。
type TaskAttemptRepository interface {
	// Begin 创建或恢复当前待提交尝试。
	Begin(ctx context.Context, taskID, runnerID int64, input domain.TaskArgs) (domain.TaskAttempt, error)
	// BindExecution 绑定 etask 执行 ID。
	BindExecution(ctx context.Context, attemptID, executionID int64) error
	// RecordSubmissionError 记录结果不确定的提交错误，保留当前尝试用于幂等重投。
	RecordSubmissionError(ctx context.Context, attemptID int64, reason string) error
	// RejectSubmission 记录 etask 明确拒绝的提交，并阻塞当前任务。
	RejectSubmission(ctx context.Context, attemptID int64, reason string) error
	// Complete 根据请求标识完成执行尝试。
	Complete(ctx context.Context, requestID string, status domain.AttemptStatus,
		output, reason string) (domain.TaskAttempt, error)
	// FindByID 根据主键查询执行尝试。
	FindByID(ctx context.Context, id int64) (domain.TaskAttempt, error)
	// ListByTaskID 查询任务全部执行尝试。
	ListByTaskID(ctx context.Context, taskID int64) ([]domain.TaskAttempt, error)
}

type taskAttemptRepository struct{ dao dao.TaskAttemptDAO }

// NewTaskAttemptRepository 创建执行尝试仓储。
func NewTaskAttemptRepository(attemptDAO dao.TaskAttemptDAO) TaskAttemptRepository {
	return &taskAttemptRepository{dao: attemptDAO}
}

func (r *taskAttemptRepository) Begin(ctx context.Context, taskID, runnerID int64,
	input domain.TaskArgs) (domain.TaskAttempt, error) {
	attempt, err := r.dao.Begin(ctx, taskID, runnerID, input)
	return toAttemptDomain(attempt), err
}

func (r *taskAttemptRepository) BindExecution(ctx context.Context, attemptID, executionID int64) error {
	return r.dao.BindExecution(ctx, attemptID, executionID)
}

func (r *taskAttemptRepository) RecordSubmissionError(ctx context.Context, attemptID int64,
	reason string) error {
	return r.dao.RecordSubmissionError(ctx, attemptID, reason)
}

func (r *taskAttemptRepository) RejectSubmission(ctx context.Context, attemptID int64, reason string) error {
	return r.dao.RejectSubmission(ctx, attemptID, reason)
}

func (r *taskAttemptRepository) Complete(ctx context.Context, requestID string,
	status domain.AttemptStatus, output, reason string) (domain.TaskAttempt, error) {
	attempt, err := r.dao.Complete(ctx, requestID, string(status), output, reason)
	return toAttemptDomain(attempt), err
}

func (r *taskAttemptRepository) FindByID(ctx context.Context, id int64) (domain.TaskAttempt, error) {
	attempt, err := r.dao.FindByID(ctx, id)
	return toAttemptDomain(attempt), err
}

func (r *taskAttemptRepository) ListByTaskID(ctx context.Context, taskID int64) ([]domain.TaskAttempt, error) {
	attempts, err := r.dao.ListByTaskID(ctx, taskID)
	return slice.Map(attempts, func(_ int, attempt dao.TaskAttempt) domain.TaskAttempt {
		return toAttemptDomain(attempt)
	}), err
}

func toAttemptDomain(attempt dao.TaskAttempt) domain.TaskAttempt {
	return domain.TaskAttempt{
		ID: attempt.ID, TenantID: attempt.TenantID, TaskID: attempt.TaskID,
		AttemptNo: attempt.AttemptNo, RequestID: attempt.RequestID, RunnerID: attempt.RunnerID,
		ExecutionID: attempt.ExecutionID.Int64, Status: domain.AttemptStatus(attempt.Status),
		Input: attempt.Input.Val, Output: attempt.Output, Error: attempt.Error,
		SubmittedAt: attempt.SubmittedAt, CompletedAt: attempt.CompletedAt,
		CTime: attempt.CTime, UTime: attempt.UTime,
	}
}
