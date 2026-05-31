package ioc

import (
	"github.com/Duke1616/eflow/internal/client/ecmdb"
	"github.com/Duke1616/eflow/internal/client/eiam"
	grpcpkg "github.com/Duke1616/etask/pkg/grpc"
	"github.com/Duke1616/etask/pkg/grpc/registry"
	"github.com/spf13/viper"
)

// InitECMDBGrpcClient 初始化 ECMDB gRPC 客户端
func InitECMDBGrpcClient(reg registry.Registry) ecmdb.ECMDBConn {
	var cfg grpcpkg.ClientConfig
	if err := viper.UnmarshalKey("grpc.client.ecmdb", &cfg); err != nil {
		panic(err)
	}

	cc, err := grpcpkg.NewClientConn(
		reg,
		grpcpkg.WithServiceName(cfg.Name),
		grpcpkg.WithClientJWTAuth(cfg.AuthToken),
	)
	if err != nil {
		panic(err)
	}

	return cc
}

// InitEIAMGrpcClient 初始化 EIAM gRPC 客户端
func InitEIAMGrpcClient(reg registry.Registry) eiam.EIAMConn {
	var cfg grpcpkg.ClientConfig
	if err := viper.UnmarshalKey("grpc.client.eiam", &cfg); err != nil {
		panic(err)
	}

	cc, err := grpcpkg.NewClientConn(
		reg,
		grpcpkg.WithServiceName(cfg.Name),
		grpcpkg.WithClientJWTAuth(cfg.AuthToken),
	)
	if err != nil {
		panic(err)
	}

	return cc
}
