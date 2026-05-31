package ioc

import (
	"log"
	"sync"

	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/internal/event/process"
	templateEvent "github.com/Duke1616/eflow/internal/event/template"
	ticketEvent "github.com/Duke1616/eflow/internal/event/ticket"
	eflowEasyflow "github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/pkg/resolve"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/repository/dao"
	codebookSvc "github.com/Duke1616/eflow/internal/service/codebook"
	departmentSvc "github.com/Duke1616/eflow/internal/service/department"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eflow/internal/service/event/assignees"
	"github.com/Duke1616/eflow/internal/service/event/easyflow"
	"github.com/Duke1616/eflow/internal/service/event/strategy"
	"github.com/Duke1616/eflow/internal/service/event/strategy/automation"
	"github.com/Duke1616/eflow/internal/service/event/strategy/carbon_copy"
	"github.com/Duke1616/eflow/internal/service/event/strategy/chat"
	"github.com/Duke1616/eflow/internal/service/event/strategy/start"
	userstrategy "github.com/Duke1616/eflow/internal/service/event/strategy/user"
	runnerSvc "github.com/Duke1616/eflow/internal/service/runner"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/internal/web/codebook"
	"github.com/Duke1616/eflow/internal/web/runner"
	"github.com/Duke1616/eflow/internal/web/task"
	"github.com/Duke1616/eflow/internal/web/template"
	"github.com/Duke1616/eflow/internal/web/ticket"
	"github.com/Duke1616/eflow/internal/web/workflow"
	"github.com/Duke1616/eflow/internal/client/ecmdb"
	"github.com/Duke1616/eflow/internal/client/eiam"
	"github.com/ecodeclub/mq-api"
	"github.com/google/wire"
	"gorm.io/gorm"
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
		eflowEasyflow.NewLogicFlowToEngineConvert,
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
		dao.NewTaskDAO,
		repository.NewTaskRepository,
		taskSvc.NewTaskService,
		task.NewHandler,
	)

	// TicketSet 工单核心模块的 Provider 集合
	TicketSet = wire.NewSet(
		dao.NewTicketDAO,
		dao.NewTaskFormDAO,
		repository.NewTicketRepository,
		ticketSvc.NewService,
		ticket.NewHandler,
	)

	// EventSet 流程事件模块的 Provider 集合
	EventSet = wire.NewSet(
		// 部门持久层依赖
		dao.NewDepartmentDAO,
		repository.NewDepartmentRepository,
		departmentSvc.NewService,

		// 7个审批人解析器
		assignees.NewAppointResolver,
		assignees.NewFounderResolver,
		assignees.NewLeaderResolver,
		assignees.NewMainLeaderResolver,
		assignees.NewOnCallResolver,
		assignees.NewTeamResolver,
		assignees.NewTemplateResolver,

		// 审批人规则解析引擎
		InitResolveEngine,

		// 5个通知推送具体节点策略
		strategy.NewService,
		automation.NewNotification,
		carbon_copy.NewNotification,
		chat.NewNotification,
		start.NewNotification,
		userstrategy.NewNotification,

		// 消息通知分发器
		InitSendStrategy,

		// 流程状态与核心反射事件处理器/Kafka 消费者/三方回调/审批模板自愈
		InitLarkClient,
		ticketEvent.NewUserServiceAdapter,
		ticketEvent.NewWechatTicketConsumer,
		ticketEvent.NewLarkCallbackHandler,
		ticketEvent.NewLarkCallbackTicketServer,
		templateEvent.NewWechatTicketEventProducer,
		templateEvent.NewWechatApprovalCallbackConsumer,
		InitOrderStatusModifyEventProducer,
		InitWorkflowEngineOnce,
		process.NewProcessEventConsumer,
	)

	grpcSet = wire.NewSet(
		InitRegistry,
		InitEIAMGrpcClient,
		InitECMDBGrpcClient,

		// 引入本地客户端网关
		ecmdb.NewECMDBClient,
		eiam.NewEIAMClient,

		// 导出具体的 Client，直接满足底层 Service 的注入需求！
		wire.FieldsOf(new(*ecmdb.ECMDBClient), "TaskClient", "ExecutorClient", "TeamClient", "RotaClient"),
		wire.FieldsOf(new(*eiam.EIAMClient), "UserClient"),
	)
	// WebSet Web 服务 Provider 集合
	WebSet = wire.NewSet(
		InitDB,
		InitTicketEventProducer,
		InitPolicySDK,
		InitPermSyncer,
		InitProviders,
		InitListener,
		InitGinMiddlewares,
		InitGinWebServer,
		InitTasks,
		grpcSet,
		BaseSet,
		TemplateSet,
		WorkflowSet,
		EngineSet,
		CodebookSet,
		RunnerSet,
		TaskSet,
		TicketSet,
		EventSet,
	)
)

func InitTicketEventProducer(q mq.MQ) (ticketSvc.TicketEventProducer, error) {
	return ticketEvent.NewTicketEventProducer(q)
}

func InitOrderStatusModifyEventProducer(q mq.MQ) (process.OrderStatusModifyEventProducer, error) {
	return process.NewOrderStatusModifyEventProducer(q)
}

var engineOnce = sync.Once{}

func InitWorkflowEngineOnce(db *gorm.DB, engineSvc engineSvc.Service, producer process.OrderStatusModifyEventProducer,
	taskSvc taskSvc.Service, ticketSvc ticketSvc.Service, workflowSvc workflowSvc.Service,
	strategy strategy.SendStrategy) *easyflow.ProcessEvent {
	event := easyflow.NewProcessEvent(producer, engineSvc, taskSvc, ticketSvc, workflowSvc, strategy)

	engineOnce.Do(func() {
		easyEngine.DB = db
		if err := easyEngine.DatabaseInitialize(); err != nil {
			log.Fatalln("easy workflow 初始化数据表失败，错误:", err)
		}
		// 是否忽略事件错误
		easyEngine.IgnoreEventError = false
	})

	return event
}

// InitSendStrategy 包装并实例化节点消息发送分发器
func InitSendStrategy(
	base strategy.Service,
	user *userstrategy.Notification,
	auto *automation.Notification,
	startSvc *start.Notification,
	chatSvc *chat.Notification,
	carbonCopy *carbon_copy.Notification,
) strategy.SendStrategy {
	return strategy.NewDispatcher(user, auto, startSvc, chatSvc, carbonCopy, base)
}

// InitResolveEngine 并发规则解析引擎及七大审批解析器自动注册
func InitResolveEngine(
	appoint *assignees.AppointResolver,
	founder *assignees.FounderResolver,
	leader *assignees.LeaderResolver,
	mainLeader *assignees.MainLeaderResolver,
	onCall *assignees.OnCallResolver,
	team *assignees.TeamResolver,
	template *assignees.TemplateResolver,
) resolve.Engine {
	engine := resolve.NewEngine()
	engine.Register(
		appoint,
		founder,
		leader,
		mainLeader,
		onCall,
		team,
		template,
	)
	return engine
}
