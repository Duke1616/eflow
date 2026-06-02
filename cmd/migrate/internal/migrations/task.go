package migrations

import (
	"github.com/Duke1616/eflow/cmd/migrate/internal/migration"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// mongoTask MongoDB 中的自动化作业执行任务源数据实体
type mongoTask struct {
	ID              int64            `bson:"id"`
	OrderID         int64            `bson:"order_id"`
	ProcessInstID   int              `bson:"process_inst_id"`
	CurrentNodeID   string           `bson:"current_node_id"`
	TriggerPosition string           `bson:"trigger_position"`
	WorkflowID      int64            `bson:"workflow_id"`
	CodebookName    string           `bson:"codebook_name"`
	CodebookUID     string           `bson:"codebook_uid"`
	Code            string           `bson:"code"`
	Language        string           `bson:"language"`
	Args            domain.TaskArgs  `bson:"args"`
	Variables       []mongoVariables `bson:"variables"`
	Status          uint8            `bson:"status"`
	Result          string           `bson:"result"`
	WantResult      string           `bson:"want_result"`
	ExternalID      string           `bson:"external_id"`
	StartTime       int64            `bson:"start_time"`
	EndTime         int64            `bson:"end_time"`
	RetryCount      int              `bson:"retry_count"`
	IsTiming        bool             `bson:"is_timing"`
	ScheduledTime   int64            `bson:"scheduled_time"`
	Kind            string           `bson:"kind"`
	Target          string           `bson:"target"`
	Handler         string           `bson:"handler"`
	MarkPassed      bool             `bson:"mark_passed"`
	Ctime           int64            `bson:"ctime"`
	Utime           int64            `bson:"utime"`
}

type taskMigrator struct{}

// NewTaskMigrator 构造自动化作业任务流转历史数据的泛型迁移对拷器
func NewTaskMigrator() migration.Migrator {
	return migration.NewMongoMigrator[mongoTask, dao.Task](taskMigrator{})
}

func (taskMigrator) Name() string {
	return "task"
}

func (taskMigrator) CollectionName() string {
	return "c_task"
}

func (taskMigrator) Convert(src mongoTask) dao.Task {
	vars := make([]domain.Variables, 0, len(src.Variables))
	for _, v := range src.Variables {
		vars = append(vars, domain.Variables{
			Key:    v.Key,
			Value:  v.Value,
			Secret: v.Secret,
		})
	}

	return dao.Task{
		Id:              src.ID,
		TenantID:        DefaultTenantID,
		TicketID:        src.OrderID,
		ProcessInstId:   src.ProcessInstID,
		CurrentNodeId:   src.CurrentNodeID,
		TriggerPosition: src.TriggerPosition,
		WorkflowId:      src.WorkflowID,
		CodebookUid:     src.CodebookUID,
		Code:            src.Code,
		Language:        src.Language,
		Args:            sqlx.JsonField[domain.TaskArgs]{Val: src.Args, Valid: src.Args != nil},
		Variables:       sqlx.JsonField[[]domain.Variables]{Val: vars, Valid: len(vars) > 0},
		Status:          src.Status,
		Result:          src.Result,
		WantResult:      src.WantResult,
		ExternalId:      src.ExternalID,
		StartTime:       src.StartTime,
		EndTime:         src.EndTime,
		RetryCount:      src.RetryCount,
		IsTiming:        src.IsTiming,
		ScheduledTime:   src.ScheduledTime,
		Kind:            src.Kind,
		Target:          src.Target,
		Handler:         src.Handler,
		AutoPassed:      src.MarkPassed,
		Ctime:           src.Ctime,
		Utime:           src.Utime,
	}
}
