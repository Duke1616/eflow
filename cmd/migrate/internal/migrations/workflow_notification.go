package migrations

import (
	"github.com/Duke1616/eiam/pkg/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
)

// mongoNotifyBinding 是 ecmdb 中 c_workflow_notify_binding 集合的历史结构。
type mongoNotifyBinding struct {
	Id         int64  `bson:"id"`
	WorkflowId int64  `bson:"workflow_id"`
	NotifyType string `bson:"notify_type"`
	Channel    string `bson:"channel"`
	TemplateId int64  `bson:"template_id"`
	Ctime      int64  `bson:"ctime"`
	Utime      int64  `bson:"utime"`
}

type workflowNotificationMigrator struct{}

// NewWorkflowNotificationMigrator 构造流程通知绑定迁移器。
func NewWorkflowNotificationMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoNotifyBinding, dao.NotifyBinding](workflowNotificationMigrator{})
}

func (workflowNotificationMigrator) Name() string {
	return "workflow_notification"
}

func (workflowNotificationMigrator) CollectionName() string {
	return "c_workflow_notify_binding"
}

func (workflowNotificationMigrator) Convert(src mongoNotifyBinding) dao.NotifyBinding {
	return dao.NotifyBinding{
		Id:         src.Id,
		TenantID:   DefaultTenantID,
		WorkflowId: src.WorkflowId,
		NotifyType: src.NotifyType,
		Channel:    src.Channel,
		TemplateId: src.TemplateId,
		Ctime:      src.Ctime,
		Utime:      src.Utime,
	}
}
