package ioc

import (
	"github.com/Duke1616/eflow/internal/client/ealert"
	"github.com/Duke1616/eflow/internal/client/ecmdb"
	"github.com/Duke1616/eflow/internal/client/eiam"
	"github.com/Duke1616/eflow/internal/client/etask"
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

// InitEALERTGrpcClient 初始化 EALERT gRPC 客户端
func InitEALERTGrpcClient(reg registry.Registry) ealert.EALERTConn {
	var cfg grpcpkg.ClientConfig
	if err := viper.UnmarshalKey("grpc.client.ealert", &cfg); err != nil {
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

// InitETASKGrpcClient 初始化 ETASK gRPC 客户端
func InitETASKGrpcClient(reg registry.Registry) etask.ETASKConn {
	var cfg grpcpkg.ClientConfig
	if err := viper.UnmarshalKey("grpc.client.etask", &cfg); err != nil {
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
