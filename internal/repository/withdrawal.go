package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
)

var (
	ErrTicketNotProcessing = errors.New("工单不处于可撤回状态")
	ErrAutomationRunning   = errors.New("存在正在执行且无法取消的自动化任务")
)

// WithdrawalRepository 管理工单撤回与自动化任务状态迁移。
type WithdrawalRepository interface {
	// Prepare 原子进入撤回中状态、记录撤单原因，并阻止新的普通自动化执行。
	Prepare(ctx context.Context, processInstanceID int, reason string) error
	// ActivateCompensations 激活指定补偿节点并取消其余尚未执行的自动化任务。
	ActivateCompensations(ctx context.Context, processInstanceID int, nodeIDs []string) error
	// TryFinalize 在补偿全部成功后将工单从撤回中推进为已撤回。
	TryFinalize(ctx context.Context, processInstanceID int) (bool, error)
	// Rollback 在流程引擎尚未撤回时恢复工单状态并清除撤单原因。
	Rollback(ctx context.Context, processInstanceID int) error
	// ListStale 分页查询长时间停留在撤回中、需要后台恢复的工单。
	ListStale(ctx context.Context, before int64, afterID, limit int64) ([]domain.WithdrawalCandidate, error)
}

type withdrawalRepository struct{ dao dao.WithdrawalDAO }

func NewWithdrawalRepository(withdrawalDAO dao.WithdrawalDAO) WithdrawalRepository {
	return &withdrawalRepository{dao: withdrawalDAO}
}

func (r *withdrawalRepository) Prepare(ctx context.Context, processInstanceID int, reason string) error {
	return mapWithdrawalError(r.dao.Prepare(ctx, processInstanceID, reason))
}

func (r *withdrawalRepository) ActivateCompensations(ctx context.Context,
	processInstanceID int, nodeIDs []string) error {
	return mapWithdrawalError(r.dao.ActivateCompensations(ctx, processInstanceID, nodeIDs))
}

func (r *withdrawalRepository) TryFinalize(ctx context.Context, processInstanceID int) (bool, error) {
	completed, err := r.dao.TryFinalize(ctx, processInstanceID)
	return completed, mapWithdrawalError(err)
}

func (r *withdrawalRepository) Rollback(ctx context.Context, processInstanceID int) error {
	return r.dao.Rollback(ctx, processInstanceID)
}

func (r *withdrawalRepository) ListStale(ctx context.Context, before int64,
	afterID, limit int64) ([]domain.WithdrawalCandidate, error) {
	candidates, err := r.dao.ListStale(ctx, before, afterID, limit)
	result := make([]domain.WithdrawalCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		result = append(result, domain.WithdrawalCandidate{
			TicketID: candidate.TicketID, TenantID: candidate.TenantID,
			ProcessInstanceID: candidate.ProcessInstanceID,
			EngineActive:      candidate.EngineActive, EngineRevoked: candidate.EngineRevoked,
		})
	}
	return result, err
}

func mapWithdrawalError(err error) error {
	switch {
	case errors.Is(err, dao.ErrTicketNotProcessing):
		return errors.Join(ErrTicketNotProcessing, err)
	case errors.Is(err, dao.ErrAutomationRunning):
		return errors.Join(ErrAutomationRunning, err)
	default:
		return err
	}
}
