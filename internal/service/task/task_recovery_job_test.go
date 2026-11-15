package task

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTaskRecoveryJobScansAllRecoverableStates(t *testing.T) {
	old := time.Now().Add(-time.Hour).UnixMilli()
	svc := &recoveryTaskServiceStub{tasks: map[domain.TaskStatus][]domain.Task{
		domain.TaskStatusSubmitting: {{ID: 1, UTime: old}, {ID: 2, UTime: old}},
		domain.TaskStatusFailed:     {{ID: 3, UTime: old}},
		domain.TaskStatusRunning:    {{ID: 4, UTime: old}, {ID: 5, UTime: old}},
	}}
	job := NewTaskRecoveryJob(svc, 1, time.Minute, time.Minute, 30*time.Second)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, svc.started)
	require.Equal(t, []int64{3}, svc.retried)
	require.Equal(t, []int64{4, 5}, svc.reconciled)
}

type recoveryTaskServiceStub struct {
	Service
	tasks      map[domain.TaskStatus][]domain.Task
	hangOn     map[int64]bool
	deadlines  []time.Duration
	started    []int64
	retried    []int64
	reconciled []int64
}

func (s *recoveryTaskServiceStub) record(ctx context.Context, id int64) error {
	remaining := time.Duration(0)
	if deadline, ok := ctx.Deadline(); ok {
		remaining = time.Until(deadline)
	}
	s.deadlines = append(s.deadlines, remaining)
	if s.hangOn[id] {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (s *recoveryTaskServiceStub) ListTasksByStatusAfterID(_ context.Context,
	status domain.TaskStatus, afterID, limit int64) ([]domain.Task, error) {
	result := make([]domain.Task, 0, limit)
	for _, task := range s.tasks[status] {
		if task.ID <= afterID {
			continue
		}
		result = append(result, task)
		if int64(len(result)) == limit {
			break
		}
	}
	return result, nil
}

func (s *recoveryTaskServiceStub) StartTask(ctx context.Context, id int64) error {
	err := s.record(ctx, id)
	s.started = append(s.started, id)
	return err
}

func (s *recoveryTaskServiceStub) AutoRetryTask(ctx context.Context, id int64) error {
	err := s.record(ctx, id)
	s.retried = append(s.retried, id)
	return err
}

func (s *recoveryTaskServiceStub) ReconcileTask(ctx context.Context, id int64) error {
	err := s.record(ctx, id)
	s.reconciled = append(s.reconciled, id)
	return err
}

func TestTaskRecoveryJobPerCallTimeout(t *testing.T) {
	old := time.Now().Add(-time.Hour).UnixMilli()
	testCases := []struct {
		name        string
		status      domain.TaskStatus
		taskTimeout time.Duration
	}{
		{"提交中恢复", domain.TaskStatusSubmitting, 30 * time.Second},
		{"失败自动重试", domain.TaskStatusFailed, 30 * time.Second},
		{"运行中对账", domain.TaskStatusRunning, 30 * time.Second},
		{"自定义短超时", domain.TaskStatusSubmitting, 2 * time.Second},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &recoveryTaskServiceStub{tasks: map[domain.TaskStatus][]domain.Task{
				tc.status: {{ID: 1, UTime: old, TenantID: 7}},
			}}
			job := NewTaskRecoveryJob(svc, 10, time.Minute, time.Minute, tc.taskTimeout)

			require.NoError(t, job.run(context.Background()))

			require.Len(t, svc.deadlines, 1)
			require.Positive(t, svc.deadlines[0], "补偿调用必须带 deadline，否则 etask 挂起会停摆整个循环")
			require.LessOrEqual(t, svc.deadlines[0], tc.taskTimeout)
			require.Greater(t, svc.deadlines[0], tc.taskTimeout-time.Second)
		})
	}
}

func TestTaskRecoveryJobHungCallDoesNotStallScan(t *testing.T) {
	old := time.Now().Add(-time.Hour).UnixMilli()
	svc := &recoveryTaskServiceStub{
		tasks: map[domain.TaskStatus][]domain.Task{
			domain.TaskStatusSubmitting: {{ID: 1, UTime: old}, {ID: 2, UTime: old}},
			domain.TaskStatusFailed:     {{ID: 3, UTime: old}},
			domain.TaskStatusRunning:    {{ID: 4, UTime: old}},
		},
		hangOn: map[int64]bool{1: true},
	}
	job := NewTaskRecoveryJob(svc, 10, time.Minute, time.Minute, 50*time.Millisecond)

	start := time.Now()
	require.NoError(t, job.run(context.Background()))
	elapsed := time.Since(start)

	require.Equal(t, []int64{1, 2}, svc.started)
	require.Equal(t, []int64{3}, svc.retried)
	require.Equal(t, []int64{4}, svc.reconciled)
	require.Less(t, elapsed, 5*time.Second, "挂起调用必须被超时切断，后续任务与后续状态仍要补偿")
}
