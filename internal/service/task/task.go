package task

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/codebook"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eflow/internal/service/runner"
	"github.com/Duke1616/eflow/internal/service/task/dispatch"
	"github.com/Duke1616/eflow/internal/service/task/scheduler"
	"github.com/Duke1616/eflow/internal/service/ticket"
	"github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/ecodeclub/ekit/slice"
	"github.com/gotomicro/ego/core/elog"
	"golang.org/x/sync/errgroup"
)

type Unit uint8

const (
	MINUTE Unit = 1
	HOUR   Unit = 2
	DAY    Unit = 3
)

// Service 自动化任务管理服务接口
type Service interface {
	// CreateTask 根据流程节点信息创建并初始化自动化任务
	CreateTask(ctx context.Context, ticketID int64, processInstID int, nodeID string) (domain.Task, error)
	// StartTask 启动指定任务的下发和执行逻辑
	StartTask(ctx context.Context, id int64) error
	// RetryTask 手动触发重试失败任务
	RetryTask(ctx context.Context, id int64) error
	// AutoRetryTask 自动重试机制触发的任务执行
	AutoRetryTask(ctx context.Context, id int64) error
	// UpdateTaskStatus 更新任务在流程中的生命周期状态
	UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error)
	// UpdateArgs 覆写修改任务下发参数
	UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error)
	// UpdateVariables 覆写修改任务运行的环境变量
	UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error)
	// ListTaskByStatus 分页获取特定状态下的任务列表
	ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]domain.Task, int64, error)
	// ListTaskByStatusAndKind 根据类型与状态筛选分页拉取任务
	ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]domain.Task, int64, error)
	// ListTask 分页拉取所有任务
	ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, int64, error)
	// ListTaskByInstanceID 根据实例 ID 分页筛选对应的子任务列表
	ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]domain.Task, int64, error)
	// ListSuccessTasksByUtime 拉取在指定更新时间之后且状态为成功的所有任务
	ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]domain.Task, int64, error)
	// FindTaskByNodeID 根据流程实例 ID 与节点 ID 查询关联的自动化任务实体
	FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (domain.Task, error)
	// FindTaskByID 查询指定主键 ID 任务的完整详细属性
	FindTaskByID(ctx context.Context, id int64) (domain.Task, error)
	// MarkTaskAsAutoPassed 强制将指定任务标记为自动通过，以进行容灾或人工干预流程推进
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	// UpdateExternalID 绑定该任务与调度系统分配的外部 ID 映射关系
	UpdateExternalID(ctx context.Context, id int64, externalID string) error
	// ListReadyTasks 检索所有处于待触发就绪状态的定时任务列表
	ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error)
}

type taskService struct {
	repo        repository.TaskRepository
	engineSvc   engine.Service
	workflowSvc workflow.Service
	codebookSvc codebook.Service
	runnerSvc   runner.Service
	ticketSvc   ticket.Service
	scheduler   scheduler.Scheduler
	dispatcher  dispatch.TaskDispatcher
	logger      *elog.Component
}

func NewTaskService(repo repository.TaskRepository, workflowSvc workflow.Service, codebookSvc codebook.Service,
	runnerSvc runner.Service, engineSvc engine.Service, ticketSvc ticket.Service, scheduler scheduler.Scheduler, dispatcher dispatch.TaskDispatcher) Service {
	return &taskService{
		repo:        repo,
		engineSvc:   engineSvc,
		workflowSvc: workflowSvc,
		codebookSvc: codebookSvc,
		runnerSvc:   runnerSvc,
		ticketSvc:   ticketSvc,
		scheduler:   scheduler,
		dispatcher:  dispatcher,
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("taskService")),
	}
}

