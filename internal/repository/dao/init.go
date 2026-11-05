package dao

import (
	flowxMysql "github.com/Duke1616/eflow/pkg/flowx/storage/mysql"
	"gorm.io/gorm"
)

// InitTables 自动迁移数据库表结构
// 新增实体时在此处添加对应的 DAO 结构体
func InitTables(db *gorm.DB) error {
	return db.AutoMigrate(
		&TemplateGroup{},
		&Template{},
		&TemplateFavorite{},
		&Workflow{},
		&Snapshot{},
		&Ticket{},
		&TicketRating{},
		&TaskForm{},
		&Task{},
		&TaskAttempt{},
		&Dispatch{},
		// flowx 核心持久化表结构
		&flowxMysql.FlowDefinitionPO{},
		&flowxMysql.FlowInstancePO{},
		&flowxMysql.FlowTaskPO{},
	)
}
