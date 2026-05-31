package ecmdb

import (
	teamv1 "github.com/Duke1616/ecmdb/api/proto/gen/ealert/team"
	rotav1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/rota/v1"
	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/executor/v1"
	taskv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/task/v1"
	"google.golang.org/grpc"
)

// ECMDBConn ECMDB 专属连接通路接口
type ECMDBConn interface {
	grpc.ClientConnInterface
}

// ECMDBClient ECMDB 专属高内聚客户端网关
type ECMDBClient struct {
	TaskClient      taskv1.TaskServiceClient
	ExecutorClient  executorv1.TaskExecutionServiceClient
	TeamClient      teamv1.TeamServiceClient
	RotaClient      rotav1.OnCallServiceClient
}

// NewECMDBClient 初始化网关，使用专属 ECMDBConn 接口
func NewECMDBClient(cc ECMDBConn) *ECMDBClient {
	return &ECMDBClient{
		TaskClient:     taskv1.NewTaskServiceClient(cc),
		ExecutorClient: executorv1.NewTaskExecutionServiceClient(cc),
		TeamClient:     teamv1.NewTeamServiceClient(cc),
		RotaClient:     rotav1.NewOnCallServiceClient(cc),
	}
}
