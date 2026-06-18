package dispatch

import (
	"context"

	taskv1 "github.com/Duke1616/eflow/api/proto/gen/etask/task/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/ecodeclub/mq-api"
)

// TaskDispatcher 自动化任务派发分发器接口
type TaskDispatcher interface {
	// Dispatch 分发并派发特定的任务
	Dispatch(ctx context.Context, task domain.Task) error
}

type taskDispatcher struct {
	kafkaSvc   TaskDispatcher
	executeSvc TaskDispatcher
}

// NewTaskDispatcher 实例化并组合任务统一分发器网关
func NewTaskDispatcher(q mq.MQ, grpcClient taskv1.TaskServiceClient,
	repo repository.TaskRepository) TaskDispatcher {
	return &taskDispatcher{
		kafkaSvc:   NewKafkaService(q),
		executeSvc: NewExecuteService(grpcClient, repo),
	}
}

// Dispatch 根据执行渠道自动执行 GRPC 外部系统调用或 Kafka 消息管道发布
func (d *taskDispatcher) Dispatch(ctx context.Context, task domain.Task) error {
	if task.Kind == domain.GRPC {
		return d.executeSvc.Dispatch(ctx, task)
	}

	return d.kafkaSvc.Dispatch(ctx, task)
}
