package task

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	dispatchSvc "github.com/Duke1616/eflow/internal/service/dispatch"
	"github.com/Duke1616/eflow/internal/service/engine"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	"github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/gotomicro/ego/core/elog"
)

const maxTaskAttempts = 5

var (
	ErrWithdrawalInProgress   = errors.New("工单正在撤回，暂不推进自动化节点")
	ErrInvalidTerminateReason = errors.New("终止原因不能为空且不能超过 500 个字符")
	ErrTaskNotTerminable      = errors.New("当前自动化任务不能终止")
)

// Service 定义流程自动化任务的编排能力。
type Service interface {
	// CreateTask 创建任务，并固化触发时的节点名称与流程版本快照。
	CreateTask(ctx context.Context, ticketID int64, instanceID domain.ID, nodeID, nodeName string, procVersion ...int) (domain.Task, error)
	// StartTask 创建或恢复执行尝试并提交 etask。
	StartTask(ctx context.Context, id int64) error
	// RetryTask 创建一次人工执行尝试。
	RetryTask(ctx context.Context, id int64) error
	// AutoRetryTask 创建一次自动恢复执行尝试。
	AutoRetryTask(ctx context.Context, id int64) error
	// TerminateTask 强制终止任务及其当前 etask execution，取消后仍可人工重试。
	TerminateTask(ctx context.Context, id int64, reason string) error
	// CompleteAttempt 根据幂等请求标识完成执行尝试。
	CompleteAttempt(ctx context.Context, requestID string, status domain.AttemptStatus,
		output, reason string) (domain.TaskAttempt, error)
	// ListTasksByStatusAfterID 按主键游标查询指定状态任务。
	ListTasksByStatusAfterID(ctx context.Context, status domain.TaskStatus,
		afterID, limit int64) ([]domain.Task, error)
	// ListTask 分页查询全部任务。
	ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, int64, error)
	// ListTaskByTicketID 查询工单下的自动化任务（核心推荐接口）。
	ListTaskByTicketID(ctx context.Context, offset, limit int64, ticketID int64) ([]domain.Task, int64, error)
	// ListUnadvancedSuccessTasks 按主键游标查询一批尚未推进流程的成功任务。
	ListUnadvancedSuccessTasks(ctx context.Context, limit, afterID int64) ([]domain.Task, error)
	// FindTaskByNodeID 查询流程实例节点任务，并补充当前执行输出。
	FindTaskByNodeID(ctx context.Context, instanceID int64, nodeID string) (domain.Task, error)
	// FindTaskByTicketNode 查询工单节点任务，并补充当前执行输出（核心入口）。
	FindTaskByTicketNode(ctx context.Context, ticketID int64, nodeID string) (domain.Task, error)
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
	// ShouldAdvanceProcessTask 判断成功任务是否还需要推进活动流程，并返回关联工单绑定的流程引擎类型。
	ShouldAdvanceProcessTask(ctx context.Context, id int64) (bool, domain.EngineType, error)
}

type taskService struct {
	tasks      repository.TaskRepository
	attempts   repository.TaskAttemptRepository
	engine     engine.IEngine
	workflows  workflow.Service
	tickets    ticket.Service
	dispatches dispatchSvc.Service
	templates  templateSvc.Service
	etask      etaskclient.Client
	logger     *elog.Component
}

// NewTaskService 创建自动化任务编排服务。
func NewTaskService(tasks repository.TaskRepository, attempts repository.TaskAttemptRepository,
	engine engine.IEngine, workflows workflow.Service, tickets ticket.Service,
	dispatches dispatchSvc.Service, templates templateSvc.Service, etask etaskclient.Client) Service {
	return &taskService{
		tasks: tasks, attempts: attempts, engine: engine, workflows: workflows,
		tickets: tickets, dispatches: dispatches, templates: templates, etask: etask,
		logger: elog.DefaultLogger.With(elog.FieldComponentName("service.automation")),
	}
}

