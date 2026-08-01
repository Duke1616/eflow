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

func TestRetryTaskResumesSubmittingAttempt(t *testing.T) {
	tasks := &taskRepositoryStub{task: activeTask(domain.TaskStatusSubmitting)}
	attempts := &attemptRepositoryStub{attempt: currentAttempt(0)}
	dispatcher := &dispatcherStub{executionID: 9001}
	svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

	err := svc.RetryTask(context.Background(), 1)

	require.NoError(t, err)
	require.Equal(t, 1, dispatcher.calls)
	require.Equal(t, "eflow:1:1", dispatcher.received.RequestID)
	require.Equal(t, int64(9001), attempts.boundExecutionID)
}

func TestRetryCancelledTaskConfirmsPreviousExecutionTermination(t *testing.T) {
	stopAfterConfirmation := errors.New("stop after confirmation")
	task := activeTask(domain.TaskStatusCancelled)
	attempt := currentAttempt(9001)
	attempt.Status = domain.AttemptStatusCancelled
	attempt.Error = "管理员强制结束"
	tasks := &taskRepositoryStub{task: task, prepareRetryErr: stopAfterConfirmation}
	attempts := &attemptRepositoryStub{attempt: attempt}
	dispatcher := &dispatcherStub{}
	svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

	err := svc.RetryTask(context.Background(), task.ID)

	require.ErrorIs(t, err, stopAfterConfirmation)
	require.Equal(t, 1, dispatcher.terminateCalls)
	require.Equal(t, int64(9001), dispatcher.terminatedExecutionID)
	require.Equal(t, "eflow:1:1", dispatcher.terminatedRequestID)
	require.Equal(t, "管理员强制结束", dispatcher.terminatedReason)
	require.Equal(t, 1, tasks.prepareRetryCalls)
}

func TestRetryCancelledTaskStopsWhenTerminationCannotBeConfirmed(t *testing.T) {
	task := activeTask(domain.TaskStatusCancelled)
	attempt := currentAttempt(9001)
	attempt.Status = domain.AttemptStatusCancelled
	tasks := &taskRepositoryStub{task: task}
	attempts := &attemptRepositoryStub{attempt: attempt}
	dispatcher := &dispatcherStub{terminateErr: errors.New("scheduler unavailable")}
	svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

	err := svc.RetryTask(context.Background(), task.ID)

	require.ErrorContains(t, err, "确认已取消任务的旧执行已终止失败")
	require.Zero(t, tasks.prepareRetryCalls)
}

func TestAutoRetryDoesNotRestartCancelledTask(t *testing.T) {
	task := activeTask(domain.TaskStatusCancelled)
	tasks := &taskRepositoryStub{task: task}
	dispatcher := &dispatcherStub{}
	svc := &taskService{tasks: tasks, attempts: &attemptRepositoryStub{}, executions: dispatcher}

	err := svc.AutoRetryTask(context.Background(), task.ID)

	require.ErrorContains(t, err, "只支持人工重试")
	require.Zero(t, dispatcher.terminateCalls)
	require.Zero(t, tasks.prepareRetryCalls)
}

func TestTerminateTaskStopsRemoteExecution(t *testing.T) {
	testCases := []struct {
		name               string
		task               domain.Task
		attempt            domain.TaskAttempt
		wantErr            string
		wantLocalTerminate bool
		wantRemote         bool
	}{
		{
			name: "运行中任务同步终止 etask", task: activeTask(domain.TaskStatusRunning),
			attempt: currentAttempt(9001), wantLocalTerminate: true, wantRemote: true,
		},
		{
			name: "提交中任务按 request ID 登记取消意图", task: activeTask(domain.TaskStatusSubmitting),
			attempt: currentAttempt(0), wantLocalTerminate: true, wantRemote: true,
		},
		{
			name: "失败任务转为已取消", task: activeTask(domain.TaskStatusFailed),
			attempt:            domain.TaskAttempt{ID: 11, TaskID: 1, Status: domain.AttemptStatusFailed},
			wantLocalTerminate: true,
		},
		{
			name: "已取消任务幂等补发远端终止", task: activeTask(domain.TaskStatusCancelled),
			attempt: func() domain.TaskAttempt {
				attempt := currentAttempt(9001)
				attempt.Status = domain.AttemptStatusCancelled
				return attempt
			}(), wantLocalTerminate: true, wantRemote: true,
		},
		{
			name: "成功任务不能终止", task: activeTask(domain.TaskStatusSuccess),
			wantErr: "任务已经成功",
		},
		{
			name: "补偿任务不能终止", task: func() domain.Task {
				task := activeTask(domain.TaskStatusRunning)
				task.ExecutionKind = domain.TaskExecutionCompensation
				return task
			}(),
			wantErr: "补偿任务必须成功",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tasks := &taskRepositoryStub{task: testCase.task}
			attempts := &attemptRepositoryStub{attempt: testCase.attempt}
			dispatcher := &dispatcherStub{executionID: 9001}
			svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

			err := svc.TerminateTask(context.Background(), 1, "管理员强制结束")
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
			require.Equal(t, testCase.wantLocalTerminate, attempts.terminateCalls == 1)
			require.Equal(t, testCase.wantRemote, dispatcher.terminateCalls == 1)
			if testCase.wantRemote {
				require.Equal(t, testCase.attempt.ExecutionID, dispatcher.terminatedExecutionID)
				require.Equal(t, "eflow:1:1", dispatcher.terminatedRequestID)
				require.Equal(t, int64(2), dispatcher.tenantID)
			}
		})
	}
}

