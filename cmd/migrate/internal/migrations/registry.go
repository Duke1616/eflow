package migrations

import "github.com/Duke1616/eflow/cmd/migrate/internal/migration"

// All 按外键和业务依赖顺序返回所有迁移任务。
func All() []migration.Migrator {
	migrators := []migration.Migrator{
		NewCodebookMigrator(),
		NewRunnerMigrator(),
		NewTemplateGroupMigrator(),
		NewTemplateMigrator(),
		NewWorkflowMigrator(),
		NewWorkflowNotificationMigrator(),
		NewWorkflowInstanceFlowMigrator(),
		NewTicketMigrator(),
		NewTaskMigrator(),
		NewTaskFormMigrator(),
	}
	migrators = append(migrators, NewEasyflowMigrators()...)
	return migrators
}
