package ioc

import (
	"time"

	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/executor/v1"
	processConsumer "github.com/Duke1616/eflow/internal/event/process"
	taskConsumer "github.com/Duke1616/eflow/internal/event/task"
	ticketConsumer "github.com/Duke1616/eflow/internal/event/ticket"
	templateConsumer "github.com/Duke1616/eflow/internal/event/template"
	"github.com/Duke1616/eflow/internal/service/engine"
	serviceTask "github.com/Duke1616/eflow/internal/service/task"
	"github.com/ecodeclub/mq-api"
)

// InitTasks 初始化所有后台任务
// NOTE: 新增后台任务时在此处注入，打通定时任务、后台作业补偿及全量大事件 Kafka 消费监听
func InitTasks(
	taskSvc serviceTask.Service,
	engineSvc engine.Service,
	executorSvc executorv1.TaskExecutionServiceClient,
	q mq.MQ,
	processConsumer *processConsumer.ProcessEventConsumer,
	wechatConsumer *ticketConsumer.WechatTicketConsumer,
	larkConsumer *ticketConsumer.LarkCallbackTicketConsumer,
	wechatCallbackConsumer *templateConsumer.WechatApprovalCallbackConsumer,
) []Task {
	consumer, err := taskConsumer.NewExecuteResultConsumer(q, taskSvc)
	if err != nil {
		panic(err)
	}

	return []Task{
		consumer,
		serviceTask.NewStartTaskJob(taskSvc, 100, 10*time.Second),
		serviceTask.NewTaskRecoveryJob(taskSvc, 100, time.Minute),
		serviceTask.NewPassProcessTaskJob(taskSvc, engineSvc, 100, 10*time.Second, 10, 0),
		serviceTask.NewTaskExecutionSyncJob(taskSvc, executorSvc, 100, 10*time.Second),
		processConsumer,
		wechatConsumer,
		larkConsumer,
		wechatCallbackConsumer,
	}
}
