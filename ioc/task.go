package ioc

import (
	"time"

	serviceTask "github.com/Duke1616/eflow/internal/service/task"
	"github.com/ecodeclub/mq-api"
)

// InitTasks 初始化所有后台任务
// NOTE: 新增后台任务（补偿器、消费者等）时在此处注入
func InitTasks(taskSvc serviceTask.Task, q mq.MQ) []Task {
	consumer, err := serviceTask.NewExecuteResultConsumer(q, taskSvc)
	if err != nil {
		panic(err)
	}
	return []Task{
		consumer,
		serviceTask.NewStartTaskJob(taskSvc, 100, 10*time.Second),
		serviceTask.NewTaskRecoveryJob(taskSvc, 100, time.Minute),
	}
}
