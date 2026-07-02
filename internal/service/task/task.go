package task

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	codebookv1 "github.com/Duke1616/eflow/api/proto/gen/etask/codebook/v1"
	runnerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/runner/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
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

	// ResetRetryCountFlag 特殊约定：-1 表示重置重试计数器为 0
	ResetRetryCountFlag = -1
)

var ErrSecretVariableManagedByEtask = errors.New("敏感变量由 etask 管理，eflow 不允许新增或改写敏感变量明文")

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
	codebookCli codebookv1.CodebookServiceClient
	runnerCli   runnerv1.RunnerServiceClient
	ticketSvc   ticket.Service
	scheduler   scheduler.Scheduler
	dispatcher  dispatch.TaskDispatcher
	userClient  userv1.UserServiceClient
	logger      *elog.Component
}

func NewTaskService(repo repository.TaskRepository, workflowSvc workflow.Service, codebookCli codebookv1.CodebookServiceClient,
	runnerCli runnerv1.RunnerServiceClient, engineSvc engine.Service, ticketSvc ticket.Service, scheduler scheduler.Scheduler, dispatcher dispatch.TaskDispatcher,
	userClient userv1.UserServiceClient) Service {
	return &taskService{
		repo:        repo,
		engineSvc:   engineSvc,
		workflowSvc: workflowSvc,
		codebookCli: codebookCli,
		runnerCli:   runnerCli,
		ticketSvc:   ticketSvc,
		scheduler:   scheduler,
		dispatcher:  dispatcher,
		userClient:  userClient,
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

	task, err = s.prepareTask(ctx, task, resp)
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
		RetryCount:      ResetRetryCountFlag,
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
	updated := make([]domain.Variables, 0, len(variables))
	for _, src := range variables {
		old, ok := oldVars[src.Key]
		if ok && old.Secret {
			updated = append(updated, old)
			continue
		}
		if ok {
			src.Secret = old.Secret
		}
		if src.Secret {
			return 0, ErrSecretVariableManagedByEtask
		}
		updated = append(updated, src)
	}
	return s.repo.UpdateVariables(ctx, id, updated)
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

// taskPrepareError 封装了任务准备阶段前置上下文拉取发生异常时的只读错误元数据
type taskPrepareError struct {
	triggerPosition string
	status          domain.TaskStatus
	err             error
}

func (e *taskPrepareError) Error() string {
	return e.err.Error()
}

// taskProcessContext 封装了流转任务执行时所需的完整依赖链路元数据
type taskProcessContext struct {
	workflowID int64
	automation easyflow.AutomationProperty
	runner     *runnerv1.Runner
	codebook   *codebookv1.Codebook
}

func (s *taskService) buildTaskProcessContext(ctx context.Context, task domain.Task) (*taskProcessContext, error) {
	// 1. 获取流程实例详情，拿到对应的业务流程定义 ID 和版本
	inst, err := s.engineSvc.GetInstanceByID(ctx, task.ProcessInstId)
	if err != nil {
		return nil, &taskPrepareError{
			triggerPosition: domain.TriggerPositionErrorGetProcessInst.ToString(),
			status:          domain.FAILED,
			err:             err,
		}
	}

	workflowID := task.WorkflowId
	if workflowID == 0 {
		workflowID, _ = strconv.ParseInt(inst.BusinessID, 10, 64)
	}

	// 2. 尝试获取流程定义快照
	flow, err := s.workflowSvc.FindInstanceFlow(ctx, workflowID, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return nil, &taskPrepareError{
			triggerPosition: domain.TriggerPositionErrorGetProcessInfo.ToString(),
			status:          domain.FAILED,
			err:             err,
		}
	}

	// 3. 解析该节点的自动化执行配置
	automation, err := s.workflowSvc.GetAutomationProperty(s.toEasyWorkflow(flow), task.CurrentNodeId)
	if err != nil {
		return nil, &taskPrepareError{
			triggerPosition: domain.TriggerPositionErrorExtractAutomationInfo.ToString(),
			status:          domain.FAILED,
			err:             err,
		}
	}

	// 4. 动态匹配或静态查找执行节点 Runner 实体
	s.logger.Info("准备匹配 Runner 执行器",
		elog.Int64("taskId", task.Id),
		elog.Int64("codebookId", automation.CodebookId),
		elog.String("tag", automation.Tag),
	)
	runnerResp, err := s.runnerCli.FindRunnerByCodebookIdAndTag(ctx, &runnerv1.FindRunnerByCodebookIdAndTagRequest{
		CodebookId: automation.CodebookId,
		Tag:        automation.Tag,
	})
	if err != nil {
		s.logger.Error("获取执行器 Runner 失败",
			elog.Int64("taskId", task.Id),
			elog.Int64("codebookId", automation.CodebookId),
			elog.String("tag", automation.Tag),
			elog.FieldErr(err),
		)
		return nil, &taskPrepareError{
			triggerPosition: domain.TriggerPositionErrorGetDispatcherNode.ToString(),
			status:          domain.BLOCKED,
			err:             err,
		}
	}
	runner := runnerResp.GetRunner()

	s.logger.Info("成功匹配 Runner 执行器",
		elog.Int64("taskId", task.Id),
		elog.Int64("runnerId", runner.GetId()),
		elog.String("runnerName", runner.GetName()),
		elog.String("runnerKind", runner.GetKind()),
		elog.String("runnerTarget", runner.GetTarget()),
		elog.String("runnerHandler", runner.GetHandler()),
	)

	// 5. 获取代码模板 Codebook
	codebookResp, err := s.codebookCli.GetCodebookByID(ctx, &codebookv1.GetCodebookByIDRequest{
		Id: runner.GetCodebookId(),
	})
	if err != nil {
		return nil, &taskPrepareError{
			triggerPosition: domain.TriggerPositionErrorGetTaskTemplate.ToString(),
			status:          domain.FAILED,
			err:             err,
		}
	}
	codebook := codebookResp.GetCodebook()

	return &taskProcessContext{
		workflowID: flow.Id,
		automation: automation,
		runner:     runner,
		codebook:   codebook,
	}, nil
}

func (s *taskService) prepareTask(ctx context.Context, task domain.Task, ticket domain.Ticket) (domain.Task, error) {
	// 1. 获取并聚合所有前置运行依赖的上下文（流程定义、步骤参数、调度节点与脚本模版）
	pCtx, err := s.buildTaskProcessContext(ctx, task)
	if err != nil {
		// 统一在此处处理前置数据拉取的异常状态记录
		if prepErr, ok := errors.AsType[*taskPrepareError](err); ok {
			_ = s.handleTaskError(ctx, task.Id, prepErr.triggerPosition, prepErr.status, prepErr.err)
		}
		return domain.Task{}, err
	}

	// 2. 合成包含用户凭单上下文等在运行时的最终参数
	args, err := s.assembleRuntimeArgs(ctx, ticket)
	if err != nil {
		return domain.Task{}, err
	}

	task.WorkflowId = pCtx.workflowID
	task.CodebookId = pCtx.codebook.GetId()
	task.Code = pCtx.codebook.GetCode()
	task.Language = getLanguageFromName(pCtx.codebook.GetName())
	task.Kind = domain.Kind(pCtx.runner.GetKind())
	task.Target = pCtx.runner.GetTarget()
	task.Handler = pCtx.runner.GetHandler()
	task.Variables = slice.Map(pCtx.runner.GetVariables(), func(_ int, src *runnerv1.Variable) domain.Variables {
		return domain.Variables{
			Key:    src.GetKey(),
			Value:  src.GetValue(),
			Secret: src.GetSecret(),
		}
	})

	// 合并必要的 ID 标识
	args["ticket_id"] = task.TicketID
	args["process_inst_id"] = task.ProcessInstId
	task.Args = args

	task.Status = domain.WAITING
	task.IsTiming = pCtx.automation.IsTiming
	task.ScheduledTime = s.calculateScheduledTime(pCtx.automation, task.Args)
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
	refreshed, err := s.refreshTaskSnapshot(ctx, task)
	if err != nil {
		return err
	}
	task = refreshed
	res := domain.TaskResult{Id: task.Id, TriggerPosition: domain.TriggerPositionManualRetry.ToString(), Status: domain.SCHEDULED}
	if auto {
		res.TriggerPosition = domain.TriggerPositionAutoRetry.ToString()
		res.RetryCount = 1
	}
	_, _ = s.UpdateTaskStatus(ctx, res)
	return s.dispatchTask(ctx, task)
}

func (s *taskService) refreshTaskSnapshot(ctx context.Context, task domain.Task) (domain.Task, error) {
	resp, err := s.ticketSvc.GetByID(ctx, task.TicketID)
	if err != nil {
		return domain.Task{}, err
	}
	ctx = ctxutil.WithTenantID(ctx, resp.TenantID)
	return s.prepareTask(ctx, task, resp)
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

	var unit Unit = HOUR
	var quantity int64 = 2

	// 根据执行方式解析时长单位和数值
	switch automation.ExecMethod {
	case "template":
		quantity = s.parseTemplateQuantity(automation.TemplateField, data)
	case "hand":
		unit = Unit(automation.Unit)
		if unit == 0 {
			unit = HOUR
		}
		quantity = automation.Quantity
		if quantity <= 0 {
			quantity = 2
		}
	}

	duration := s.calculateDuration(unit, quantity)
	return time.Now().Add(duration).UnixMilli()
}

// parseTemplateQuantity 尝试从动态表单上下文中提取并转换为合法的时长数量 (int64)
func (s *taskService) parseTemplateQuantity(field string, data map[string]interface{}) int64 {
	const defaultQuantity = 2

	quantityVal, exist := data[field]
	if !exist {
		s.logger.Warn("模板时长字段不存在, 赋予默认值 2 小时", elog.String("field", field))
		return defaultQuantity
	}

	switch v := quantityVal.(type) {
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case string:
		parsedQuantity, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			s.logger.Error("模板时长解析失败, 赋予默认值 2 小时", elog.FieldErr(err), elog.Any("value", v))
			return defaultQuantity
		}
		return parsedQuantity
	default:
		s.logger.Warn("模板时长类型未知, 赋予默认值 2 小时", elog.Any("type", fmt.Sprintf("%T", v)), elog.Any("value", v))
		return defaultQuantity
	}
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

func (s *taskService) assembleRuntimeArgs(ctx context.Context, ticket domain.Ticket) (map[string]interface{}, error) {
	// 获取基础表单参数和用户信息
	args, err := s.prepareUserArgs(ctx, ticket)
	if err != nil {
		return nil, err
	}

	// 获取用户审批提交的增量表单数据
	formValues, err := s.ticketSvc.ListTaskFormsByTicketID(ctx, ticket.Id)
	if err != nil {
		return nil, err
	}

	// 覆盖合并
	for _, value := range formValues {
		args[value.Key] = value.Value
	}

	return args, nil
}

func (s *taskService) prepareUserArgs(ctx context.Context, ticket domain.Ticket) (map[string]interface{}, error) {
	args := make(map[string]interface{})
	for k, v := range ticket.Data {
		args[k] = v
	}

	resp, err := s.userClient.QueryByUsername(ctx, &userv1.QueryByUsernameReq{
		Username: ticket.CreateBy,
	})
	if err != nil {
		s.logger.Error("获取用户信息失败", elog.FieldErr(err))
		return args, nil
	}
	if resp.User == nil {
		s.logger.Warn("获取用户信息为空", elog.String("username", ticket.CreateBy))
		return args, nil
	}

	userInfoJSON, _ := json.Marshal(resp.User)
	args["user_info"] = string(userInfoJSON)
	return args, nil
}

func getLanguageFromName(name string) string {
	name = strings.ToLower(name)
	if strings.HasSuffix(name, ".sh") {
		return "shell"
	}
	if strings.HasSuffix(name, ".py") {
		return "python"
	}
	return "python"
}
