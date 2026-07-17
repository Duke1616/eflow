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
	job := NewTaskRecoveryJob(svc, 1, time.Minute, time.Minute)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2}, svc.started)
	require.Equal(t, []int64{3}, svc.retried)
	require.Equal(t, []int64{4, 5}, svc.reconciled)
}

type recoveryTaskServiceStub struct {
	Service
	tasks      map[domain.TaskStatus][]domain.Task
	started    []int64
	retried    []int64
	reconciled []int64
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

func (s *recoveryTaskServiceStub) StartTask(_ context.Context, id int64) error {
	s.started = append(s.started, id)
	return nil
}

func (s *recoveryTaskServiceStub) AutoRetryTask(_ context.Context, id int64) error {
	s.retried = append(s.retried, id)
	return nil
}

func (s *recoveryTaskServiceStub) ReconcileTask(_ context.Context, id int64) error {
	s.reconciled = append(s.reconciled, id)
	return nil
}
