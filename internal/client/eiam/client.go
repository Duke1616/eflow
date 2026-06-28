package eiam

import (
	departmentv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/department/v1"
	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	"google.golang.org/grpc"
)

// EIAMConn EIAM 专属连接通路接口
type EIAMConn interface {
	grpc.ClientConnInterface
}

// EIAMClient EIAM 专属高内聚客户端网关
type EIAMClient struct {
	UserClient       userv1.UserServiceClient
	DepartmentClient departmentv1.DepartmentServiceClient
}

// NewEIAMClient 初始化网关，使用专属 EIAMConn 接口
func NewEIAMClient(cc EIAMConn) *EIAMClient {
	return &EIAMClient{
		UserClient:       userv1.NewUserServiceClient(cc),
		DepartmentClient: departmentv1.NewDepartmentServiceClient(cc),
	}
}
