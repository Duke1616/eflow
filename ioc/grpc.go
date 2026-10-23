package ioc

import (
	ticketv1 "github.com/Duke1616/eflow/api/proto/gen/eflow/ticket/v1"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	registrysdk "github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/spf13/viper"
)

// InitGrpcServer 初始化 eflow 的 gRPC 服务器并挂载 TicketService 服务
func InitGrpcServer(
	registry registrysdk.Registry,
	ticketServer ticketv1.TicketServiceServer,
) *grpcpkg.Server {
	var cfg grpcpkg.ServerConfig
	if err := viper.UnmarshalKey("grpc.server.eflow", &cfg); err != nil {
		panic(err)
	}

	server := grpcpkg.NewServer(cfg, registry, grpcpkg.WithJWTAuth(cfg.AuthToken))

	ticketv1.RegisterTicketServiceServer(server, ticketServer)
	return server
}
