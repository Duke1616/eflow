package ioc

import (
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/internal/service"
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
		service.NewTemplateService,
		template.NewHandler,
	)

	// WorkflowSet 工作流模块的 Provider 集合
	WorkflowSet = wire.NewSet(
		dao.NewWorkflowDAO,
		repository.NewWorkflowRepository,
		service.NewWorkflowService,
		workflow.NewHandler,
		easyflow.NewLogicFlowToEngineConvert,
	)

	// EngineSet 流程引擎核心模块的 Provider 集合
	EngineSet = wire.NewSet(
		dao.NewProcessEngineDAO,
		repository.NewProcessEngineRepository,
		service.NewEngineService,
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
		TemplateSet,
		WorkflowSet,
		EngineSet,
	)
)


