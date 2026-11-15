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
	const flowxKey = "7395001234567890123"

	testCases := []struct {
		name           string
		candidates     []domain.WithdrawalCandidate
		wantBuilt      []domain.ID
		wantApplied    []domain.ID
		wantFinalized  []domain.ID
		wantRolledBack []domain.ID
		wantTenantIDs  []int64
	}{
		{
			name: "easyflow 已撤销走补偿重建、仍活动走状态回滚",
			candidates: []domain.WithdrawalCandidate{
				{TicketID: 1, TenantID: 11, ProcessInstanceID: 101, EngineRevoked: true},
				{TicketID: 2, TenantID: 12, ProcessInstanceID: 102, EngineActive: true},
				{TicketID: 3, TenantID: 13, ProcessInstanceID: 103},
			},
			wantBuilt:      []domain.ID{domain.NewID(101)},
			wantApplied:    []domain.ID{domain.NewID(101)},
			wantFinalized:  []domain.ID{domain.NewID(101)},
			wantRolledBack: []domain.ID{domain.NewID(102)},
			wantTenantIDs:  []int64{11, 12},
		},
		{
			// flowx 落库 process_instance_id 恒为 0，只认 process_instance_key；
			// 取错字段会让补偿与回滚全部寻址到空实例，工单永久卡在撤回中。
			name: "flowx 雪花实例标识按 Key 寻址",
			candidates: []domain.WithdrawalCandidate{
				{TicketID: 4, TenantID: 21, ProcessInstanceKey: flowxKey, EngineRevoked: true},
				{TicketID: 5, TenantID: 22, ProcessInstanceKey: "7395001234567890456", EngineActive: true},
			},
			wantBuilt:      []domain.ID{domain.NewID(flowxKey)},
			wantApplied:    []domain.ID{domain.NewID(flowxKey)},
			wantFinalized:  []domain.ID{domain.NewID(flowxKey)},
			wantRolledBack: []domain.ID{domain.NewID("7395001234567890456")},
			wantTenantIDs:  []int64{21, 22},
		},
		{
			name: "两个引擎状态都缺失时跳过不寻址",
			candidates: []domain.WithdrawalCandidate{
				{TicketID: 6, TenantID: 31, ProcessInstanceKey: flowxKey},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &recoveryRepositoryStub{candidates: tc.candidates}
			planner := &recoveryJobPlannerStub{}
			job := NewRecoveryJob(repo, planner, 10, time.Minute, 5*time.Minute)

			err := job.run(context.Background())

			require.NoError(t, err)
			require.Equal(t, tc.wantBuilt, planner.built)
			require.Equal(t, tc.wantApplied, planner.applied)
			require.Equal(t, tc.wantFinalized, repo.finalized)
			require.Equal(t, tc.wantRolledBack, repo.rolledBack)
			require.Equal(t, tc.wantTenantIDs, repo.tenantIDs)
		})
	}
}

type recoveryRepositoryStub struct {
	repository.WithdrawalRepository
	candidates []domain.WithdrawalCandidate
	finalized  []domain.ID
	rolledBack []domain.ID
	tenantIDs  []int64
}

func (s *recoveryRepositoryStub) ListStale(_ context.Context, _ int64,
	afterID, _ int64) ([]domain.WithdrawalCandidate, error) {
	if afterID > 0 {
		return nil, nil
	}
	return s.candidates, nil
}

func (s *recoveryRepositoryStub) TryFinalize(ctx context.Context, instanceID domain.ID) (bool, error) {
	s.finalized = append(s.finalized, instanceID)
	s.tenantIDs = append(s.tenantIDs, ctxutil.GetTenantID(ctx).Int64())
	return true, nil
}

func (s *recoveryRepositoryStub) Rollback(ctx context.Context, instanceID domain.ID) error {
	s.rolledBack = append(s.rolledBack, instanceID)
	s.tenantIDs = append(s.tenantIDs, ctxutil.GetTenantID(ctx).Int64())
	return nil
}

type recoveryJobPlannerStub struct {
	built   []domain.ID
	applied []domain.ID
}

func (s *recoveryJobPlannerStub) Build(_ context.Context, instanceID domain.ID) (CompensationPlan, error) {
	s.built = append(s.built, instanceID)
	return CompensationPlan{}, nil
}

func (s *recoveryJobPlannerStub) Apply(_ context.Context, instanceID domain.ID,
	_ CompensationPlan) error {
	s.applied = append(s.applied, instanceID)
	return nil
}
