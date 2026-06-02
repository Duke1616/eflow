package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// mongoLogicFlow MongoDB 中的 LogicFlow 画布拓扑源数据实体
type mongoLogicFlow struct {
	Edges []domain.FlowEdge `bson:"edges"`
	Nodes []domain.FlowNode `bson:"nodes"`
}

// mongoWorkflow MongoDB 中的工作流定义源数据实体
type mongoWorkflow struct {
	ID           int64          `bson:"id"`
	TemplateID   int64          `bson:"template_id"`
	Name         string         `bson:"name"`
	Icon         string         `bson:"icon"`
	Owner        string         `bson:"owner"`
	Desc         string         `bson:"desc"`
	ProcessID    int            `bson:"process_id"`
	FlowData     mongoLogicFlow `bson:"flow_data"`
	IsNotify     bool           `bson:"is_notify"`
	NotifyMethod uint8          `bson:"notify_method"`
	Ctime        int64          `bson:"ctime"`
	Utime        int64          `bson:"utime"`
}

type workflowMigrator struct{}

// NewWorkflowMigrator 构造工作流定义的泛型迁移对拷器
func NewWorkflowMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoWorkflow, dao.Workflow](workflowMigrator{})
}

func (workflowMigrator) Name() string {
	return "workflow"
}

func (workflowMigrator) CollectionName() string {
	return "c_workflow"
}

func (workflowMigrator) Convert(src mongoWorkflow) dao.Workflow {
	return dao.Workflow{
		Id:         src.ID,
		TenantID:   DefaultTenantID,
		TemplateId: src.TemplateID,
		Name:       src.Name,
		Icon:       src.Icon,
		Owner:      src.Owner,
		Desc:       src.Desc,
		ProcessId:  src.ProcessID,
		FlowData: sqlx.JsonField[dao.LogicFlow]{
			Val: dao.LogicFlow{
				Edges: src.FlowData.Edges,
				Nodes: src.FlowData.Nodes,
			},
			Valid: src.FlowData.Edges != nil || src.FlowData.Nodes != nil,
		},
		IsNotify:     src.IsNotify,
		NotifyMethod: src.NotifyMethod,
		Ctime:        src.Ctime,
		Utime:        src.Utime,
	}
}
