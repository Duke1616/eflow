package withdrawal

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestRevokeCoordinatesStateTransitions(t *testing.T) {
	testCases := []struct {
		name         string
		prepareErr   error
		buildErr     error
		revokeErr    error
		applyErr     error
		finalizeErr  error
		wantErr      string
		wantBuild    int
		wantRevoke   int
		wantApply    int
		wantFinalize int
		wantRollback int
	}{
		{name: "无补偿任务直接撤回", wantBuild: 1, wantRevoke: 1, wantApply: 1, wantFinalize: 1},
		{name: "运行中任务拒绝撤回", prepareErr: repository.ErrAutomationRunning,
			wantErr: "自动化任务正在执行"},
		{name: "补偿计划失败恢复工单", buildErr: errors.New("snapshot unavailable"),
			wantErr: "生成撤回补偿计划失败", wantBuild: 1, wantRollback: 1},
		{name: "流程引擎撤回失败恢复工单", revokeErr: errors.New("engine unavailable"),
			wantErr: "engine unavailable", wantBuild: 1, wantRevoke: 1, wantRollback: 1},
		{name: "引擎撤回后激活失败保持撤回中", applyErr: errors.New("db unavailable"),
			wantErr: "启动撤回补偿失败", wantBuild: 1, wantRevoke: 1, wantApply: 1},
		{name: "补偿完成检查失败保持撤回中", finalizeErr: errors.New("db unavailable"),
			wantErr: "完成工单撤回状态失败", wantBuild: 1, wantRevoke: 1,
			wantApply: 1, wantFinalize: 1},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := &withdrawalRepositoryStub{
				prepareErr: testCase.prepareErr, finalizeErr: testCase.finalizeErr,
			}
			planner := &compensationPlannerStub{buildErr: testCase.buildErr, applyErr: testCase.applyErr}
			revoker := &processRevokerStub{err: testCase.revokeErr}
			svc := newService(repo, planner, revoker)

			err := svc.Revoke(context.Background(), domain.EngineTypeEasyFlow, domain.NewID(101), true, "starter", "不再需要该申请")

			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
			require.Equal(t, testCase.wantBuild, planner.buildCalls)
			require.Equal(t, testCase.wantRevoke, revoker.calls)
			require.Equal(t, testCase.wantApply, planner.applyCalls)
			require.Equal(t, testCase.wantFinalize, repo.finalizeCalls)
			require.Equal(t, testCase.wantRollback, repo.rollbackCalls)
			if testCase.prepareErr == nil {
				require.Equal(t, "不再需要该申请", repo.reason)
			} else {
				require.Empty(t, repo.reason)
			}
		})
	}
}

func TestRevokeValidatesIdentity(t *testing.T) {
	svc := newService(&withdrawalRepositoryStub{}, &compensationPlannerStub{}, &processRevokerStub{})

	require.Error(t, svc.Revoke(context.Background(), domain.EngineTypeEasyFlow, "", false, "starter", "不再需要"))
	require.Error(t, svc.Revoke(context.Background(), domain.EngineTypeEasyFlow, domain.NewID(1), false, "", "不再需要"))
	require.ErrorIs(t, svc.Revoke(context.Background(), domain.EngineTypeEasyFlow, domain.NewID(1), false, "starter", " "), ErrInvalidRevokeReason)
}

// TestRevokePreservesFlowxInstanceID 撤回全链路必须原样透传 flowx 雪花实例标识：
// 一旦中途退化成 int 或 0，补偿计划、任务激活与状态收尾都会寻址到空实例。
func TestRevokePreservesFlowxInstanceID(t *testing.T) {
	instanceID := domain.NewID("7395001234567890123")
	repo := &withdrawalRepositoryStub{}
	planner := &compensationPlannerStub{}
	revoker := &processRevokerStub{}
	svc := newService(repo, planner, revoker)

	err := svc.Revoke(context.Background(), domain.EngineTypeFlowx, instanceID, false, "starter", "不再需要该申请")

	require.NoError(t, err)
	require.Equal(t, instanceID, planner.builtID)
	require.Equal(t, instanceID, planner.appliedID)
	require.Equal(t, instanceID, repo.preparedID)
	require.Equal(t, instanceID, repo.finalizedID)
	require.Equal(t, instanceID, revoker.instanceID)
	require.Zero(t, repo.rollbackCalls)
}

type withdrawalRepositoryStub struct {
	repository.WithdrawalRepository
	prepareErr    error
	finalizeErr   error
	finalizeCalls int
	rollbackCalls int
	reason        string
	preparedID    domain.ID
	finalizedID   domain.ID
}

func (s *withdrawalRepositoryStub) Prepare(_ context.Context, instanceID domain.ID, reason string) error {
	s.preparedID = instanceID
	if s.prepareErr == nil {
		s.reason = reason
	}
	return s.prepareErr
}

func (s *withdrawalRepositoryStub) TryFinalize(_ context.Context, instanceID domain.ID) (bool, error) {
	s.finalizeCalls++
	s.finalizedID = instanceID
	return s.finalizeErr == nil, s.finalizeErr
}

func (s *withdrawalRepositoryStub) Rollback(context.Context, domain.ID) error {
	s.rollbackCalls++
	return nil
}

type processRevokerStub struct {
	err        error
	calls      int
	instanceID domain.ID
}

type compensationPlannerStub struct {
	buildErr   error
	applyErr   error
	buildCalls int
	applyCalls int
	builtID    domain.ID
	appliedID  domain.ID
}

func (s *compensationPlannerStub) Build(_ context.Context, instanceID domain.ID) (CompensationPlan, error) {
	s.buildCalls++
	s.builtID = instanceID
	return CompensationPlan{}, s.buildErr
}

func (s *compensationPlannerStub) Apply(_ context.Context, instanceID domain.ID, _ CompensationPlan) error {
	s.applyCalls++
	s.appliedID = instanceID
	return s.applyErr
}

func (s *processRevokerStub) Revoke(_ context.Context, _ domain.EngineType, instanceID domain.ID, _ bool, _ string) error {
	s.calls++
	s.instanceID = instanceID
	return s.err
}
