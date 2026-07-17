package task

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestPassProcessTaskJobDoesNotSkipShrinkingPages(t *testing.T) {
	tasks := &passTaskServiceStub{remaining: []domain.Task{
		{ID: 1, TenantID: 11, NodeID: "node-1", ProcessInstanceID: 101},
		{ID: 2, TenantID: 12, NodeID: "node-2", ProcessInstanceID: 102},
		{ID: 3, TenantID: 13, NodeID: "node-3", ProcessInstanceID: 103},
	}}
	engineSvc := &passEngineStub{}
	job := NewPassProcessTaskJob(tasks, engineSvc, 2, time.Minute)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Equal(t, []int64{1, 2, 3}, tasks.marked)
	require.Equal(t, 2, tasks.listCalls)
	require.Equal(t, []int64{11, 12, 13}, engineSvc.tenantIDs)
}

func TestPassProcessTaskJobContinuesAfterTaskFailure(t *testing.T) {
	tasks := &passTaskServiceStub{remaining: []domain.Task{
		{ID: 1, TenantID: 11, NodeID: "node-1", ProcessInstanceID: 101},
		{ID: 2, TenantID: 12, NodeID: "node-2", ProcessInstanceID: 102},
		{ID: 3, TenantID: 13, NodeID: "node-3", ProcessInstanceID: 103},
	}}
	engineSvc := &passEngineStub{errors: map[int]error{101: errors.New("engine unavailable")}}
	job := NewPassProcessTaskJob(tasks, engineSvc, 2, time.Minute)

	err := job.run(context.Background())

	require.ErrorContains(t, err, "task_id=1")
	require.Equal(t, []int64{2, 3}, tasks.marked)
	require.Equal(t, 2, tasks.listCalls)
}

type passTaskServiceStub struct {
	Service
	remaining []domain.Task
	marked    []int64
	listCalls int
}

func (s *passTaskServiceStub) ListUnadvancedSuccessTasks(_ context.Context, limit,
	afterID int64) ([]domain.Task, error) {
	s.listCalls++
	result := make([]domain.Task, 0, limit)
	for _, task := range s.remaining {
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

func (s *passTaskServiceStub) MarkTaskAsAutoPassed(_ context.Context, id int64) error {
	s.marked = append(s.marked, id)
	for index := range s.remaining {
		if s.remaining[index].ID == id {
			s.remaining = append(s.remaining[:index], s.remaining[index+1:]...)
			break
		}
	}
	return nil
}

type passEngineStub struct {
	engine.Service
	tenantIDs []int64
	errors    map[int]error
}

func (s *passEngineStub) GetAutomationTask(ctx context.Context, _ string, processInstanceID int) (model.Task, error) {
	s.tenantIDs = append(s.tenantIDs, ctxutil.GetTenantID(ctx).Int64())
	if err := s.errors[processInstanceID]; err != nil {
		return model.Task{}, err
	}
	return model.Task{TaskID: processInstanceID}, nil
}

func (s *passEngineStub) Pass(context.Context, int, string) error { return nil }
