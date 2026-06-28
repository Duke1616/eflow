package dao

import "gorm.io/gorm"

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
		&TaskForm{},
		&Task{},
		&Dispatch{},
	)
}
