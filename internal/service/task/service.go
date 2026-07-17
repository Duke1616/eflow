package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	dispatchSvc "github.com/Duke1616/eflow/internal/service/dispatch"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/gotomicro/ego/core/elog"
)

const maxTaskAttempts = 5

// Service 定义流程自动化任务的编排能力。
type Service interface {
	// CreateTask 创建任务，并固化触发时的节点名称与流程版本快照。
	CreateTask(ctx context.Context, ticketID int64, processInstanceID int, nodeID, nodeName string) (domain.Task, error)
	// StartTask 创建或恢复执行尝试并提交 etask。
	StartTask(ctx context.Context, id int64) error
	// RetryTask 创建一次人工执行尝试。
	RetryTask(ctx context.Context, id int64) error
	// AutoRetryTask 创建一次自动恢复执行尝试。
	AutoRetryTask(ctx context.Context, id int64) error
	// CompleteAttempt 根据幂等请求标识完成执行尝试。
	CompleteAttempt(ctx context.Context, requestID string, status domain.AttemptStatus,
		output, reason string) (domain.TaskAttempt, error)
	// ListTasksByStatusAfterID 按主键游标查询指定状态任务。
	ListTasksByStatusAfterID(ctx context.Context, status domain.TaskStatus,
		afterID, limit int64) ([]domain.Task, error)
	// ListTask 分页查询全部任务。
	ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, int64, error)
	// ListTaskByInstanceID 查询流程实例下的任务。
	ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]domain.Task, int64, error)
	// ListUnadvancedSuccessTasks 按主键游标查询一批尚未推进流程的成功任务。
	ListUnadvancedSuccessTasks(ctx context.Context, limit, afterID int64) ([]domain.Task, error)
	// FindTaskByNodeID 查询流程实例节点任务，并补充当前执行输出。
	FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (domain.Task, error)
	// ListAttempts 查询任务全部执行尝试。
	ListAttempts(ctx context.Context, taskID int64) ([]domain.TaskAttempt, error)
	// Logs 根据执行尝试读取 etask 日志。
	Logs(ctx context.Context, attemptID, minID int64, limit int) ([]etaskclient.ExecutionLog, int64, error)
	// ReconcileTask 使用 etask 持久化状态对账长时间运行中的任务。
	ReconcileTask(ctx context.Context, id int64) error
	// MarkTaskAsAutoPassed 标记任务已经推进流程。
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	// ListReadyTasks 查询已经到计划时间的任务。
	ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error)
}

type taskService struct {
	tasks      repository.TaskRepository
	attempts   repository.TaskAttemptRepository
	engine     engine.Service
	workflows  workflow.Service
	runners    etaskclient.RunnerCatalog
	tickets    ticket.Service
	dispatches dispatchSvc.Service
	executions etaskclient.TaskDispatcher
	reader     etaskclient.ExecutionReader
	users      userv1.UserServiceClient
	logger     *elog.Component
}

// NewTaskService 创建自动化任务编排服务。
func NewTaskService(tasks repository.TaskRepository, attempts repository.TaskAttemptRepository,
	workflows workflow.Service, runners etaskclient.RunnerCatalog, engineService engine.Service,
	tickets ticket.Service, dispatches dispatchSvc.Service, executions etaskclient.TaskDispatcher,
	reader etaskclient.ExecutionReader, users userv1.UserServiceClient) Service {
	return &taskService{
		tasks: tasks, attempts: attempts, workflows: workflows, runners: runners,
		engine: engineService, tickets: tickets, dispatches: dispatches,
		executions: executions, reader: reader, users: users,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("service.automation")),
	}
}

