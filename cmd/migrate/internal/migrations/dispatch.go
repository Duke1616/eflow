package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
)

// mongoDiscovery 是 ecmdb 中 c_discovery 集合的历史结构。
type mongoDiscovery struct {
	Id         int64  `bson:"id"`
	TemplateId int64  `bson:"template_id"`
	RunnerId   int64  `bson:"runner_id"`
	Field      string `bson:"field"`
	Value      string `bson:"value"`
	Ctime      int64  `bson:"ctime"`
	Utime      int64  `bson:"utime"`
}

type dispatchMigrator struct{}

// NewDispatchMigrator 构造自动派发规则迁移器。
func NewDispatchMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoDiscovery, dao.Dispatch](dispatchMigrator{})
}

func (dispatchMigrator) Name() string {
	return "dispatch"
}

func (dispatchMigrator) CollectionName() string {
	return "c_discovery"
}

func (dispatchMigrator) Convert(src mongoDiscovery) dao.Dispatch {
	return dao.Dispatch{
		Id:         src.Id,
		TenantID:   DefaultTenantID,
		TemplateId: src.TemplateId,
		RunnerId:   src.RunnerId,
		Field:      src.Field,
		Value:      src.Value,
		Ctime:      src.Ctime,
		Utime:      src.Utime,
	}
}
