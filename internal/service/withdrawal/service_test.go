package withdrawal

import (
	"context"
	"errors"
	"testing"

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

			err := svc.Revoke(context.Background(), 101, true, "starter")

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
		})
	}
}

func TestRevokeValidatesIdentity(t *testing.T) {
	svc := newService(&withdrawalRepositoryStub{}, &compensationPlannerStub{}, &processRevokerStub{})

	require.Error(t, svc.Revoke(context.Background(), 0, false, "starter"))
	require.Error(t, svc.Revoke(context.Background(), 1, false, ""))
}

type withdrawalRepositoryStub struct {
	repository.WithdrawalRepository
	prepareErr    error
	finalizeErr   error
	finalizeCalls int
	rollbackCalls int
}

func (s *withdrawalRepositoryStub) Prepare(context.Context, int) error { return s.prepareErr }

func (s *withdrawalRepositoryStub) TryFinalize(context.Context, int) (bool, error) {
	s.finalizeCalls++
	return s.finalizeErr == nil, s.finalizeErr
}

func (s *withdrawalRepositoryStub) Rollback(context.Context, int) error {
	s.rollbackCalls++
	return nil
}

type processRevokerStub struct {
	err   error
	calls int
}

type compensationPlannerStub struct {
	buildErr   error
	applyErr   error
	buildCalls int
	applyCalls int
}

func (s *compensationPlannerStub) Build(context.Context, int) (CompensationPlan, error) {
	s.buildCalls++
	return CompensationPlan{}, s.buildErr
}

func (s *compensationPlannerStub) Apply(context.Context, int, CompensationPlan) error {
	s.applyCalls++
	return s.applyErr
}

func (s *processRevokerStub) Revoke(int, bool, string) error {
	s.calls++
	return s.err
}
