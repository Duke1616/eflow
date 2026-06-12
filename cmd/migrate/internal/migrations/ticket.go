package migrations

import (
	"github.com/Duke1616/eiam/pkg/migration"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// mongoTicket 是 ecmdb 中 c_order 集合的历史结构。
type mongoTicket struct {
	Id                int64                         `bson:"id"`
	BizID             int64                         `bson:"biz_id"`
	Key               string                        `bson:"key"`
	TemplateId        int64                         `bson:"template_id"`
	WorkflowId        int64                         `bson:"workflow_id"`
	ProcessInstanceId int                           `bson:"process_instance_id"`
	CreateBy          string                        `bson:"create_by"`
	Provide           uint8                         `bson:"provide"`
	Data              domain.TicketData             `bson:"data"`
	Status            uint8                         `bson:"status"`
	Ctime             int64                         `bson:"ctime"`
	Wtime             int64                         `bson:"wtime"`
	Utime             int64                         `bson:"utime"`
	NotificationConf  mongoTicketNotificationConfig `bson:"notification_conf"`
}

type mongoTicketNotificationConfig struct {
	TemplateID     int64                     `bson:"template_id"`
	TemplateParams domain.NotificationParams `bson:"template_params"`
	Channel        string                    `bson:"channel"`
}

type ticketMigrator struct{}

// NewTicketMigrator 构造工单数据迁移器。
func NewTicketMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoTicket, dao.Ticket](ticketMigrator{})
}

func (ticketMigrator) Name() string {
	return "ticket"
}

func (ticketMigrator) CollectionName() string {
	return "c_order"
}

func (ticketMigrator) Convert(src mongoTicket) dao.Ticket {
	return dao.Ticket{
		Id:                src.Id,
		TenantID:          DefaultTenantID,
		BizID:             src.BizID,
		Key:               src.Key,
		TemplateId:        src.TemplateId,
		WorkflowId:        src.WorkflowId,
		ProcessInstanceId: src.ProcessInstanceId,
		CreateBy:          src.CreateBy,
		Provide:           src.Provide,
		Data:              sqlx.JsonField[domain.TicketData]{Val: src.Data, Valid: src.Data != nil},
		Status:            src.Status,
		NotificationConf: sqlx.JsonField[dao.NotificationConf]{
			Val: dao.NotificationConf{
				TemplateID:     src.NotificationConf.TemplateID,
				TemplateParams: src.NotificationConf.TemplateParams,
				Channel:        src.NotificationConf.Channel,
			},
			Valid: true,
		},
		Ctime: src.Ctime,
		Wtime: src.Wtime,
		Utime: src.Utime,
	}
}
