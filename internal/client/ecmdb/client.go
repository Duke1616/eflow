package ecmdb

import (
	rotav1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/rota/v1"
	"google.golang.org/grpc"
)

// ECMDBConn ECMDB 专属连接通路接口
type ECMDBConn interface {
	grpc.ClientConnInterface
}

// ECMDBClient ECMDB 专属高内聚客户端网关
type ECMDBClient struct {
	RotaClient rotav1.OnCallServiceClient
}

// NewECMDBClient 初始化网关，使用专属 ECMDBConn 接口
func NewECMDBClient(cc ECMDBConn) *ECMDBClient {
	return &ECMDBClient{
		RotaClient: rotav1.NewOnCallServiceClient(cc),
	}
}
