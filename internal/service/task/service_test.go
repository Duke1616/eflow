package task

import (
	"context"
	"errors"
	"testing"

	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestCreateTaskDoesNotResetExistingTask(t *testing.T) {
	existing := domain.Task{
		ID: 1, TenantID: 2, TicketID: 3, ProcessInstanceID: 4,
		NodeID: "automation", Status: domain.TaskStatusSuccess, Phase: domain.TaskPhaseSucceeded,
	}
	tasks := &taskRepositoryStub{findOrCreateTask: existing}
	svc := &taskService{
		tasks:   tasks,
		tickets: &ticketServiceStub{ticket: domain.Ticket{Id: 3, TenantID: 2}},
	}

	actual, err := svc.CreateTask(context.Background(), 3, 4, "automation", "自动化")

	require.NoError(t, err)
	require.Equal(t, existing, actual)
}

func TestStartTaskResumesCurrentAttempt(t *testing.T) {
	testCases := []struct {
		name        string
		task        domain.Task
		attempt     domain.TaskAttempt
		dispatchID  int64
		dispatchErr error
		before      func(*taskRepositoryStub, *attemptRepositoryStub, *dispatcherStub)
		after       func(*testing.T, *attemptRepositoryStub, *dispatcherStub)
		wantErr     string
	}{
		{
			name: "提交中任务使用原 request ID 恢复", task: activeTask(domain.TaskStatusSubmitting),
			attempt: currentAttempt(0), dispatchID: 9001,
			after: func(t *testing.T, attempts *attemptRepositoryStub, dispatcher *dispatcherStub) {
				require.Equal(t, int64(9001), attempts.boundExecutionID)
				require.Equal(t, "eflow:1:1", dispatcher.received.RequestID)
				require.Equal(t, int64(2), dispatcher.tenantID)
			},
		},
		{
			name: "运行中任务不重复提交", task: activeTask(domain.TaskStatusRunning),
			attempt: currentAttempt(9001),
			after: func(t *testing.T, attempts *attemptRepositoryStub, dispatcher *dispatcherStub) {
				require.Zero(t, dispatcher.calls)
				require.Zero(t, attempts.boundExecutionID)
			},
		},
		{
			name: "传输错误保留当前尝试", task: activeTask(domain.TaskStatusSubmitting),
			attempt: currentAttempt(0), dispatchErr: errors.New("timeout"), wantErr: "timeout",
			after: func(t *testing.T, attempts *attemptRepositoryStub, _ *dispatcherStub) {
				require.Equal(t, int64(11), attempts.recordedAttemptID)
				require.Zero(t, attempts.rejectedAttemptID)
			},
		},
		{
			name: "明确拒绝结束当前尝试", task: activeTask(domain.TaskStatusSubmitting),
			attempt: currentAttempt(0), dispatchErr: etaskclient.ErrRejected, wantErr: "拒绝",
			after: func(t *testing.T, attempts *attemptRepositoryStub, _ *dispatcherStub) {
				require.Equal(t, int64(11), attempts.rejectedAttemptID)
				require.Zero(t, attempts.recordedAttemptID)
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tasks := &taskRepositoryStub{task: testCase.task}
			attempts := &attemptRepositoryStub{attempt: testCase.attempt}
			dispatcher := &dispatcherStub{executionID: testCase.dispatchID, err: testCase.dispatchErr}
			if testCase.before != nil {
				testCase.before(tasks, attempts, dispatcher)
			}
			svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

			err := svc.StartTask(context.Background(), testCase.task.ID)
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
			if testCase.after != nil {
				testCase.after(t, attempts, dispatcher)
			}
		})
	}
}

func TestCompleteAttemptValidatesTerminalIdentity(t *testing.T) {
	testCases := []struct {
		name      string
		requestID string
		status    domain.AttemptStatus
		wantErr   string
		wantCalls int
	}{
		{name: "成功终态", requestID: "eflow:1:1", status: domain.AttemptStatusSuccess, wantCalls: 1},
		{name: "失败终态", requestID: "eflow:1:1", status: domain.AttemptStatusFailed, wantCalls: 1},
		{name: "缺少请求标识", status: domain.AttemptStatusSuccess, wantErr: "请求标识不能为空"},
		{name: "非终态", requestID: "eflow:1:1", status: domain.AttemptStatusRunning,
			wantErr: "终态非法"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			attempts := &attemptRepositoryStub{}
			svc := &taskService{attempts: attempts}
			_, err := svc.CompleteAttempt(context.Background(), testCase.requestID, testCase.status, "", "")
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
			require.Equal(t, testCase.wantCalls, attempts.completeCalls)
		})
	}
}

