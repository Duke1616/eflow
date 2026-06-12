package migrations

import (
	"github.com/Duke1616/eiam/pkg/migration"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// mongoWorkflowSnapshot 是 ecmdb 中 c_workflow_snapshot 集合的历史结构。
type mongoWorkflowSnapshot struct {
	Id             int64          `bson:"id"`
	WorkflowId     int            `bson:"workflow_id"`
	ProcessId      int            `bson:"process_id"`
	ProcessVersion int            `bson:"process_version"`
	Name           string         `bson:"name"`
	FlowData       mongoLogicFlow `bson:"flow_data"`
	Ctime          int64          `bson:"ctime"`
}

type workflowInstanceFlowMigrator struct{}

// NewWorkflowInstanceFlowMigrator 构造流程快照迁移器。
func NewWorkflowInstanceFlowMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoWorkflowSnapshot, dao.Snapshot](workflowInstanceFlowMigrator{})
}

func (workflowInstanceFlowMigrator) Name() string {
	return "workflow_instance_flow"
}

func (workflowInstanceFlowMigrator) CollectionName() string {
	return "c_workflow_snapshot"
}

func (workflowInstanceFlowMigrator) Convert(src mongoWorkflowSnapshot) dao.Snapshot {
	return dao.Snapshot{
		Id:             src.Id,
		TenantID:       DefaultTenantID,
		WorkflowId:     int64(src.WorkflowId),
		ProcessId:      src.ProcessId,
		ProcessVersion: src.ProcessVersion,
		Name:           src.Name,
		FlowData: sqlx.JsonField[dao.LogicFlow]{
			Val: dao.LogicFlow{
				Edges: src.FlowData.Edges,
				Nodes: src.FlowData.Nodes,
			},
			Valid: src.FlowData.Edges != nil || src.FlowData.Nodes != nil,
		},
		Ctime: src.Ctime,
	}
}
