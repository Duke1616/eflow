package migrations

import (
	"github.com/Bunny3th/easy-workflow/workflow/database"
	"github.com/Duke1616/eiam/pkg/migration"
)

type easyflowTableMigrator[T any] struct {
	name  string
	model any
}

func newEasyflowTableMigrator[T any](name string, model any) migration.Migrator {
	return migration.NewMySQLMigrator[T](easyflowTableMigrator[T]{
		name:  name,
		model: model,
	})
}

func (m easyflowTableMigrator[T]) Name() string {
	return m.name
}

func (m easyflowTableMigrator[T]) Source() any {
	return m.model
}

func (m easyflowTableMigrator[T]) Destination() any {
	return m.model
}

// NewEasyflowMigrators 返回 easyflow 流程引擎自带表的 MySQL -> MySQL 1:1 迁移任务。
func NewEasyflowMigrators() []migration.Migrator {
	return []migration.Migrator{
		newEasyflowTableMigrator[database.ProcDef]("easyflow_proc_def", &database.ProcDef{}),
		newEasyflowTableMigrator[database.HistProcDef]("easyflow_hist_proc_def", &database.HistProcDef{}),
		newEasyflowTableMigrator[database.ProcInst]("easyflow_proc_inst", &database.ProcInst{}),
		newEasyflowTableMigrator[database.HistProcInst]("easyflow_hist_proc_inst", &database.HistProcInst{}),
		newEasyflowTableMigrator[database.ProcTask]("easyflow_proc_task", &database.ProcTask{}),
		newEasyflowTableMigrator[database.HistProcTask]("easyflow_hist_proc_task", &database.HistProcTask{}),
		newEasyflowTableMigrator[database.ProcExecution]("easyflow_proc_execution", &database.ProcExecution{}),
		newEasyflowTableMigrator[database.HistProcExecution]("easyflow_hist_proc_execution", &database.HistProcExecution{}),
		newEasyflowTableMigrator[database.ProcInstVariable]("easyflow_proc_inst_variable", &database.ProcInstVariable{}),
		newEasyflowTableMigrator[database.HistProcInstVariable]("easyflow_hist_proc_inst_variable", &database.HistProcInstVariable{}),
	}
}
