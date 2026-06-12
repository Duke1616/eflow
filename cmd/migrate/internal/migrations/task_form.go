package migrations

import (
	"github.com/Duke1616/eiam/pkg/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// mongoTaskForm 是 ecmdb 中 c_order_task_forms 集合的历史结构。
type mongoTaskForm struct {
	Id      int64       `bson:"id"`
	OrderId int64       `bson:"order_id"`
	TaskId  int         `bson:"task_id"`
	Name    string      `bson:"name"`
	Key     string      `bson:"key"`
	Type    string      `bson:"type"`
	Value   interface{} `bson:"value"`
	Ctime   int64       `bson:"ctime"`
}

type taskFormMigrator struct{}

// NewTaskFormMigrator 构造审批节点表单快照迁移器。
func NewTaskFormMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoTaskForm, dao.TaskForm](taskFormMigrator{})
}

func (taskFormMigrator) Name() string {
	return "task_form"
}

func (taskFormMigrator) CollectionName() string {
	return "c_order_task_forms"
}

func (taskFormMigrator) Convert(src mongoTaskForm) dao.TaskForm {
	return dao.TaskForm{
		Id:       src.Id,
		TenantID: DefaultTenantID,
		TicketId: src.OrderId,
		TaskId:   src.TaskId,
		Name:     src.Name,
		Key:      src.Key,
		Type:     src.Type,
		Value:    sqlx.JsonField[interface{}]{Val: src.Value, Valid: src.Value != nil},
		Ctime:    src.Ctime,
	}
}