func (s *taskService) CreateTask(ctx context.Context, ticketID int64,
	processInstanceID int, nodeID, nodeName string) (domain.Task, error) {
	ticket, err := s.tickets.GetByID(ctx, ticketID)
	if err != nil {
		return domain.Task{}, err
	}
	ctx = tenantContext(ctx, ticket.TenantID)
	existing, err := s.tasks.FindByProcessNode(ctx, processInstanceID, nodeID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrTaskNotFound) {
		return domain.Task{}, err
	}
	instance, err := s.engine.GetInstanceByID(ctx, processInstanceID)
	if err != nil {
		return domain.Task{}, fmt.Errorf("查询流程实例失败: %w", err)
	}
	draft := domain.Task{
		TenantID: ticket.TenantID, TicketID: ticketID,
		ProcessInstanceID: processInstanceID, NodeID: nodeID,
		NodeName: nodeName, ProcessVersion: instance.ProcVersion,
		Status: domain.TaskStatusWaiting, Phase: domain.TaskPhaseReady,
	}
	scheduledAt, err := s.prepareSchedule(ctx, draft, ticket)
	if err != nil {
		draft.Status = domain.TaskStatusBlocked
		draft.Phase = domain.TaskPhaseBlocked
		draft.LastError = err.Error()
		blocked, _, persistErr := s.tasks.FindOrCreate(ctx, draft)
		if persistErr != nil {
			return domain.Task{}, errors.Join(err,
				fmt.Errorf("保存自动化任务阻塞状态失败: %w", persistErr))
		}
		return blocked, err
	}
	draft.ScheduledAt = scheduledAt
	task, created, err := s.tasks.FindOrCreate(ctx, draft)
	if err != nil {
		return domain.Task{}, err
	}
	if !created {
		return task, nil
	}
	if scheduledAt <= time.Now().UnixMilli() {
		if startErr := s.StartTask(ctx, task.ID); startErr != nil {
			s.logger.Error("即时自动化任务启动失败", elog.Int64("taskID", task.ID), elog.FieldErr(startErr))
			return task, startErr
		}
		return s.tasks.FindByID(ctx, task.ID)
	}
	return task, nil
}

func (s *taskService) StartTask(ctx context.Context, id int64) error {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	ctx = tenantContext(ctx, task.TenantID)

	if task.CurrentAttemptID > 0 &&
		(task.Status == domain.TaskStatusSubmitting || task.Status == domain.TaskStatusRunning) {
		attempt, findErr := s.attempts.FindByID(ctx, task.CurrentAttemptID)
		if findErr != nil {
			return findErr
		}
		if attempt.ExecutionID > 0 {
			return nil
		}
		return s.submitAttempt(ctx, attempt)
	}
	if task.Status != domain.TaskStatusWaiting {
		return nil
	}

	runnerID, input, err := s.prepareAttempt(ctx, task)
	if err != nil {
		return s.blockTask(ctx, task.ID, err)
	}
	attempt, err := s.attempts.Begin(ctx, task.ID, runnerID, input)
	if err != nil {
		return err
	}
	return s.submitAttempt(ctx, attempt)
}

func (s *taskService) submitAttempt(ctx context.Context, attempt domain.TaskAttempt) error {
	executionID, err := s.executions.Dispatch(ctx, attempt)
	if err != nil {
		var stateErr error
		if errors.Is(err, etaskclient.ErrRejected) {
			stateErr = s.attempts.RejectSubmission(ctx, attempt.ID, err.Error())
		} else {
			// 传输错误无法判断 etask 是否已创建执行，保留 request ID 供补偿任务幂等重投。
			stateErr = s.attempts.RecordSubmissionError(ctx, attempt.ID, err.Error())
		}
		if stateErr != nil {
			return errors.Join(err, fmt.Errorf("记录自动化任务提交状态失败: %w", stateErr))
		}
		return err
	}
	return s.attempts.BindExecution(ctx, attempt.ID, executionID)
}

func (s *taskService) RetryTask(ctx context.Context, id int64) error {
	return s.retry(ctx, id, false)
}

func (s *taskService) AutoRetryTask(ctx context.Context, id int64) error {
	return s.retry(ctx, id, true)
}

func (s *taskService) retry(ctx context.Context, id int64, automatic bool) error {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	ctx = tenantContext(ctx, task.TenantID)
	if !task.Status.CanRetry() {
		return fmt.Errorf("只有失败或阻塞的自动化任务可以重试")
	}
	if automatic {
		attempts, listErr := s.attempts.ListByTaskID(ctx, id)
		if listErr != nil {
			return listErr
		}
		if len(attempts) >= maxTaskAttempts {
			return s.tasks.Block(ctx, id, "超过最大执行尝试次数")
		}
	}
	if err = s.tasks.PrepareRetry(ctx, id); err != nil {
		return err
	}
	return s.StartTask(ctx, id)
}

func (s *taskService) CompleteAttempt(ctx context.Context, requestID string,
	status domain.AttemptStatus, output, reason string) (domain.TaskAttempt, error) {
	if strings.TrimSpace(requestID) == "" {
		return domain.TaskAttempt{}, fmt.Errorf("执行尝试请求标识不能为空")
	}
	if !status.IsTerminal() {
		return domain.TaskAttempt{}, fmt.Errorf("执行尝试终态非法: %s", status)
	}
	return s.attempts.Complete(ctx, requestID, status, output, reason)
}

