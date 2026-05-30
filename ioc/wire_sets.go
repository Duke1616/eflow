package ioc

import (
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/repository/dao"
	codebookSvc "github.com/Duke1616/eflow/internal/service/codebook"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	runnerSvc "github.com/Duke1616/eflow/internal/service/runner"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/internal/web/codebook"
	"github.com/Duke1616/eflow/internal/web/runner"
	"github.com/Duke1616/eflow/internal/web/task"
	"github.com/Duke1616/eflow/internal/web/template"
	"github.com/Duke1616/eflow/internal/web/workflow"
	"github.com/Duke1616/eflow/pkg/easyflow"
	"github.com/google/wire"
)

var (
	// BaseSet 基础设施 Provider 集合
	BaseSet = wire.NewSet(
		InitEtcdClient,
		InitMQ,
	)

	// TemplateSet 工单模板模块的 Provider 集合
	TemplateSet = wire.NewSet(
		InitWorkWx,
		dao.NewTemplateDAO,
		repository.NewTemplateRepository,
		templateSvc.NewTemplateService,
		template.NewHandler,
	)

	// WorkflowSet 工作流模块的 Provider 集合
	WorkflowSet = wire.NewSet(
		dao.NewWorkflowDAO,
		repository.NewWorkflowRepository,
		workflowSvc.NewWorkflowService,
		workflow.NewHandler,
		easyflow.NewLogicFlowToEngineConvert,
	)

	// EngineSet 流程引擎核心模块的 Provider 集合
	EngineSet = wire.NewSet(
		dao.NewProcessEngineDAO,
		repository.NewProcessEngineRepository,
		engineSvc.NewEngineService,
	)

	// CodebookSet 自动化脚本库模块 Provider 集合
	CodebookSet = wire.NewSet(
		dao.NewCodebookDAO,
		repository.NewCodebookRepository,
		codebookSvc.NewService,
		codebook.NewHandler,
	)

	// RunnerSet 自动化执行器模块 Provider 集合
	RunnerSet = wire.NewSet(
		dao.NewRunnerDAO,
		repository.NewRunnerRepository,
		runnerSvc.NewRunnerService,
		runner.NewHandler,
	)

	// TaskSet 自动化任务模块 Provider 集合
	TaskSet = wire.NewSet(
		InitTaskServiceClient,
		dao.NewTaskDAO,
		repository.NewTaskRepository,
		taskSvc.NewTaskService,
		task.NewHandler,
	)

	// WebSet Web 服务 Provider 集合
	WebSet = wire.NewSet(
		InitDB,
		InitECMDBGrpcClient,
		InitEndpointServiceClient,
		InitPolicySDK,
		InitPermSyncer,
		InitProviders,
		InitListener,
		InitGinMiddlewares,
		InitGinWebServer,
		InitTasks,
		BaseSet,
		TemplateSet,
		WorkflowSet,
		EngineSet,
		CodebookSet,
		RunnerSet,
		TaskSet,
	)
)
