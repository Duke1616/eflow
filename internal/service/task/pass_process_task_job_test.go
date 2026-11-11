package task

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestPassProcessTaskJobDoesNotSkipShrinkingPages(t *testing.T) {
	tasks := &passTaskServiceStub{remaining: []domain.Task{
		{ID: 1, TenantID: 11, NodeID: "node-1", InstanceID: domain.NewID(101)},
		{ID: 2, TenantID: 12, NodeID: "node-2", InstanceID: domain.NewID(102)},
		{ID: 3, TenantID: 13, NodeID: "node-3", InstanceID: domain.NewID(103)},
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
		{ID: 1, TenantID: 11, NodeID: "node-1", InstanceID: domain.NewID(101)},
		{ID: 2, TenantID: 12, NodeID: "node-2", InstanceID: domain.NewID(102)},
		{ID: 3, TenantID: 13, NodeID: "node-3", InstanceID: domain.NewID(103)},
	}}
	engineSvc := &passEngineStub{errors: map[int]error{101: errors.New("engine unavailable")}}
	job := NewPassProcessTaskJob(tasks, engineSvc, 2, time.Minute)

	err := job.run(context.Background())

	require.ErrorContains(t, err, "task_id=1")
	require.Equal(t, []int64{2, 3}, tasks.marked)
	require.Equal(t, 2, tasks.listCalls)
}

func TestPassProcessTaskJobSkipsArchivedProcess(t *testing.T) {
	advance := false
	tasks := &passTaskServiceStub{
		remaining: []domain.Task{{ID: 1, TenantID: 11, TicketID: 21}},
		advance:   &advance,
	}
	engineSvc := &passEngineStub{}
	job := NewPassProcessTaskJob(tasks, engineSvc, 10, time.Minute)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Equal(t, []int64{1}, tasks.marked)
	require.Empty(t, engineSvc.tenantIDs)
}

func TestPassProcessTaskJobDefersWithdrawingProcess(t *testing.T) {
	tasks := &passTaskServiceStub{
		remaining:    []domain.Task{{ID: 1, TenantID: 11, TicketID: 21}},
		advanceError: ErrWithdrawalInProgress,
	}
	engineSvc := &passEngineStub{}
	job := NewPassProcessTaskJob(tasks, engineSvc, 10, time.Minute)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Empty(t, tasks.marked)
	require.Empty(t, engineSvc.tenantIDs)
}

func TestPassProcessTaskJobMarksAutoPassedWhenEngineTaskNotFound(t *testing.T) {
	// 流程引擎中节点任务不存在（已被跳过或完结），应自动标记已推进，避免死循环报错
	tasks := &passTaskServiceStub{remaining: []domain.Task{
		{ID: 1, TenantID: 11, NodeID: "node-1", InstanceID: domain.NewID(101)},
	}}
	engineSvc := &passEngineStub{errors: map[int]error{
		// 采用引擎适配层真实的包装形态，验证 errors.Is 能穿透 %w 上下文识别哨兵
		101: fmt.Errorf("%w: 未找到对应的自动化节点待办任务 [node_id=node-1]", engine.ErrTaskNotFound),
	}}
	job := NewPassProcessTaskJob(tasks, engineSvc, 10, time.Minute)

	err := job.run(context.Background())

	require.NoError(t, err)
	require.Equal(t, []int64{1}, tasks.marked)
}

type passTaskServiceStub struct {
	Service
	remaining    []domain.Task
	marked       []int64
	listCalls    int
	advance      *bool
	advanceError error
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

func (s *passTaskServiceStub) ShouldAdvanceProcessTask(context.Context, int64) (bool, domain.EngineType, error) {
	if s.advanceError != nil {
		return false, "", s.advanceError
	}
	if s.advance == nil {
		return true, domain.EngineTypeEasyFlow, nil
	}
	return *s.advance, domain.EngineTypeEasyFlow, nil
}

type passEngineStub struct {
	engine.IEngine
	tenantIDs []int64
	errors    map[int]error
}

func (s *passEngineStub) GetAutomationTask(ctx context.Context, _ domain.EngineType, instanceID domain.ID, _ string) (domain.TaskRecord, error) {
	s.tenantIDs = append(s.tenantIDs, ctxutil.GetTenantID(ctx).Int64())
	procInstID := instanceID.Int()
	if err := s.errors[procInstID]; err != nil {
		return domain.TaskRecord{}, err
	}
	return domain.TaskRecord{TaskID: domain.NewID(procInstID)}, nil
}

func (s *passEngineStub) Pass(context.Context, domain.TaskPassReq) error { return nil }