func (s *taskService) CreateTask(ctx context.Context, ticketID int64,
	instanceID domain.ID, nodeID, nodeName string, procVersion ...int) (domain.Task, error) {
	var (
		ticket domain.Ticket
		err    error
	)
	if ticketID > 0 {
		ticket, err = s.tickets.GetByID(ctx, ticketID)
	} else if !instanceID.IsZero() {
		ticket, err = s.tickets.GetByProcessInstance(ctx, instanceID)
		if err == nil {
			ticketID = ticket.Id
		}
	} else {
		return domain.Task{}, fmt.Errorf("创建自动化任务缺少有效工单标识与流程实例标识")
	}
	if err != nil {
		return domain.Task{}, fmt.Errorf("查询关联工单失败 [ticketID=%d, instanceID=%s]: %w", ticketID, instanceID, err)
	}

	ctx = tenantContext(ctx, ticket.TenantID)
	existing, err := s.tasks.FindByTicketNode(ctx, ticketID, nodeID)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, repository.ErrTaskNotFound) {
		return domain.Task{}, err
	}

	// 优先使用触发调用方透传的流程快照版本；若未透传，统一通过 IEngine 防腐层拉取运行态视图
	var version int
	if len(procVersion) > 0 && procVersion[0] > 0 {
		version = procVersion[0]
	} else if s.engine != nil {
		view, viewErr := s.engine.GetInstanceExecutionView(ctx, ticket.Process.ResolveEngineType(), instanceID, ticket.Status.ToUint8())
		if viewErr != nil {
			return domain.Task{}, fmt.Errorf("通过流程引擎获取实例快照版本失败: %w", viewErr)
		}
		version = view.ProcessVersion
	}

	if version <= 0 {
		return domain.Task{}, fmt.Errorf("未能获取到流程实例 [%s] 的有效快照版本", instanceID)
	}

	task := domain.NewProcessTask(ticket.TenantID, ticketID, instanceID, nodeID, nodeName, version)
	definition, err := s.prepareTaskDefinition(ctx, task, ticket)
	task.Schedule(definition.scheduledAt, definition.compensationNodeID)
	if err != nil {
		task.Block(err.Error())
		blocked, _, persistErr := s.tasks.FindOrCreate(ctx, task)
		if persistErr != nil {
			return domain.Task{}, errors.Join(err,
				fmt.Errorf("保存自动化任务阻塞状态失败: %w", persistErr))
		}
		return blocked, err
	}

	saved, created, err := s.tasks.FindOrCreate(ctx, task)
	if err != nil || !created {
		return saved, err
	}
	if saved.ShouldStartImmediately(time.Now().UnixMilli(), ticket.Status) {
		if startErr := s.StartTask(ctx, saved.ID); startErr != nil {
			s.logger.Error("即时自动化任务启动失败", elog.Int64("taskID", saved.ID), elog.FieldErr(startErr))
			return saved, startErr
		}
		return s.tasks.FindByID(ctx, saved.ID)
	}
	return saved, nil
}

func (s *taskService) StartTask(ctx context.Context, id int64) error {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	ctx = tenantContext(ctx, task.TenantID)

	// 1. 若当前尝试仍处于提交恢复窗口，幂等补发
	if task.IsPendingSubmission() {
		return s.resumeSubmission(ctx, task.CurrentAttemptID)
	}
	// 2. 非等待中任务无需调度
	if !task.IsWaiting() {
		return nil
	}
	// 3. 准备并开启新执行尝试
	return s.startNewAttempt(ctx, task)
}

func (s *taskService) resumeSubmission(ctx context.Context, attemptID int64) error {
	attempt, err := s.attempts.FindByID(ctx, attemptID)
	if err != nil || attempt.HasExecution() {
		return err
	}
	return s.submitAttempt(ctx, attempt)
}

func (s *taskService) startNewAttempt(ctx context.Context, task domain.Task) error {
	decision, input, err := s.prepareAttempt(ctx, task)
	if err != nil {
		return s.blockTask(ctx, task.ID, err)
	}
	attempt, err := s.attempts.Begin(ctx, task.ID, decision, input)
	if err != nil {
		if errors.Is(err, repository.ErrTaskNotRunnable) {
			return nil
		}
		return err
	}
	return s.submitAttempt(ctx, attempt)
}

func (s *taskService) submitAttempt(ctx context.Context, attempt domain.TaskAttempt) error {
	executionID, err := s.etask.Dispatch(ctx, attempt)
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

func (s *taskService) TerminateTask(ctx context.Context, id int64, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return ErrInvalidTerminateReason
	}
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if err = task.ValidateTerminable(); err != nil {
		return fmt.Errorf("%w: %w", ErrTaskNotTerminable, err)
	}

	ctx = tenantContext(ctx, task.TenantID)
	attempt, err := s.attempts.TerminateTask(ctx, id, reason)
	if err != nil || !attempt.NeedsRemoteTermination() {
		return err
	}
	// etask 会持久化 request ID 取消意图；execution 尚未创建时无需重新 Dispatch。
	return s.etask.TerminateExecution(ctx, attempt.ExecutionID, attempt.RequestID, reason)
}

