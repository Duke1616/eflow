package ioc

import (
	"log"
	"sync"

	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	feishuCallback "github.com/Duke1616/eflow/internal/callback/feishu"
	"github.com/Duke1616/eflow/internal/client/ealert"
	"github.com/Duke1616/eflow/internal/client/eiam"
	"github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/event"
	"github.com/Duke1616/eflow/internal/grpc"
	"github.com/Duke1616/eflow/internal/pkg/compiler"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/repository/dao"
	channelSvc "github.com/Duke1616/eflow/internal/service/channel"
	dispatchSvc "github.com/Duke1616/eflow/internal/service/dispatch"
	easyflowDrv "github.com/Duke1616/eflow/internal/service/engine/easyflow"
	"github.com/Duke1616/eflow/internal/service/engine/easyflow/hook"
	flowxHook "github.com/Duke1616/eflow/internal/service/engine/flowx/hook"
	"github.com/Duke1616/eflow/internal/service/notification"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	"github.com/Duke1616/eflow/internal/service/notification/strategy/automation"
	"github.com/Duke1616/eflow/internal/service/notification/strategy/carbon_copy"
	"github.com/Duke1616/eflow/internal/service/notification/strategy/chat"
	"github.com/Duke1616/eflow/internal/service/notification/strategy/start"
	userstrategy "github.com/Duke1616/eflow/internal/service/notification/strategy/user"
	ratingSvc "github.com/Duke1616/eflow/internal/service/rating"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	dispatchHdl "github.com/Duke1616/eflow/internal/web/dispatch"
	"github.com/Duke1616/eflow/internal/web/task"
	"github.com/Duke1616/eflow/internal/web/template"
	"github.com/Duke1616/eflow/internal/web/ticket"
	"github.com/Duke1616/eflow/internal/web/workflow"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/mqx"
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
		workflowSvc.NewTemplateSyncer,
		workflow.NewHandler,
		compiler.NewDefaultCompositeCompiler,
	)

	// EngineSet 流程引擎核心模块的 Provider 集合
	EngineSet = wire.NewSet(
		dao.NewProcessEngineDAO,
		repository.NewProcessEngineRepository,
		easyflowDrv.NewService,
		InitFlowxEngine,
		InitEngine,
	)

	// TaskSet 自动化任务模块 Provider 集合
	TaskSet = wire.NewSet(
		dao.NewTaskDAO,
		dao.NewTaskAttemptDAO,
		repository.NewTaskRepository,
		repository.NewTaskAttemptRepository,
		taskSvc.NewTaskService,
		task.NewHandler,
	)

	WithdrawalSet = wire.NewSet(
		dao.NewWithdrawalDAO,
		repository.NewWithdrawalRepository,
		withdrawalSvc.NewCompensationPlanner,
		withdrawalSvc.NewService,
	)

	// TicketSet 工单核心模块的 Provider 集合
	TicketSet = wire.NewSet(
		dao.NewTicketDAO,
		dao.NewTaskFormDAO,
		dao.NewTicketRatingDAO,
		repository.NewTicketRepository,
		repository.NewTicketRatingRepository,
		ticketSvc.NewService,
		ticketSvc.NewQueryService,
		ratingSvc.NewService,
		ticket.NewHandler,
	)

	// DispatchSet 执行单元路由模块的 Provider 集合。
	DispatchSet = wire.NewSet(
		dao.NewDispatchDAO,
		repository.NewDispatchRepository,
		dispatchSvc.NewTemplateReader,
		dispatchSvc.NewService,
		dispatchHdl.NewHandler,
	)

	// EventSet 流程事件模块的 Provider 集合
	EventSet = wire.NewSet(
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
		eiam.NewUserClient,
		channelSvc.NewChannelIdentityResolver,
		channelSvc.NewChannelActionService,
		InitWechatTicketConsumer,
		feishuCallback.NewLarkCallbackHandler,
		InitLarkDispatcher,
		InitLarkServer,
		InitWechatTicketEventProducer,
		InitWechatApprovalCallbackConsumer,
		InitTicketStatusModifyEventProducer,
		InitWorkflowEngineOnce,
		InitProcessEventConsumer,
		InitExecuteResultConsumer,

		// 初始化通知发送器
		InitNotificationSender,
	)

	grpcSet = wire.NewSet(
		InitRegistry,
		InitEIAMGrpcClient,
		InitEALERTGrpcClient,
		InitETASKGrpcClient,

		// 引入本地客户端网关
		eiam.NewEIAMClient,
		ealert.NewEALERTClient,
		etask.NewETASKClient,
		etask.NewClient,

		// 导出具体的 Client，直接满足底层 Service 的注入需求！
		wire.FieldsOf(new(*eiam.EIAMClient), "UserClient", "DepartmentClient"),
		wire.FieldsOf(new(*ealert.EALERTClient), "TeamClient", "NotificationClient", "TemplateClient", "OnCallClient"),
	)
	// GrpcSet gRPC 内部服务端点装配集合
	GrpcSet = wire.NewSet(
		grpc.NewTicketServer,
		InitGrpcServer,
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
		GrpcSet,
		grpcSet,
		BaseSet,
		TemplateSet,
		WorkflowSet,
		EngineSet,
		TaskSet,
		TicketSet,
		WithdrawalSet,
		EventSet,
		DispatchSet,
	)
)

var engineOnce = sync.Once{}

func InitWorkflowEngineOnce(db *gorm.DB, engineSvc easyflowDrv.Service, flowxEngine flowx.Engine, ticketRepo repository.TicketRepository, repo repository.IEngineRepository, producer mqx.Producer[event.TicketStatusModifyEvent],
	taskSvc taskSvc.Service, ticketSvc ticketSvc.Service, workflowSvc workflowSvc.Service,
	sendStrategy strategy.SendStrategy) *hook.ProcessEvent {
	hookEvent := hook.NewProcessEvent(producer, engineSvc, taskSvc, ticketSvc, workflowSvc, sendStrategy, repo)

	engineOnce.Do(func() {
		easyEngine.DB = db
		if err := easyEngine.DatabaseInitialize(); err != nil {
			log.Fatalln("easy workflow 初始化数据表失败，错误:", err)
		}
		// 是否忽略事件错误
		easyEngine.IgnoreEventError = false

		// 挂载 flowx 专属飞书通知监听器，实现与 easyflow 等价的消息广播与生命周期协同
		if dispatcher, ok := sendStrategy.(*strategy.Dispatcher); ok {
			flowxEngine.AddListener(notification.NewFlowxNotificationListener(ticketRepo, workflowSvc, dispatcher))
		}

		// 挂载 flowx 自动化任务执行器，将 flowx 引擎触发的自动化节点对接到统一自动化任务编排服务
		flowxHook.RegisterAutomationWorker(flowxEngine, taskSvc)
	})

	return hookEvent
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