func (s *taskService) ListTasksByStatusAfterID(ctx context.Context, status domain.TaskStatus,
	afterID, limit int64) ([]domain.Task, error) {
	return s.tasks.ListByStatusAfterID(ctx, status, afterID, limit)
}

func (s *taskService) ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, int64, error) {
	tasks, err := s.tasks.List(ctx, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.tasks.Count(ctx)
	return tasks, total, err
}

func (s *taskService) ListTaskByInstanceID(ctx context.Context, offset, limit int64,
	instanceID int) ([]domain.Task, int64, error) {
	tasks, err := s.tasks.ListByInstanceID(ctx, offset, limit, instanceID)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.tasks.CountByInstanceID(ctx, instanceID)
	return tasks, total, err
}

func (s *taskService) ListUnadvancedSuccessTasks(ctx context.Context, limit,
	afterID int64) ([]domain.Task, error) {
	return s.tasks.ListSucceededUnadvanced(ctx, limit, afterID)
}

func (s *taskService) FindTaskByNodeID(ctx context.Context, instanceID int,
	nodeID string) (domain.Task, error) {
	task, err := s.tasks.FindByProcessNode(ctx, instanceID, nodeID)
	if err != nil || task.CurrentAttemptID <= 0 {
		return task, err
	}
	attempt, err := s.attempts.FindByID(ctx, task.CurrentAttemptID)
	if err != nil {
		return domain.Task{}, err
	}
	task.Output = attempt.Output
	return task, nil
}

func (s *taskService) ListAttempts(ctx context.Context, taskID int64) ([]domain.TaskAttempt, error) {
	return s.attempts.ListByTaskID(ctx, taskID)
}

func (s *taskService) Logs(ctx context.Context, attemptID, minID int64,
	limit int) ([]etaskclient.ExecutionLog, int64, error) {
	attempt, err := s.attempts.FindByID(ctx, attemptID)
	if err != nil {
		return nil, 0, err
	}
	if attempt.ExecutionID <= 0 {
		return nil, 0, fmt.Errorf("执行尝试尚未绑定 etask execution")
	}
	return s.reader.Logs(ctx, attempt.ExecutionID, minID, limit)
}

func (s *taskService) ReconcileTask(ctx context.Context, id int64) error {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if task.Status != domain.TaskStatusRunning || task.CurrentAttemptID <= 0 {
		return nil
	}
	ctx = tenantContext(ctx, task.TenantID)
	attempt, err := s.attempts.FindByID(ctx, task.CurrentAttemptID)
	if err != nil {
		return err
	}
	if attempt.ExecutionID <= 0 {
		return nil
	}
	execution, err := s.reader.Find(ctx, attempt.ExecutionID)
	if err != nil {
		return fmt.Errorf("查询 etask 执行状态失败: %w", err)
	}
	switch execution.Status {
	case "SUCCESS":
		_, err = s.attempts.Complete(ctx, attempt.RequestID, domain.AttemptStatusSuccess,
			execution.Result, "")
	case "FAILED":
		_, err = s.attempts.Complete(ctx, attempt.RequestID, domain.AttemptStatusFailed,
			execution.Result, execution.Result)
	}
	return err
}

func (s *taskService) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return s.tasks.MarkAdvanced(ctx, id)
}

func (s *taskService) ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error) {
	return s.tasks.ListReady(ctx, limit)
}

func tenantContext(ctx context.Context, tenantID int64) context.Context {
	if tenantID <= 0 {
		return ctx
	}
	ctx = ctxutil.WithTenantID(ctx, tenantID)
	return ctxutil.WithOriginTenantID(ctx, tenantID)
}

func (s *taskService) blockTask(ctx context.Context, taskID int64, cause error) error {
	err := s.tasks.Block(ctx, taskID, cause.Error())
	if err != nil {
		return errors.Join(cause, fmt.Errorf("更新自动化任务阻塞状态失败: %w", err))
	}
	return cause
}

func (s *taskService) taskError(taskID int64, operation string, err error) error {
	s.logger.Error("自动化任务处理失败", elog.Int64("taskID", taskID),
		elog.String("operation", operation), elog.FieldErr(err))
	if taskID <= 0 {
		return fmt.Errorf("自动化任务%s失败: %w", operation, err)
	}
	return fmt.Errorf("自动化任务 %d %s失败: %w", taskID, operation, err)
}