func TestReconcileTaskUsesPersistedExecutionState(t *testing.T) {
	testCases := []struct {
		name       string
		execution  etaskclient.Execution
		wantCalls  int
		wantStatus domain.AttemptStatus
	}{
		{name: "成功执行完成本地尝试", execution: etaskclient.Execution{ID: 9, Status: "SUCCESS", Result: `{"ok":true}`},
			wantCalls: 1, wantStatus: domain.AttemptStatusSuccess},
		{name: "失败执行完成本地尝试", execution: etaskclient.Execution{ID: 9, Status: "FAILED", Result: "执行失败"},
			wantCalls: 1, wantStatus: domain.AttemptStatusFailed},
		{name: "仍在运行不修改本地尝试", execution: etaskclient.Execution{ID: 9, Status: "RUNNING"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tasks := &taskRepositoryStub{task: activeTask(domain.TaskStatusRunning)}
			attempts := &attemptRepositoryStub{attempt: currentAttempt(9)}
			svc := &taskService{
				tasks: tasks, attempts: attempts,
				reader: &executionReaderStub{execution: testCase.execution},
			}

			err := svc.ReconcileTask(context.Background(), 1)

			require.NoError(t, err)
			require.Equal(t, testCase.wantCalls, attempts.completeCalls)
			require.Equal(t, testCase.wantStatus, attempts.completedStatus)
		})
	}
}

func activeTask(status domain.TaskStatus) domain.Task {
	return domain.Task{ID: 1, TenantID: 2, Status: status, CurrentAttemptID: 11}
}

func currentAttempt(executionID int64) domain.TaskAttempt {
	return domain.TaskAttempt{ID: 11, TaskID: 1, RequestID: "eflow:1:1", ExecutionID: executionID}
}

type taskRepositoryStub struct {
	repository.TaskRepository
	task             domain.Task
	findOrCreateTask domain.Task
	created          bool
}

func (s *taskRepositoryStub) FindOrCreate(context.Context, domain.Task) (domain.Task, bool, error) {
	return s.findOrCreateTask, s.created, nil
}

func (s *taskRepositoryStub) FindByID(context.Context, int64) (domain.Task, error) {
	return s.task, nil
}

func (s *taskRepositoryStub) FindByProcessNode(context.Context, int, string) (domain.Task, error) {
	if s.findOrCreateTask.ID > 0 {
		return s.findOrCreateTask, nil
	}
	return domain.Task{}, repository.ErrTaskNotFound
}

type attemptRepositoryStub struct {
	repository.TaskAttemptRepository
	attempt           domain.TaskAttempt
	boundExecutionID  int64
	recordedAttemptID int64
	rejectedAttemptID int64
	completeCalls     int
	completedStatus   domain.AttemptStatus
}

func (s *attemptRepositoryStub) FindByID(context.Context, int64) (domain.TaskAttempt, error) {
	return s.attempt, nil
}

func (s *attemptRepositoryStub) BindExecution(_ context.Context, _ int64, executionID int64) error {
	s.boundExecutionID = executionID
	return nil
}

func (s *attemptRepositoryStub) RecordSubmissionError(_ context.Context, attemptID int64, _ string) error {
	s.recordedAttemptID = attemptID
	return nil
}

func (s *attemptRepositoryStub) RejectSubmission(_ context.Context, attemptID int64, _ string) error {
	s.rejectedAttemptID = attemptID
	return nil
}

func (s *attemptRepositoryStub) Complete(_ context.Context, _ string, status domain.AttemptStatus,
	_, _ string) (domain.TaskAttempt, error) {
	s.completeCalls++
	s.completedStatus = status
	return domain.TaskAttempt{}, nil
}

type executionReaderStub struct {
	etaskclient.ExecutionReader
	execution etaskclient.Execution
}

func (s *executionReaderStub) Find(context.Context, int64) (etaskclient.Execution, error) {
	return s.execution, nil
}

type ticketServiceStub struct {
	ticketSvc.Service
	ticket domain.Ticket
}

func (s *ticketServiceStub) GetByID(context.Context, int64) (domain.Ticket, error) {
	return s.ticket, nil
}

type dispatcherStub struct {
	etaskclient.TaskDispatcher
	executionID int64
	err         error
	calls       int
	received    domain.TaskAttempt
	tenantID    int64
}

func (s *dispatcherStub) Dispatch(ctx context.Context, attempt domain.TaskAttempt) (int64, error) {
	s.calls++
	s.received = attempt
	s.tenantID = ctxutil.GetTenantID(ctx).Int64()
	return s.executionID, s.err
}