func (s *taskService) CreateTask(ctx context.Context, ticketID int64, processInstID int, nodeID string) (domain.Task, error) {
	// 补全租户信息
	resp, err := s.ticketSvc.GetByID(ctx, ticketID)
	if err != nil {
		return domain.Task{}, err
	}
	ctx = ctxutil.WithTenantID(ctx, resp.TenantID)

	// 创建或更新
	task, err := s.repo.FindOrCreate(ctx, domain.Task{
		ProcessInstId:   processInstID,
		TriggerPosition: domain.TriggerPositionTaskWaiting.ToString(),
		CurrentNodeId:   nodeID,
		Status:          domain.WAITING,
		TicketID:        ticketID,
	})
	if err != nil {
		return domain.Task{}, err
	}

	task, err = s.prepareTask(ctx, task)
	if err != nil {
		return task, err
	}
	if !task.IsTiming {
		if startErr := s.StartTask(ctx, task.Id); startErr != nil {
			s.logger.Error("即时任务自动启动失败", elog.FieldErr(startErr), elog.Int64("taskId", task.Id))
		}
	}
	return task, nil
}

func (s *taskService) StartTask(ctx context.Context, id int64) error {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return s.dispatchTask(ctx, task)
}

func (s *taskService) RetryTask(ctx context.Context, id int64) error {
	_, _ = s.UpdateTaskStatus(ctx, domain.TaskResult{
		Id:              id,
		TriggerPosition: domain.TriggerPositionManualRetry.ToString(),
		RetryCount:      -1,
	})
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return s.retry(ctx, task, false)
}

func (s *taskService) AutoRetryTask(ctx context.Context, id int64) error {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return s.retry(ctx, task, true)
}

func (s *taskService) UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error) {
	if req.Status != domain.SCHEDULED && req.Status != domain.WAITING {
		s.scheduler.Remove(req.Id)
	}
	now := time.Now().UnixMilli()
	switch req.Status {
	case domain.RUNNING:
		req.StartTime = now
	case domain.SUCCESS, domain.FAILED:
		req.EndTime = now
	}
	return s.repo.UpdateTaskStatus(ctx, req)
}

func (s *taskService) UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error) {
	return s.repo.UpdateArgs(ctx, id, args)
}

func (s *taskService) UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error) {
	task, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return 0, err
	}
	oldVars := slice.ToMap(task.Variables, func(element domain.Variables) string { return element.Key })
	variables = slice.Map(variables, func(idx int, src domain.Variables) domain.Variables {
		old, ok := oldVars[src.Key]
		if ok && old.Secret {
			return old
		}
		if ok {
			src.Secret = old.Secret
		}
		return src
	})
	return s.repo.UpdateVariables(ctx, id, variables)
}

func (s *taskService) ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]domain.Task, int64, error) {
	var eg errgroup.Group
	var ts []domain.Task
	var total int64
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTaskByStatus(ctx, offset, limit, status)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.Total(ctx, status)
		return err
	})
	return ts, total, eg.Wait()
}

func (s *taskService) ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]domain.Task, int64, error) {
	var eg errgroup.Group
	var ts []domain.Task
	var total int64
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTaskByStatusAndKind(ctx, offset, limit, status, kind)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.TotalByStatusAndKind(ctx, status, kind)
		return err
	})
	return ts, total, eg.Wait()
}

func (s *taskService) ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, int64, error) {
	var eg errgroup.Group
	var ts []domain.Task
	var total int64
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTask(ctx, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.Total(ctx, 0)
		return err
	})
	return ts, total, eg.Wait()
}

func (s *taskService) ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]domain.Task, int64, error) {
	var eg errgroup.Group
	var ts []domain.Task
	var total int64
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTaskByInstanceID(ctx, offset, limit, instanceID)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.TotalByInstanceID(ctx, instanceID)
		return err
	})
	return ts, total, eg.Wait()
}

func (s *taskService) ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]domain.Task, int64, error) {
	var eg errgroup.Group
	var ts []domain.Task
	var total int64
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListSuccessTasksByUtime(ctx, offset, limit, utime)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.TotalByUtime(ctx, utime)
		return err
	})
	return ts, total, eg.Wait()
}