func (s *taskService) retry(ctx context.Context, id int64, automatic bool) error {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return err
	}
	ctx = tenantContext(ctx, task.TenantID)

	// 1. 提交中任务使用 request ID 幂等恢复当前尝试，不创建新的 attempt
	if task.Status == domain.TaskStatusSubmitting {
		return s.StartTask(ctx, id)
	}
	// 2. 领域规则校验（状态约束、自动重试限制）
	if err = task.ValidateRetry(automatic); err != nil {
		return err
	}
	// 3. 已取消任务重新执行前确认旧执行已彻底终结
	if task.Status == domain.TaskStatusCancelled {
		if err = s.confirmCancelledAttemptTerminated(ctx, task); err != nil {
			return err
		}
	}
	// 4. 自动重试防雪崩频次控制
	if automatic {
		if overLimit, limitErr := s.isMaxAttemptsReached(ctx, id); limitErr != nil || overLimit {
			return limitErr
		}
	}
	if err = s.tasks.PrepareRetry(ctx, id); err != nil {
		return err
	}
	return s.StartTask(ctx, id)
}

func (s *taskService) isMaxAttemptsReached(ctx context.Context, taskID int64) (bool, error) {
	attempts, err := s.attempts.ListByTaskID(ctx, taskID)
	if err != nil {
		return false, err
	}
	if len(attempts) >= maxTaskAttempts {
		return true, s.tasks.Block(ctx, taskID, domain.ErrTaskMaxAttemptsExceeded.Error())
	}
	return false, nil
}

func (s *taskService) confirmCancelledAttemptTerminated(ctx context.Context, task domain.Task) error {
	if task.CurrentAttemptID <= 0 {
		return nil
	}
	attempt, err := s.attempts.FindByID(ctx, task.CurrentAttemptID)
	if err != nil {
		return fmt.Errorf("查询已取消任务的执行尝试失败: %w", err)
	}
	if !attempt.HasIdentifiableRequest() {
		return nil
	}
	reason := strings.TrimSpace(attempt.Error)
	if reason == "" {
		reason = "重新执行前确认旧执行已终止"
	}
	if err = s.etask.TerminateExecution(ctx, attempt.ExecutionID, attempt.RequestID, reason); err != nil {
		return fmt.Errorf("确认已取消任务的旧执行已终止失败: %w", err)
	}
	return nil
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

func (s *taskService) ListTaskByTicketID(ctx context.Context, offset, limit int64,
	ticketID int64) ([]domain.Task, int64, error) {
	tasks, err := s.tasks.ListByTicketID(ctx, offset, limit, ticketID)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.tasks.CountByTicketID(ctx, ticketID)
	return tasks, total, err
}

func (s *taskService) ListUnadvancedSuccessTasks(ctx context.Context, limit,
	afterID int64) ([]domain.Task, error) {
	return s.tasks.ListSucceededUnadvanced(ctx, limit, afterID)
}

func (s *taskService) FindTaskByNodeID(ctx context.Context, instanceID int64,
	nodeID string) (domain.Task, error) {
	task, err := s.tasks.FindByProcessNode(ctx, instanceID, nodeID)
	if err != nil {
		return task, err
	}
	return s.enrichTaskOutput(ctx, task)
}

func (s *taskService) FindTaskByTicketNode(ctx context.Context, ticketID int64,
	nodeID string) (domain.Task, error) {
	task, err := s.tasks.FindByTicketNode(ctx, ticketID, nodeID)
	if err != nil {
		return task, err
	}
	return s.enrichTaskOutput(ctx, task)
}

func (s *taskService) enrichTaskOutput(ctx context.Context, task domain.Task) (domain.Task, error) {
	if task.CurrentAttemptID <= 0 {
		return task, nil
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
	return s.etask.Logs(ctx, attempt.ExecutionID, minID, limit)
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
	if err != nil || !attempt.HasExecution() {
		return err
	}
	execution, err := s.etask.Find(ctx, attempt.ExecutionID)
	if err != nil {
		return fmt.Errorf("查询 etask 执行状态失败: %w", err)
	}
	switch execution.Status {
	case "SUCCESS":
		_, err = s.CompleteAttempt(ctx, attempt.RequestID, domain.AttemptStatusSuccess,
			execution.Result, "")
	case "FAILED":
		_, err = s.CompleteAttempt(ctx, attempt.RequestID, domain.AttemptStatusFailed,
			execution.Result, execution.Result)
	case "CANCELLED":
		_, err = s.CompleteAttempt(ctx, attempt.RequestID, domain.AttemptStatusCancelled,
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

func (s *taskService) ShouldAdvanceProcessTask(ctx context.Context, id int64) (bool, domain.EngineType, error) {
	task, err := s.tasks.FindByID(ctx, id)
	if err != nil {
		return false, "", err
	}
	ticket, err := s.tickets.GetByID(tenantContext(ctx, task.TenantID), task.TicketID)
	if err != nil {
		return false, "", err
	}
	engineType := ticket.Process.ResolveEngineType()
	if ticket.Status == domain.WITHDRAWING {
		return false, engineType, ErrWithdrawalInProgress
	}
	return ticket.Status == domain.PROCESS, engineType, nil
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
