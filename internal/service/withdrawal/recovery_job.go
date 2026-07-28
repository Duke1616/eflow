package withdrawal

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/repository"
	"github.com/gotomicro/ego/core/elog"
)

// RecoveryJob 修复因进程退出而长期停留在 WITHDRAWING 的撤回操作。
type RecoveryJob struct {
	repo       repository.WithdrawalRepository
	planner    CompensationPlanner
	limit      int64
	interval   time.Duration
	staleAfter time.Duration
	logger     *elog.Component
}

func NewRecoveryJob(repo repository.WithdrawalRepository, planner CompensationPlanner, limit int64,
	interval, staleAfter time.Duration) *RecoveryJob {
	return &RecoveryJob{
		repo: repo, planner: planner, limit: limit, interval: interval, staleAfter: staleAfter,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("withdrawal.recovery")),
	}
}

func (j *RecoveryJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if err := j.run(ctx); err != nil {
			j.logger.Error("恢复撤回中工单失败", elog.FieldErr(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *RecoveryJob) run(ctx context.Context) error {
	before := time.Now().Add(-j.staleAfter).UnixMilli()
	var afterID int64
	var runErr error
	for {
		candidates, err := j.repo.ListStale(ctx, before, afterID, j.limit)
		if err != nil {
			return fmt.Errorf("查询撤回中工单失败: %w", err)
		}
		for _, candidate := range candidates {
			afterID = candidate.TicketID
			candidateCtx := withdrawalTenantContext(ctx, candidate.TenantID)
			switch {
			case candidate.EngineRevoked:
				err = j.resume(candidateCtx, candidate.ProcessInstanceID)
			case candidate.EngineActive:
				err = j.repo.Rollback(candidateCtx, candidate.ProcessInstanceID)
			default:
				j.logger.Warn("撤回中工单缺少活动和撤销历史流程实例",
					elog.Int64("ticketID", candidate.TicketID),
					elog.Int("processInstanceID", candidate.ProcessInstanceID))
				continue
			}
			if err != nil {
				runErr = errors.Join(runErr,
					fmt.Errorf("恢复撤回中工单失败: ticket_id=%d: %w", candidate.TicketID, err))
			}
		}
		if int64(len(candidates)) < j.limit {
			return runErr
		}
	}
}

func (j *RecoveryJob) resume(ctx context.Context, processInstanceID int) error {
	plan, err := j.planner.Build(ctx, processInstanceID)
	if err != nil {
		return fmt.Errorf("重建撤回补偿计划失败: %w", err)
	}
	if err = j.planner.Apply(ctx, processInstanceID, plan); err != nil {
		return err
	}
	_, err = j.repo.TryFinalize(ctx, processInstanceID)
	return err
}
