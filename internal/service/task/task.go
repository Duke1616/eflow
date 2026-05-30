package task

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	taskv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/task/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/codebook"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eflow/internal/service/runner"
	"github.com/Duke1616/eflow/internal/service/workflow"
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

type Service interface {
	CreateTask(ctx context.Context, orderId int64, processInstId int, nodeId string) (domain.Task, error)
	StartTask(ctx context.Context, id int64) error
	RetryTask(ctx context.Context, id int64) error
	AutoRetryTask(ctx context.Context, id int64) error
	UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error)
	UpdateTaskResult(ctx context.Context, req domain.TaskResult) (int64, error)
	UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error)
	UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error)
	ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]domain.Task, int64, error)
	ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]domain.Task, int64, error)
	ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, int64, error)
	ListTaskByInstanceId(ctx context.Context, offset, limit int64, instanceId int) ([]domain.Task, int64, error)
	ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]domain.Task, int64, error)
	FindTaskResult(ctx context.Context, instanceId int, nodeId string) (domain.Task, error)
	Detail(ctx context.Context, id int64) (domain.Task, error)
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	UpdateExternalId(ctx context.Context, id int64, externalId string) error
	ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error)
}

type taskService struct {
	repo        repository.TaskRepository
	engineSvc   engine.Service
	workflowSvc workflow.Service
	codebookSvc codebook.Service
	runnerSvc   runner.Service
	grpcClient  taskv1.TaskServiceClient
	scheduler   *taskScheduler
	logger      *elog.Component
}

func NewTaskService(repo repository.TaskRepository, workflowSvc workflow.Service, codebookSvc codebook.Service,
	runnerSvc runner.Service, engineSvc engine.Service, grpcClient taskv1.TaskServiceClient) Service {
	return &taskService{
		repo:        repo,
		engineSvc:   engineSvc,
		workflowSvc: workflowSvc,
		codebookSvc: codebookSvc,
		runnerSvc:   runnerSvc,
		grpcClient:  grpcClient,
		scheduler:   newTaskScheduler(),
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("taskService")),
	}
}