func (s *taskService) FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (domain.Task, error) {
	return s.repo.FindTaskByNodeID(ctx, instanceID, nodeID)
}

func (s *taskService) FindTaskByID(ctx context.Context, id int64) (domain.Task, error) {
	return s.repo.FindByID(ctx, id)
}

func (s *taskService) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return s.repo.MarkTaskAsAutoPassed(ctx, id)
}

func (s *taskService) UpdateExternalID(ctx context.Context, id int64, externalID string) error {
	return s.repo.UpdateExternalID(ctx, id, externalID)
}

func (s *taskService) ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error) {
	return s.repo.ListReadyTasks(ctx, limit)
}

func (s *taskService) getAutomationProperty(ctx context.Context, task domain.Task) (int64, easyflow.AutomationProperty, error) {
	inst, err := s.engineSvc.GetInstanceByID(ctx, task.ProcessInstId)
	if err != nil {
		return 0, easyflow.AutomationProperty{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetProcessInst.ToString(), domain.FAILED, err)
	}
	workflowID := task.WorkflowId
	if workflowID == 0 {
		workflowID, _ = strconv.ParseInt(inst.BusinessID, 10, 64)
	}
	flow, err := s.workflowSvc.FindInstanceFlow(ctx, workflowID, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return 0, easyflow.AutomationProperty{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetProcessInfo.ToString(), domain.FAILED, err)
	}
	automation, err := s.workflowSvc.GetAutomationProperty(s.toEasyWorkflow(flow), task.CurrentNodeId)
	if err != nil {
		return 0, easyflow.AutomationProperty{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorExtractAutomationInfo.ToString(), domain.FAILED, err)
	}
	return flow.Id, automation, nil
}

func (s *taskService) getRunnerAndCodebook(ctx context.Context, task domain.Task, automation easyflow.AutomationProperty) (domain.Runner, domain.Codebook, error) {
	s.logger.Info("准备匹配 Runner 执行器",
		elog.Int64("taskId", task.Id),
		elog.String("codebookUid", automation.CodebookUid),
		elog.String("tag", automation.Tag),
	)
	runner, err := s.runnerSvc.FindByCodebookUidAndTag(ctx, automation.CodebookUid, automation.Tag)
	if err != nil {
		s.logger.Error("获取执行器 Runner 失败",
			elog.Int64("taskId", task.Id),
			elog.String("codebookUid", automation.CodebookUid),
			elog.String("tag", automation.Tag),
			elog.FieldErr(err),
		)
		return domain.Runner{}, domain.Codebook{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetDispatcherNode.ToString(), domain.BLOCKED, err)
	}
	s.logger.Info("成功匹配 Runner 执行器",
		elog.Int64("taskId", task.Id),
		elog.Int64("runnerId", runner.Id),
		elog.String("runnerName", runner.Name),
		elog.String("runnerKind", runner.Kind.ToString()),
		elog.String("runnerTarget", runner.Target),
		elog.String("runnerHandler", runner.Handler),
	)
	codebook, err := s.codebookSvc.GetByIdentifier(ctx, runner.CodebookUid)
	if err != nil {
		return domain.Runner{}, domain.Codebook{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetTaskTemplate.ToString(), domain.FAILED, err)
	}
	return runner, codebook, nil
}

func (s *taskService) prepareTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	flowID, automation, err := s.getAutomationProperty(ctx, task)
	if err != nil {
		return domain.Task{}, err
	}

	runner, codebook, err := s.getRunnerAndCodebook(ctx, task, automation)
	if err != nil {
		return domain.Task{}, err
	}

	task.WorkflowId = flowID
	task.CodebookUid = codebook.Identifier
	task.Code = codebook.Code
	task.Language = codebook.Language
	task.Kind = runner.Kind
	task.Target = runner.Target
	task.Handler = runner.Handler
	task.Variables = runner.Variables
	task.Args = domain.TaskArgs{"ticket_id": task.TicketID, "process_inst_id": task.ProcessInstId}
	task.Status = domain.WAITING
	task.IsTiming = automation.IsTiming
	task.ScheduledTime = s.calculateScheduledTime(automation, task.Args)
	task.TriggerPosition = domain.TriggerPositionReadyToStartNode.ToString()
	if task.IsTiming {
		task.TriggerPosition = fmt.Sprintf("预计 %s 触发", time.UnixMilli(task.ScheduledTime).Format("2006-01-02 15:04:05"))
	}
	_, err = s.repo.UpdateTask(ctx, task)
	return task, err
}

