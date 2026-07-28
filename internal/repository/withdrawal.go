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
	Prepare(ctx context.Context, processInstanceID int) error
	ActivateCompensations(ctx context.Context, processInstanceID int, nodeIDs []string) error
	TryFinalize(ctx context.Context, processInstanceID int) (bool, error)
	Rollback(ctx context.Context, processInstanceID int) error
	ListStale(ctx context.Context, before int64, afterID, limit int64) ([]domain.WithdrawalCandidate, error)
}

type withdrawalRepository struct{ dao dao.WithdrawalDAO }

func NewWithdrawalRepository(withdrawalDAO dao.WithdrawalDAO) WithdrawalRepository {
	return &withdrawalRepository{dao: withdrawalDAO}
}

func (r *withdrawalRepository) Prepare(ctx context.Context, processInstanceID int) error {
	return mapWithdrawalError(r.dao.Prepare(ctx, processInstanceID))
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
