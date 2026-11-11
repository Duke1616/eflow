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

func TestMySQLStorage_ListPendingTasksSQL(t *testing.T) {
	testCases := []struct {
		name        string
		query       model.PendingTaskQuery
		wantContain []string
		wantAbsent  []string
	}{
		{
			name:        "指定审批人查询待办",
			query:       model.PendingTaskQuery{Assignee: "leader_zhang"},
			wantContain: []string{"SELECT * FROM `flow_task`", "flow_task.status = 0", "flow_task.assignee = 'leader_zhang'"},
			wantAbsent:  []string{"flow_instance", "flow_definition"},
		},
		{
			name:        "空审批人查询全部待办",
			query:       model.PendingTaskQuery{},
			wantContain: []string{"SELECT * FROM `flow_task`", "flow_task.status = 0"},
			wantAbsent:  []string{"assignee", "flow_instance"},
		},
		{
			name:        "流程名过滤经子查询回溯定义表_外层保持单表避免同名列覆盖",
			query:       model.PendingTaskQuery{Assignee: "leader_zhang", ProcessName: "请假审批"},
			wantContain: []string{"SELECT * FROM `flow_task`", "flow_task.instance_id IN (SELECT", "flow_instance AS fi", "flow_definition AS fd", "fd.proc_key = fi.proc_key", "fd.version = fi.version", "fd.name = '请假审批'"},
			// 外层严禁出现 JOIN：flow_instance 与 flow_task 的 id/status/ctime 同名列会污染扫描结果
			wantAbsent: []string{"JOIN flow_instance AS fi ON fi.id = flow_task.instance_id"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDryRunDB(t)
			s := NewStorage(db)

			sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
				query := s.buildListPendingTasksQuery(tx, tc.query)
				var pos []FlowTaskPO
				return query.Order("flow_task.ctime DESC").Offset(0).Limit(10).Find(&pos)
			})

			for _, want := range tc.wantContain {
				assert.Contains(t, sql, want)
			}
			for _, absent := range tc.wantAbsent {
				assert.NotContains(t, sql, absent)
			}
		})
	}
}

func TestMySQLStorage_UpdateTasksStatusSQL(t *testing.T) {
	db := newTestDryRunDB(t)

	now := time.Now()
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&FlowTaskPO{}).
			Where("id IN ?", []string{"task_001", "task_002"}).
			Updates(map[string]any{
				"status": uint8(model.TaskCancelled),
				"ftime":  now.UnixMilli(),
			})
	})

	assert.Contains(t, sql, "UPDATE `flow_task`")
	assert.Contains(t, sql, "`status`=4")
	assert.Contains(t, sql, "`ftime`=")
	// 验证 ftime 更新的是纯数字毫秒时间戳，而非带日期的字符串
	assert.NotContains(t, sql, now.Format("2006-01-02"))
}

func TestMySQLStorage_ListPendingTasksByInstancesSQL(t *testing.T) {
	db := newTestDryRunDB(t)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var pos []FlowTaskPO
		return tx.Where("status = ? AND instance_id IN ?", uint8(model.TaskPending), []string{"inst_001", "inst_002"}).
			Find(&pos)
	})

	assert.Contains(t, sql, "SELECT * FROM `flow_task`")
	assert.Contains(t, sql, "status = 0")
	assert.Contains(t, sql, "instance_id IN ('inst_001','inst_002')")
}