func (s *taskService) dispatchTask(ctx context.Context, task domain.Task) error {
	if !s.scheduler.Add(task.Id) {
		return nil
	}
	if err := s.dispatcher.Dispatch(ctx, task); err != nil {
		s.scheduler.Remove(task.Id)
		return err
	}
	if !task.IsTiming || task.Kind == domain.GRPC {
		_, _ = s.UpdateTaskStatus(ctx, domain.TaskResult{
			Id:              task.Id,
			Status:          domain.RUNNING,
			TriggerPosition: domain.TriggerPositionDispatchDelivered.ToString(),
		})
	}
	return nil
}

func (s *taskService) retry(ctx context.Context, task domain.Task, auto bool) error {
	if auto && task.RetryCount >= 5 {
		_, _ = s.UpdateTaskStatus(ctx, domain.TaskResult{
			Id:              task.Id,
			TriggerPosition: domain.TriggerPositionAutoRetryLimitExceeded.ToString(),
			Status:          domain.BLOCKED,
		})
		return nil
	}
	res := domain.TaskResult{Id: task.Id, TriggerPosition: domain.TriggerPositionManualRetry.ToString(), Status: domain.SCHEDULED}
	if auto {
		res.TriggerPosition = domain.TriggerPositionAutoRetry.ToString()
		res.RetryCount = 1
	}
	_, _ = s.UpdateTaskStatus(ctx, res)
	return s.dispatchTask(ctx, task)
}

func (s *taskService) handleTaskError(ctx context.Context, taskID int64, triggerPosition string, status domain.TaskStatus, err error) error {
	_, updateErr := s.UpdateTaskStatus(ctx, domain.TaskResult{
		Id:              taskID,
		TriggerPosition: triggerPosition,
		Status:          status,
		Result:          err.Error(),
	})
	if updateErr != nil {
		s.logger.Error("更新任务状态失败", elog.FieldErr(updateErr))
	}
	return err
}

func (s *taskService) calculateScheduledTime(automation easyflow.AutomationProperty, data map[string]interface{}) int64 {
	if !automation.IsTiming {
		return time.Now().UnixMilli()
	}
	unit := Unit(automation.Unit)
	if unit == 0 {
		unit = HOUR
	}
	quantity := automation.Quantity
	if quantity <= 0 {
		quantity = 2
	}
	return time.Now().Add(s.calculateDuration(unit, quantity)).UnixMilli()
}

func (s *taskService) calculateDuration(unit Unit, quantity int64) time.Duration {
	switch unit {
	case MINUTE:
		return time.Duration(quantity) * time.Minute
	case DAY:
		return time.Duration(quantity) * 24 * time.Hour
	default:
		return time.Duration(quantity) * time.Hour
	}
}

func (s *taskService) toEasyWorkflow(wf domain.Workflow) easyflow.Workflow {
	edges := make([]map[string]interface{}, len(wf.FlowData.Edges))
	for i, e := range wf.FlowData.Edges {
		edges[i] = map[string]interface{}(e)
	}
	nodes := make([]map[string]interface{}, len(wf.FlowData.Nodes))
	for i, n := range wf.FlowData.Nodes {
		nodes[i] = map[string]interface{}(n)
	}
	return easyflow.Workflow{
		Id: wf.Id, Name: wf.Name, Owner: wf.Owner,
		FlowData: easyflow.LogicFlow{Edges: edges, Nodes: nodes},
	}
}
