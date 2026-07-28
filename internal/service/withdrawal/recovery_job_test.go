package withdrawal

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestRecoveryJobReconcilesEngineState(t *testing.T) {
	repo := &recoveryRepositoryStub{candidates: []domain.WithdrawalCandidate{
		{TicketID: 1, TenantID: 11, ProcessInstanceID: 101, EngineRevoked: true},
		{TicketID: 2, TenantID: 12, ProcessInstanceID: 102, EngineActive: true},
		{TicketID: 3, TenantID: 13, ProcessInstanceID: 103},
	}}
	planner := &recoveryJobPlannerStub{}
	job := NewRecoveryJob(repo, planner, 10, time.Minute, 5*time.Minute)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Equal(t, []int{101}, planner.built)
	require.Equal(t, []int{101}, planner.applied)
	require.Equal(t, []int{101}, repo.finalized)
	require.Equal(t, []int{102}, repo.rolledBack)
	require.Equal(t, []int64{11, 12}, repo.tenantIDs)
}

type recoveryRepositoryStub struct {
	repository.WithdrawalRepository
	candidates []domain.WithdrawalCandidate
	finalized  []int
	rolledBack []int
	tenantIDs  []int64
}

func (s *recoveryRepositoryStub) ListStale(_ context.Context, _ int64,
	afterID, _ int64) ([]domain.WithdrawalCandidate, error) {
	if afterID > 0 {
		return nil, nil
	}
	return s.candidates, nil
}

func (s *recoveryRepositoryStub) TryFinalize(ctx context.Context, instanceID int) (bool, error) {
	s.finalized = append(s.finalized, instanceID)
	s.tenantIDs = append(s.tenantIDs, ctxutil.GetTenantID(ctx).Int64())
	return true, nil
}

func (s *recoveryRepositoryStub) Rollback(ctx context.Context, instanceID int) error {
	s.rolledBack = append(s.rolledBack, instanceID)
	s.tenantIDs = append(s.tenantIDs, ctxutil.GetTenantID(ctx).Int64())
	return nil
}

type recoveryJobPlannerStub struct {
	built   []int
	applied []int
}

func (s *recoveryJobPlannerStub) Build(_ context.Context, processInstanceID int) (CompensationPlan, error) {
	s.built = append(s.built, processInstanceID)
	return CompensationPlan{}, nil
}

func (s *recoveryJobPlannerStub) Apply(_ context.Context, processInstanceID int,
	_ CompensationPlan) error {
	s.applied = append(s.applied, processInstanceID)
	return nil
}