func TestSubmitAttemptOnlyBindsExecutionAfterDurableCancellationIntent(t *testing.T) {
	tasks := &taskRepositoryStub{}
	attempts := &attemptRepositoryStub{}
	dispatcher := &dispatcherStub{executionID: 9001}
	svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

	err := svc.submitAttempt(tenantContext(context.Background(), 2), currentAttempt(0))

	require.NoError(t, err)
	require.Equal(t, int64(9001), attempts.boundExecutionID)
	require.Zero(t, dispatcher.terminateCalls)
}

func TestTerminateTaskUsesAttemptReturnedByTerminationTransaction(t *testing.T) {
	before := activeTask(domain.TaskStatusRunning)
	before.CurrentAttemptID = 0
	tasks := &taskRepositoryStub{findByID: []domain.Task{before}}
	attempts := &attemptRepositoryStub{attempt: func() domain.TaskAttempt {
		attempt := currentAttempt(0)
		attempt.Status = domain.AttemptStatusCancelled
		return attempt
	}()}
	dispatcher := &dispatcherStub{}
	svc := &taskService{tasks: tasks, attempts: attempts, executions: dispatcher}

	err := svc.TerminateTask(context.Background(), 1, "管理员强制结束")

	require.NoError(t, err)
	require.Equal(t, 1, attempts.terminateCalls)
	require.Equal(t, 1, dispatcher.terminateCalls)
	require.Equal(t, int64(0), dispatcher.terminatedExecutionID)
	require.Equal(t, "eflow:1:1", dispatcher.terminatedRequestID)
	require.Equal(t, 1, tasks.findByIDCalls)
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

func TestCompleteAttemptFinalizesSuccessfulCompensation(t *testing.T) {
	tasks := &taskRepositoryStub{task: domain.Task{
		ID: 1, TenantID: 7, ProcessInstanceID: 101, ExecutionKind: domain.TaskExecutionCompensation,
	}}
	attempts := &attemptRepositoryStub{completedAttempt: domain.TaskAttempt{
		ID: 11, TaskID: 1, Status: domain.AttemptStatusSuccess,
	}}
	withdrawals := &taskWithdrawalRepositoryStub{}
	svc := &taskService{tasks: tasks, attempts: attempts, withdrawal: withdrawals}

	_, err := svc.CompleteAttempt(context.Background(), "eflow:1:1",
		domain.AttemptStatusSuccess, "", "")

	require.NoError(t, err)
	require.Equal(t, 101, withdrawals.processInstanceID)
	require.Equal(t, int64(7), withdrawals.tenantID)
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
	task              domain.Task
	findByID          []domain.Task
	findByIDCalls     int
	findOrCreateTask  domain.Task
	created           bool
	prepareRetryCalls int
	prepareRetryErr   error
}

func (s *taskRepositoryStub) PrepareRetry(context.Context, int64) error {
	s.prepareRetryCalls++
	return s.prepareRetryErr
}

func (s *taskRepositoryStub) FindOrCreate(context.Context, domain.Task) (domain.Task, bool, error) {
	return s.findOrCreateTask, s.created, nil
}

func (s *taskRepositoryStub) FindByID(context.Context, int64) (domain.Task, error) {
	if s.findByIDCalls < len(s.findByID) {
		task := s.findByID[s.findByIDCalls]
		s.findByIDCalls++
		return task, nil
	}
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
	completedAttempt  domain.TaskAttempt
	terminateCalls    int
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

func (s *attemptRepositoryStub) TerminateTask(context.Context, int64,
	string) (domain.TaskAttempt, error) {
	s.terminateCalls++
	return s.attempt, nil
}

func (s *attemptRepositoryStub) Complete(_ context.Context, _ string, status domain.AttemptStatus,
	_, _ string) (domain.TaskAttempt, error) {
	s.completeCalls++
	s.completedStatus = status
	return s.completedAttempt, nil
}

type taskWithdrawalRepositoryStub struct {
	repository.WithdrawalRepository
	processInstanceID int
	tenantID          int64
}

func (s *taskWithdrawalRepositoryStub) TryFinalize(ctx context.Context,
	processInstanceID int) (bool, error) {
	s.processInstanceID = processInstanceID
	s.tenantID = ctxutil.GetTenantID(ctx).Int64()
	return true, nil
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
	executionID           int64
	err                   error
	calls                 int
	received              domain.TaskAttempt
	tenantID              int64
	terminateCalls        int
	terminatedExecutionID int64
	terminatedRequestID   string
	terminatedReason      string
	terminateErr          error
}

func (s *dispatcherStub) TerminateExecution(ctx context.Context, executionID int64,
	requestID, reason string) error {
	s.terminateCalls++
	s.terminatedExecutionID = executionID
	s.terminatedRequestID = requestID
	s.terminatedReason = reason
	s.tenantID = ctxutil.GetTenantID(ctx).Int64()
	return s.terminateErr
}

func (s *dispatcherStub) Dispatch(ctx context.Context, attempt domain.TaskAttempt) (int64, error) {
	s.calls++
	s.received = attempt
	s.tenantID = ctxutil.GetTenantID(ctx).Int64()
	return s.executionID, s.err
}
