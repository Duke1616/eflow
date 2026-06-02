package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
)

// mongoCodebook MongoDB 中的脚本库源数据实体
type mongoCodebook struct {
	ID         int64  `bson:"id"`
	Name       string `bson:"name"`
	Owner      string `bson:"owner"`
	Identifier string `bson:"identifier"`
	Code       string `bson:"code"`
	Language   string `bson:"language"`
	Secret     string `bson:"secret"`
	Ctime      int64  `bson:"ctime"`
	Utime      int64  `bson:"utime"`
}

type codebookMigrator struct{}

// NewCodebookMigrator 构造脚本库的泛型迁移对拷器
func NewCodebookMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoCodebook, dao.Codebook](codebookMigrator{})
}

func (codebookMigrator) Name() string {
	return "codebook"
}

func (codebookMigrator) CollectionName() string {
	return "c_codebook"
}

func (codebookMigrator) Convert(src mongoCodebook) dao.Codebook {
	return dao.Codebook{
		Id:         src.ID,
		TenantID:   DefaultTenantID,
		Name:       src.Name,
		Owner:      src.Owner,
		Identifier: src.Identifier,
		Code:       src.Code,
		Language:   src.Language,
		Secret:     src.Secret,
		Ctime:      src.Ctime,
		Utime:      src.Utime,
	}
}
