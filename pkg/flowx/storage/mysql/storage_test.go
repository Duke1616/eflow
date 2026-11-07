package mysql

import (
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newTestDryRunDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(gormmysql.New(gormmysql.Config{
		DSN:                       "test:test@tcp(localhost:3306)/test?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:               true,
		DisableAutomaticPing: true,
	})
	require.NoError(t, err)
	return db
}

func TestMySQLStorage_SaveDefinitionSQL(t *testing.T) {
	db := newTestDryRunDB(t)
	s := NewStorage(db)

	def := model.ProcessDefinition{
		Key:     "leave",
		Name:    "请假流",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Name: "发起", Type: model.NodeStart},
			{ID: "end", Name: "结束", Type: model.NodeEnd},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "end"},
		},
	}
	po, err := toDefinitionPO(def)
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return s.buildSaveDefinitionQuery(tx, &po)
	})

	assert.Contains(t, sql, "INSERT INTO `flow_definition`")
	assert.Contains(t, sql, "ON DUPLICATE KEY UPDATE")
	assert.Contains(t, sql, "`proc_key`")
}

func TestMySQLStorage_GetTaskForUpdateSQL(t *testing.T) {
	db := newTestDryRunDB(t)
	s := NewStorage(db)

	var po FlowTaskPO
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return s.buildGetTaskForUpdateQuery(tx, "task_001", &po)
	})

	assert.Contains(t, sql, "SELECT * FROM `flow_task` WHERE id = 'task_001'")
	assert.Contains(t, sql, "FOR UPDATE")
}

func TestMySQLStorage_GetInstanceForUpdateSQL(t *testing.T) {
	db := newTestDryRunDB(t)
	s := NewStorage(db)

	var po FlowInstancePO
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return s.buildGetInstanceForUpdateQuery(tx, "inst_100", &po)
	})

	assert.Contains(t, sql, "SELECT * FROM `flow_instance` WHERE id = 'inst_100'")
	assert.Contains(t, sql, "FOR UPDATE")
}

func TestMySQLStorage_DO_POMapping(t *testing.T) {
	now := time.Now()
	instDO := &model.Instance{
		ID:          "inst_123",
		ProcKey:     "leave",
		Version:     2,
		Starter:     "alice",
		Status:      model.InstanceRunning,
		CurrentNode: "node_audit",
		Variables:   map[string]any{"days": 5, "dept": "IT"},
		CreateTime:  now,
	}

	po, err := toInstancePO(instDO)
	require.NoError(t, err)
	assert.Equal(t, "inst_123", po.ID)
	assert.Equal(t, "leave", po.ProcKey)
	assert.Contains(t, po.Variables, `"days":5`)

	recoveredDO, err := toInstanceDO(po)
	require.NoError(t, err)
	assert.Equal(t, instDO.ID, recoveredDO.ID)
	assert.Equal(t, instDO.Status, recoveredDO.Status)
	assert.Equal(t, float64(5), recoveredDO.Variables["days"])
}

func TestMySQLStorage_DoTxInterfaceGuard(t *testing.T) {
	db := newTestDryRunDB(t)
	s := NewStorage(db)

	// 验证实现了 ITxStorage 接口
	var txStore storage.ITxStorage = s
	require.NotNil(t, txStore)
}

func TestMySQLStorage_ListPendingTasksByAssigneeSQL(t *testing.T) {
	db := newTestDryRunDB(t)
	s := NewStorage(db)

	t.Run("指定审批人查询待办", func(t *testing.T) {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			query := s.buildListPendingTasksQuery(tx, "leader_zhang")
			var pos []FlowTaskPO
			return query.Order("ctime DESC").Offset(0).Limit(10).Find(&pos)
		})
		assert.Contains(t, sql, "SELECT * FROM `flow_task`")
		assert.Contains(t, sql, "status = 0")
		assert.Contains(t, sql, "assignee = 'leader_zhang'")
	})

	t.Run("空审批人查询全部待办", func(t *testing.T) {
		sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
			query := s.buildListPendingTasksQuery(tx, "")
			var pos []FlowTaskPO
			return query.Order("ctime DESC").Offset(0).Limit(10).Find(&pos)
		})
		assert.Contains(t, sql, "SELECT * FROM `flow_task`")
		assert.Contains(t, sql, "status = 0")
		assert.NotContains(t, sql, "assignee")
	})
}
