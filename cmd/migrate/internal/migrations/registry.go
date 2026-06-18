package migrations

import "github.com/Duke1616/eiam/pkg/migration"

const DefaultTenantID int64 = 2

// All 按外键和业务依赖顺序返回所有迁移任务。
func All() []migration.Migrator {
	migrators := []migration.Migrator{
		NewTemplateGroupMigrator(),
		NewTemplateMigrator(),
		NewWorkflowMigrator(),
		NewWorkflowNotificationMigrator(),
		NewWorkflowInstanceFlowMigrator(),
		NewTicketMigrator(),
		NewTaskMigrator(),
		NewTaskFormMigrator(),
		NewDispatchMigrator(),
	}
	migrators = append(migrators, NewEasyflowMigrators()...)
	return migrators
}
