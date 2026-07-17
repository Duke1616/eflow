package task

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestStartTaskJobLimitsConcurrency(t *testing.T) {
	testCases := []struct {
		name        string
		concurrency int
	}{
		{name: "单执行槽串行启动", concurrency: 1},
		{name: "两个执行槽并行启动", concurrency: 2},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tasks := make([]domain.Task, 5)
			for index := range tasks {
				tasks[index].ID = int64(index + 1)
			}
			svc := &startTaskServiceStub{tasks: tasks}
			job := NewStartTaskJob(svc, 10, testCase.concurrency, time.Minute, time.Second)

			err := job.run(context.Background())

			require.NoError(t, err)
			require.Equal(t, int32(len(tasks)), svc.calls.Load())
			require.LessOrEqual(t, svc.maximum.Load(), int32(testCase.concurrency))
		})
	}
}

type startTaskServiceStub struct {
	Service
	tasks   []domain.Task
	current atomic.Int32
	maximum atomic.Int32
	calls   atomic.Int32
}

func (s *startTaskServiceStub) ListReadyTasks(context.Context, int64) ([]domain.Task, error) {
	return s.tasks, nil
}

func (s *startTaskServiceStub) StartTask(context.Context, int64) error {
	s.calls.Add(1)
	current := s.current.Add(1)
	for {
		previous := s.maximum.Load()
		if current <= previous || s.maximum.CompareAndSwap(previous, current) {
			break
		}
	}
	time.Sleep(10 * time.Millisecond)
	s.current.Add(-1)
	return nil
}
