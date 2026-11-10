package etask

import (
	executorv1 "github.com/Duke1616/eflow/api/proto/gen/etask/executor/v1"
	runnerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/runner/v1"
	schedulerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/scheduler/v1"
	"google.golang.org/grpc"
)

// ETASKConn ETASK 专属连接通路接口
type ETASKConn interface {
	grpc.ClientConnInterface
}

// ETASKClient ETASK 专属高内聚客户端网关
type ETASKClient struct {
	ExecutorClient  executorv1.TaskExecutionServiceClient
	RunnerClient    runnerv1.RunnerServiceClient
	SchedulerClient schedulerv1.SchedulerServiceClient
}

// NewETASKClient 初始化网关，使用专属 ETASKConn 接口
func NewETASKClient(cc ETASKConn) *ETASKClient {
	return &ETASKClient{
		ExecutorClient:  executorv1.NewTaskExecutionServiceClient(cc),
		RunnerClient:    runnerv1.NewRunnerServiceClient(cc),
		SchedulerClient: schedulerv1.NewSchedulerServiceClient(cc),
	}
}

// Client 统一汇聚 eflow 对 etask 系统的执行调度、日志查询与执行单元编排能力。
type Client interface {
	RunnerCatalog
	TaskDispatcher
	ExecutionReader
}

type compositeClient struct {
	RunnerCatalog
	TaskDispatcher
	ExecutionReader
}

// NewClient 创建统一的 etask 客户端门面，消减分散依赖。
func NewClient(gateway *ETASKClient) Client {
	return &compositeClient{
		RunnerCatalog:   NewRunnerCatalog(gateway),
		TaskDispatcher:  NewTaskDispatcher(gateway),
		ExecutionReader: NewExecutionReader(gateway),
	}
}

