package ealert

import (
	notificationv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/notification/v1"
	oncallv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/oncall/v1"
	teamv1 "github.com/Duke1616/eflow/api/proto/gen/ealert/team"
	templatev1 "github.com/Duke1616/eflow/api/proto/gen/ealert/template/v1"
	"google.golang.org/grpc"
)

// EALERTConn EALERT 专属连接通路接口
type EALERTConn interface {
	grpc.ClientConnInterface
}

// EALERTClient EALERT 专属高内聚客户端网关
type EALERTClient struct {
	TeamClient         teamv1.TeamServiceClient
	NotificationClient notificationv1.NotificationServiceClient
	TemplateClient     templatev1.TemplateServiceClient
	OnCallClient       oncallv1.OnCallServiceClient
}

// NewEALERTClient 初始化网关，使用专属 EALERTConn 接口
func NewEALERTClient(cc EALERTConn) *EALERTClient {
	return &EALERTClient{
		TeamClient:         teamv1.NewTeamServiceClient(cc),
		NotificationClient: notificationv1.NewNotificationServiceClient(cc),
		TemplateClient:     templatev1.NewTemplateServiceClient(cc),
		OnCallClient:       oncallv1.NewOnCallServiceClient(cc),
	}
}
