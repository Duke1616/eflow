package etask

import (
	executorv1 "github.com/Duke1616/eflow/api/proto/gen/etask/executor/v1"
	taskv1 "github.com/Duke1616/eflow/api/proto/gen/etask/task/v1"
	"google.golang.org/grpc"
)

// ETASKConn ETASK 专属连接通路接口
type ETASKConn interface {
	grpc.ClientConnInterface
}

// ETASKClient ETASK 专属高内聚客户端网关
type ETASKClient struct {
	TaskClient     taskv1.TaskServiceClient
	ExecutorClient executorv1.TaskExecutionServiceClient
}

// NewETASKClient 初始化网关，使用专属 ETASKConn 接口
func NewETASKClient(cc ETASKConn) *ETASKClient {
	return &ETASKClient{
		TaskClient:     taskv1.NewTaskServiceClient(cc),
		ExecutorClient: executorv1.NewTaskExecutionServiceClient(cc),
	}
}
