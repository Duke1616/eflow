package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// mongoVariables MongoDB 中的环境变量源数据实体
type mongoVariables struct {
	Key    string `bson:"key"`
	Value  string `bson:"value"`
	Secret bool   `bson:"secret"`
}

// mongoRunner MongoDB 中的执行器源数据实体
type mongoRunner struct {
	ID             int64            `bson:"id"`
	Name           string           `bson:"name"`
	CodebookUid    string           `bson:"codebook_uid"`
	CodebookSecret string           `bson:"codebook_secret"`
	Kind           string           `bson:"kind"`
	Target         string           `bson:"target"`
	Handler        string           `bson:"handler"`
	Tags           []string         `bson:"tags"`
	Action         uint8            `bson:"action"`
	Desc           string           `bson:"desc"`
	Variables      []mongoVariables `bson:"variables"`
	Ctime          int64            `bson:"ctime"`
	Utime          int64            `bson:"utime"`
}

type runnerMigrator struct{}

// NewRunnerMigrator 构造执行器数据的泛型迁移对拷器
func NewRunnerMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoRunner, dao.Runner](runnerMigrator{})
}

func (runnerMigrator) Name() string {
	return "runner"
}

func (runnerMigrator) CollectionName() string {
	return "c_runner"
}

func (runnerMigrator) Convert(src mongoRunner) dao.Runner {
	vars := make([]dao.Variables, 0, len(src.Variables))
	for _, v := range src.Variables {
		vars = append(vars, dao.Variables{
			Key:    v.Key,
			Value:  v.Value,
			Secret: v.Secret,
		})
	}

	return dao.Runner{
		Id:             src.ID,
		TenantID:       DefaultTenantID,
		Name:           src.Name,
		CodebookUid:    src.CodebookUid,
		CodebookSecret: src.CodebookSecret,
		Kind:           src.Kind,
		Target:         src.Target,
		Handler:        src.Handler,
		Tags:           sqlx.JsonField[[]string]{Val: src.Tags, Valid: src.Tags != nil},
		Action:         src.Action,
		Desc:           src.Desc,
		Variables:      sqlx.JsonField[[]dao.Variables]{Val: vars, Valid: len(vars) > 0},
		Ctime:          src.Ctime,
		Utime:          src.Utime,
	}
}