func (s *taskService) CreateTask(ctx context.Context, orderId int64, processInstId int, nodeId string) (domain.Task, error) {
	task, err := s.repo.FindOrCreate(ctx, domain.Task{
		ProcessInstId:   processInstId,
		TriggerPosition: domain.TriggerPositionTaskWaiting.ToString(),
		CurrentNodeId:   nodeId,
		Status:          domain.WAITING,
		OrderId:         orderId,
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
	task, err := s.repo.FindById(ctx, id)
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
	task, err := s.repo.FindById(ctx, id)
	if err != nil {
		return err
	}
	return s.retry(ctx, task, false)
}

func (s *taskService) AutoRetryTask(ctx context.Context, id int64) error {
	task, err := s.repo.FindById(ctx, id)
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

func (s *taskService) UpdateTaskResult(ctx context.Context, req domain.TaskResult) (int64, error) {
	return s.UpdateTaskStatus(ctx, req)
}

func (s *taskService) UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error) {
	return s.repo.UpdateArgs(ctx, id, args)
}

func (s *taskService) UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error) {
	task, err := s.repo.FindById(ctx, id)
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

func (s *taskService) ListTaskByInstanceId(ctx context.Context, offset, limit int64, instanceId int) ([]domain.Task, int64, error) {
	var eg errgroup.Group
	var ts []domain.Task
	var total int64
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTaskByInstanceId(ctx, offset, limit, instanceId)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.TotalByInstanceId(ctx, instanceId)
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

func (s *taskService) FindTaskResult(ctx context.Context, instanceId int, nodeId string) (domain.Task, error) {
	return s.repo.FindTaskResult(ctx, instanceId, nodeId)
}

func (s *taskService) Detail(ctx context.Context, id int64) (domain.Task, error) {
	return s.repo.FindById(ctx, id)
}

func (s *taskService) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return s.repo.MarkTaskAsAutoPassed(ctx, id)
}

func (s *taskService) UpdateExternalId(ctx context.Context, id int64, externalId string) error {
	return s.repo.UpdateExternalId(ctx, id, externalId)
}

func (s *taskService) ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error) {
	return s.repo.ListReadyTasks(ctx, limit)
}

func (s *taskService) prepareTask(ctx context.Context, task domain.Task) (domain.Task, error) {
	inst, err := s.engineSvc.GetInstanceByID(ctx, task.ProcessInstId)
	if err != nil {
		return domain.Task{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetProcessInst.ToString(), domain.FAILED, err)
	}
	workflowID := task.WorkflowId
	if workflowID == 0 {
		workflowID, _ = strconv.ParseInt(inst.BusinessID, 10, 64)
	}
	flow, err := s.workflowSvc.FindInstanceFlow(ctx, workflowID, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return domain.Task{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetProcessInfo.ToString(), domain.FAILED, err)
	}
	automation, err := s.workflowSvc.GetAutomationProperty(s.toEasyWorkflow(flow), task.CurrentNodeId)
	if err != nil {
		return domain.Task{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorExtractAutomationInfo.ToString(), domain.FAILED, err)
	}
	runner, err := s.runnerSvc.FindByCodebookUidAndTag(ctx, automation.CodebookUid, automation.Tag)
	if err != nil {
		return domain.Task{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetDispatcherNode.ToString(), domain.BLOCKED, err)
	}
	codebook, err := s.codebookSvc.GetByIdentifier(ctx, runner.CodebookUid)
	if err != nil {
		return domain.Task{}, s.handleTaskError(ctx, task.Id, domain.TriggerPositionErrorGetTaskTemplate.ToString(), domain.FAILED, err)
	}

	task.WorkflowId = flow.Id
	task.CodebookUid = codebook.Identifier
	task.CodebookName = codebook.Name
	task.Code = codebook.Code
	task.Language = codebook.Language
	task.Kind = runner.Kind
	task.Target = runner.Target
	task.Handler = runner.Handler
	task.Variables = runner.Variables
	task.Args = domain.TaskArgs{"order_id": task.OrderId, "process_inst_id": task.ProcessInstId}
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
	if task.Kind == domain.GRPC {
		if err := s.dispatchGRPC(ctx, task); err != nil {
			s.scheduler.Remove(task.Id)
			return err
		}
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

func (s *taskService) dispatchGRPC(ctx context.Context, task domain.Task) error {
	taskId := strconv.FormatInt(task.Id, 10)
	args, _ := json.Marshal(task.Args)
	vars, _ := json.Marshal(task.Variables)
	hash := s.sumHash(taskId, task.Code, string(args), string(vars))
	resp, err := s.grpcClient.CreateTask(ctx, &taskv1.CreateTaskRequest{
		Name:     fmt.Sprintf("%s_%s", task.CodebookName, hash),
		Type:     taskv1.TaskType_ONE_TIME,
		CronExpr: s.calculateCronExpr(task),
		GrpcConfig: &taskv1.GrpcConfig{
			ServiceName: task.Target,
			HandlerName: task.Handler,
			Params: map[string]string{
				"task_id":   taskId,
				"code":      task.Code,
				"args":      string(args),
				"variables": string(vars),
			},
		},
	})
	if err != nil {
		return err
	}
	if resp.Code != taskv1.TaskErrorCode_SUCCESS {
		return fmt.Errorf("任务平台业务错误: %s (code: %d)", resp.Message, resp.Code)
	}
	return s.repo.UpdateExternalId(ctx, task.Id, strconv.FormatInt(resp.Id, 10))
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

func (s *taskService) calculateCronExpr(task domain.Task) string {
	executeTime := time.Now().Add(2 * time.Second)
	if task.IsTiming && task.ScheduledTime > time.Now().UnixMilli() {
		executeTime = time.UnixMilli(task.ScheduledTime)
	}
	return executeTime.Format("05 04 15 02 01 ?")
}

func (s *taskService) sumHash(ss ...string) string {
	h := md5.New()
	for _, str := range ss {
		_, _ = h.Write([]byte(str))
		_, _ = h.Write([]byte("|"))
	}
	return hex.EncodeToString(h.Sum(nil))
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

type taskScheduler struct {
	mu  sync.Mutex
	ids map[int64]struct{}
}

func newTaskScheduler() *taskScheduler {
	return &taskScheduler{ids: map[int64]struct{}{}}
}

func (s *taskScheduler) Add(id int64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.ids[id]; ok {
		return false
	}
	s.ids[id] = struct{}{}
	return true
}

func (s *taskScheduler) Remove(id int64) {
	s.mu.Lock()
	delete(s.ids, id)
	s.mu.Unlock()
}
