package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/xen0n/go-workwx"
)

// mongoTemplate 是 ecmdb 中 c_template 集合的历史结构。
type mongoTemplate struct {
	Id                 int64                     `bson:"id"`
	Name               string                    `bson:"name"`
	WorkflowId         int64                     `bson:"workflow_id"`
	GroupId            int64                     `bson:"group_id"`
	Icon               string                    `bson:"icon"`
	CreateType         uint8                     `bson:"create_type"`
	Rules              []dao.Rule                `bson:"rules"`
	Options            dao.TemplateOptions       `bson:"options"`
	ExternalTemplateId string                    `bson:"external_template_id"`
	UniqueHash         string                    `bson:"unique_hash"`
	WechatOAControls   workwx.OATemplateControls `bson:"wechat_oa_controls,omitempty"`
	Desc               string                    `bson:"desc,omitempty"`
	Ctime              int64                     `bson:"ctime"`
	Utime              int64                     `bson:"utime"`
}

type templateMigrator struct{}

// NewTemplateMigrator 构造工单模板迁移器。
func NewTemplateMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoTemplate, dao.Template](templateMigrator{})
}

func (templateMigrator) Name() string {
	return "template"
}

func (templateMigrator) CollectionName() string {
	return "c_template"
}

func (templateMigrator) Convert(src mongoTemplate) dao.Template {
	return dao.Template{
		Id:                 src.Id,
		TenantID:           DefaultTenantID,
		Name:               src.Name,
		WorkflowId:         src.WorkflowId,
		GroupId:            src.GroupId,
		Icon:               src.Icon,
		CreateType:         src.CreateType,
		Rules:              sqlx.JsonField[[]dao.Rule]{Val: src.Rules, Valid: src.Rules != nil},
		Options:            sqlx.JsonField[dao.TemplateOptions]{Val: src.Options, Valid: src.Options != nil},
		ExternalTemplateId: src.ExternalTemplateId,
		UniqueHash:         src.UniqueHash,
		WechatOAControls: sqlx.JsonField[workwx.OATemplateControls]{
			Val:   src.WechatOAControls,
			Valid: true,
		},
		Desc:  src.Desc,
		Ctime: src.Ctime,
		Utime: src.Utime,
	}
}
