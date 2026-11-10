package ioc

import (
	"time"

	templatev1 "github.com/Duke1616/eflow/api/proto/gen/ealert/template/v1"
	processConsumer "github.com/Duke1616/eflow/internal/event/process"
	taskConsumer "github.com/Duke1616/eflow/internal/event/task"
	templateConsumer "github.com/Duke1616/eflow/internal/event/template"
	feishuCallback "github.com/Duke1616/eflow/internal/callback/feishu"
	wechatConsumerPkg "github.com/Duke1616/eflow/internal/event/wechat"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
	serviceTask "github.com/Duke1616/eflow/internal/service/task"
	withdrawalSvc "github.com/Duke1616/eflow/internal/service/withdrawal"
	workflow "github.com/Duke1616/eflow/internal/service/workflow"
)

// InitTasks 初始化所有后台任务
// NOTE: 新增后台任务时在此处注入，打通定时任务、后台作业补偿及全量大事件 Kafka 消费监听
func InitTasks(
	taskSvc serviceTask.Service,
	withdrawalRepo repository.WithdrawalRepository,
	compensationPlanner withdrawalSvc.CompensationPlanner,
	engine engine.IEngine,
	executeResultConsumer *taskConsumer.ExecuteResultConsumer,
	processConsumer *processConsumer.ProcessEventConsumer,
	wechatConsumer *wechatConsumerPkg.WechatTicketConsumer,
	larkWsServer *feishuCallback.LarkCallbackServer,
	wechatCallbackConsumer *templateConsumer.WechatApprovalCallbackConsumer,
	templateClient templatev1.TemplateServiceClient,
) []Task {
	return []Task{
		executeResultConsumer,
		serviceTask.NewStartTaskJob(taskSvc, 100, 8, 10*time.Second, 30*time.Second),
		serviceTask.NewTaskRecoveryJob(taskSvc, 100, time.Minute, time.Minute),
		serviceTask.NewPassProcessTaskJob(taskSvc, engine, 100, 10*time.Second),
		withdrawalSvc.NewRecoveryJob(withdrawalRepo, compensationPlanner, 100, time.Minute, 5*time.Minute),
		processConsumer,
		wechatConsumer,
		larkWsServer,
		wechatCallbackConsumer,
		workflow.NewTemplateBootstrapTask(templateClient),
	}
}
