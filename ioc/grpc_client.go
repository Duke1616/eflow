package ioc

import (
	endpointv1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/endpoint/v1"
	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/executor/v1"
	taskv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/task/v1"
	"github.com/spf13/viper"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InitECMDBGrpcClient 初始化 ECMDB gRPC 客户端
func InitECMDBGrpcClient() grpc.ClientConnInterface {
	type ClientConfig struct {
		Name      string `mapstructure:"name"`
		AuthToken string `mapstructure:"auth_token"`
	}

	var cfg ClientConfig
	if err := viper.UnmarshalKey("grpc.client.ecmdb", &cfg); err != nil {
		panic(err)
	}

	// TODO: 接入服务发现后替换为动态解析
	cc, err := grpc.NewClient(
		cfg.Name,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		panic(err)
	}

	return cc
}

// InitEndpointServiceClient 初始化 Endpoint 服务客户端
func InitEndpointServiceClient(cc grpc.ClientConnInterface) endpointv1.EndpointServiceClient {
	return endpointv1.NewEndpointServiceClient(cc)
}

func InitTaskServiceClient(cc grpc.ClientConnInterface) taskv1.TaskServiceClient {
	return taskv1.NewTaskServiceClient(cc)
}

func InitTaskExecutionServiceClient(cc grpc.ClientConnInterface) executorv1.TaskExecutionServiceClient {
	return executorv1.NewTaskExecutionServiceClient(cc)
}
