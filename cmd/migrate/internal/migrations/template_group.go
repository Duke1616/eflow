package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
)

// mongoTemplateGroup 是 ecmdb 中 c_template_group 集合的历史结构。
type mongoTemplateGroup struct {
	Id    int64  `bson:"id"`
	Name  string `bson:"name"`
	Icon  string `bson:"icon"`
	Ctime int64  `bson:"ctime"`
	Utime int64  `bson:"utime"`
}

type templateGroupMigrator struct{}

// NewTemplateGroupMigrator 构造模板分组迁移器。
func NewTemplateGroupMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoTemplateGroup, dao.TemplateGroup](templateGroupMigrator{})
}

func (templateGroupMigrator) Name() string {
	return "template_group"
}

func (templateGroupMigrator) CollectionName() string {
	return "c_template_group"
}

func (templateGroupMigrator) Convert(src mongoTemplateGroup) dao.TemplateGroup {
	return dao.TemplateGroup{
		Id:    src.Id,
		Name:  src.Name,
		Icon:  src.Icon,
		Ctime: src.Ctime,
		Utime: src.Utime,
	}
}
