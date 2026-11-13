package dao

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/pkg/ticketpbac"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/pbac"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// priorTaskColumns 两张任务表的公共列。时间列必须声明 DATETIME：
// LocalTime.Scan 只接受 time.Time，声明成 TEXT 会在扫描阶段直接报错
const priorTaskColumns = `proc_id INTEGER, proc_inst_id INTEGER, business_id TEXT, starter TEXT,
	node_id TEXT, node_name TEXT, prev_node_id TEXT, is_cosigned INTEGER, batch_code TEXT,
	user_id TEXT, status INTEGER, is_finished INTEGER, comment TEXT,
	proc_inst_create_time DATETIME, create_time DATETIME, finished_time DATETIME`

// priorTaskColumnNames 与 priorTaskColumns 顺序严格一致，供 INSERT 使用
const priorTaskColumnNames = `proc_id, proc_inst_id, business_id, starter,
	node_id, node_name, prev_node_id, is_cosigned, batch_code,
	user_id, status, is_finished, comment,
	proc_inst_create_time, create_time, finished_time`

// sqlRecorder 以 logger 写入器身份捕获最终插值 SQL。
// 不用 Query 回调：回调拿到的是占位符形态，且会漏掉外层 SELECT 只留下 UNION 子查询
type sqlRecorder struct {
	mu    sync.Mutex
	stmts []string
}

func (r *sqlRecorder) Printf(format string, args ...interface{}) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stmts = append(r.stmts, fmt.Sprintf(format, args...))
}

func (r *sqlRecorder) joined() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return strings.Join(r.stmts, "\n")
}

// reset 丢掉建表与铺数据的 SQL，只留下被测查询，断言失败时信息才可读
func (r *sqlRecorder) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stmts = nil
}

func insertPriorTask(t *testing.T, db *gorm.DB, table string, id, instID, status int, comment string, finished time.Time) {
	t.Helper()
	insertPriorTaskOnNode(t, db, table, id, instID, "node", status, comment, finished)
}

func insertPriorTaskOnNode(t *testing.T, db *gorm.DB, table string, id, instID int, nodeID string, status int, comment string, finished time.Time) {
	t.Helper()
	idCol := "id"
	if table == "hist_proc_task" {
		idCol = "task_id"
	}
	require.NoError(t, db.Exec(
		"INSERT INTO "+table+" ("+idCol+","+priorTaskColumnNames+") VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)",
		id, 7, instID, "biz", "alice", nodeID, "环节", "", 0, "batch", "alice", status, 1, comment,
		finished, finished, finished,
	).Error)
}

// setupPriorCommentDB 铺一条链路的真实数据：含待办、系统裁决、空白意见、归档表记录，
// 以及跨实例/跨租户/非本人工单三类越权数据，用于验证下推过滤没有放宽可见范围
func setupPriorCommentDB(t *testing.T) (*gorm.DB, context.Context, *sqlRecorder) {
	t.Helper()
	rec := &sqlRecorder{}
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.New(rec, logger.Config{LogLevel: logger.Info}),
	})
	require.NoError(t, err)

	for _, ddl := range []string{
		"CREATE TABLE proc_task (id INTEGER PRIMARY KEY," + priorTaskColumns + ")",
		"CREATE TABLE hist_proc_task (task_id INTEGER PRIMARY KEY," + priorTaskColumns + ")",
		"CREATE TABLE proc_def (id INTEGER PRIMARY KEY, name TEXT)",
		"CREATE TABLE ticket (process_instance_id INTEGER, tenant_id INTEGER, create_by TEXT)",
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO proc_def VALUES (7, '请假流程')").Error)
	for _, tk := range []struct {
		instID, tenantID int
		createBy         string
	}{
		{100, 1, "alice"}, {200, 1, "alice"}, {300, 2, "alice"}, {400, 1, "bob"},
	} {
		require.NoError(t, db.Exec("INSERT INTO ticket VALUES (?,?,?)", tk.instID, tk.tenantID, tk.createBy).Error)
	}

	at := func(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC) }
	// 同一时刻的三条记录（归档 10、在办 8、在办 7）用于验证 id 兜底排序，
	// UNION ALL 的物理返回序是 7,8,10，若依赖它就会拿到错误顺序
	insertPriorTask(t, db, "proc_task", 1, 100, 0, "", at(12, 9))
	insertPriorTask(t, db, "proc_task", 2, 100, 1, "同意，预算内", at(10, 9))
	insertPriorTask(t, db, "proc_task", 3, 100, 2, "口径不对，驳回", at(11, 9))
	insertPriorTask(t, db, "proc_task", 4, 100, 3, "系统自动通过", at(12, 9))
	insertPriorTask(t, db, "proc_task", 5, 100, 1, "   ", at(12, 10))
	insertPriorTask(t, db, "proc_task", 6, 100, 1, "", at(12, 11))
	insertPriorTask(t, db, "proc_task", 7, 100, 1, "同一秒A", at(13, 9))
	insertPriorTask(t, db, "proc_task", 8, 100, 1, "同一秒B", at(13, 9))
	insertPriorTask(t, db, "proc_task", 11, 200, 1, "别的实例", at(13, 9))
	insertPriorTask(t, db, "proc_task", 12, 300, 1, "别的租户", at(13, 9))
	insertPriorTask(t, db, "proc_task", 13, 400, 1, "非本人工单", at(13, 9))
	insertPriorTask(t, db, "hist_proc_task", 9, 100, 2, "历史驳回意见", at(9, 9))
	insertPriorTask(t, db, "hist_proc_task", 10, 100, 1, "历史通过", at(13, 9))

	ctx := ctxutil.WithTenantID(pbac.WithDecision(context.Background(), pbac.Decision{
		Allowed:       true,
		FilterProfile: ticketpbac.HistoryProfile,
		AllowAccessScope: &pbac.AccessScope{Predicate: &pbac.Predicate{
			Key: ticketpbac.TicketCreateBy, Operator: pbac.StringEquals,
			Values: []pbac.Operand{pbac.Literal("alice")},
		}},
	}), 1)

	rec.reset()
	return db, ctx, rec
}

func TestListAccessiblePriorComments(t *testing.T) {
	db, ctx, _ := setupPriorCommentDB(t)
	dao := NewProcessEngineDAO(db)

	testCases := []struct {
		name    string
		limit   int
		wantIDs []int
	}{
		{
			name:    "按展开上限截断_只回传最近5条",
			limit:   5,
			wantIDs: []int{10, 8, 7, 3, 2},
		},
		{
			name:    "上限小于可读条数_截断不影响总数口径",
			limit:   2,
			wantIDs: []int{10, 8},
		},
		{
			name:    "上限等于可读条数_全部回传且归档记录参与排序",
			limit:   6,
			wantIDs: []int{10, 8, 7, 3, 2, 9},
		},
		{
			name:    "上限远大于可读条数_待办与系统裁决及空白意见始终被排除",
			limit:   100,
			wantIDs: []int{10, 8, 7, 3, 2, 9},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tasks, err := dao.ListAccessiblePriorComments(ctx, 100, tc.limit, nil)
			require.NoError(t, err)

			gotIDs := make([]int, 0, len(tasks))
			for _, task := range tasks {
				gotIDs = append(gotIDs, task.TaskID)
			}
			assert.Equal(t, tc.wantIDs, gotIDs)

			total, err := dao.CountAccessiblePriorComments(ctx, 100, nil)
			require.NoError(t, err)
			assert.Equal(t, int64(6), total, "总数必须是过滤后的可读意见数，不随 limit 变化")
		})
	}
}

func TestListAccessiblePriorComments_FieldMapping(t *testing.T) {
	db, ctx, _ := setupPriorCommentDB(t)

	tasks, err := NewProcessEngineDAO(db).ListAccessiblePriorComments(ctx, 100, 1, nil)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	// 归档表首列名为 task_id，靠 UNION 首个 SELECT 的列名统一成 id，这里验证该假设成立
	assert.Equal(t, 10, tasks[0].TaskID)
	assert.Equal(t, "历史通过", tasks[0].Comment)
	assert.Equal(t, 1, tasks[0].Status)
	assert.Equal(t, "node", tasks[0].NodeID)
	assert.Equal(t, "环节", tasks[0].NodeName)
	assert.Equal(t, "alice", tasks[0].UserID)
	assert.Equal(t, "请假流程", tasks[0].ProcName)
	require.NotNil(t, tasks[0].FinishedTime)
	assert.Equal(t, "2026-11-13 09:00:00", tasks[0].FinishedTime.String())
}

func TestPriorCommentsPushdownToSQL(t *testing.T) {
	db, ctx, rec := setupPriorCommentDB(t)
	dao := NewProcessEngineDAO(db)

	_, err := dao.ListAccessiblePriorComments(ctx, 100, 5, nil)
	require.NoError(t, err)
	_, err = dao.CountAccessiblePriorComments(ctx, 100, nil)
	require.NoError(t, err)

	sql := rec.joined()
	// 过滤、排序与截断必须真的出现在 SQL 里，而不是取回后在内存做
	assert.Contains(t, sql, "a.status IN (1,2)")
	assert.Contains(t, sql, "TRIM(a.comment) <> ''")
	assert.Contains(t, sql, "ORDER BY a.finished_time DESC, a.id DESC")
	assert.Contains(t, sql, "LIMIT 5")
	// 精确查询不得丢掉租户与 PBAC 约束，否则等于放宽了审批记录的可见范围。
	// 这里只断言谓词列存在：字面量引号风格由方言决定（sqlite 用双引号），
	// 取值是否真的生效由实跑用例中「非本人工单被排除」证明
	assert.Contains(t, sql, "ticket.tenant_id = 1")
	assert.Contains(t, sql, "ticket.create_by IN")
	assert.Contains(t, sql, "hist_proc_task")
}

// TestPriorCommentsExcludeMachineNodes 验证机器节点排除真的落在 SQL 并生效。
// 内置反向对照：同一批数据不排除时，机器条目既挤占 LIMIT 名额又虚增 COUNT，证明该过滤是承重的
func TestPriorCommentsExcludeMachineNodes(t *testing.T) {
	db, ctx, rec := setupPriorCommentDB(t)
	dao := NewProcessEngineDAO(db)

	at := func(day, hour int) time.Time { return time.Date(2026, 11, day, hour, 0, 0, 0, time.UTC) }
	// 机器条目全部晚于人工意见：若先截断再过滤，整页会被机器条目占满而人工意见一条不剩
	insertPriorTaskOnNode(t, db, "proc_task", 60, 100, "cc", 1, "CarbonCopy Auto Pass", at(14, 9))
	insertPriorTaskOnNode(t, db, "proc_task", 61, 100, "chat", 1, "ChatGroup Auto Pass", at(14, 10))
	insertPriorTaskOnNode(t, db, "proc_task", 62, 100, "proxy", 1, "Sys Auto Pass", at(14, 11))
	insertPriorTaskOnNode(t, db, "hist_proc_task", 63, 100, "auto", 1, "自动化作业完成", at(14, 12))

	exclude := []string{"cc", "chat", "proxy", "auto"}

	tasks, err := dao.ListAccessiblePriorComments(ctx, 100, 5, exclude)
	require.NoError(t, err)
	gotIDs := make([]int, 0, len(tasks))
	for _, task := range tasks {
		gotIDs = append(gotIDs, task.TaskID)
	}
	assert.Equal(t, []int{10, 8, 7, 3, 2}, gotIDs, "机器条目不得挤占展开名额")

	total, err := dao.CountAccessiblePriorComments(ctx, 100, exclude)
	require.NoError(t, err)
	assert.Equal(t, int64(6), total, "总数不得被机器条目虚增")

	assert.Contains(t, rec.joined(), "a.node_id NOT IN", "排除必须下推到 SQL，不能取回内存再过滤")

	rawTasks, err := dao.ListAccessiblePriorComments(ctx, 100, 5, nil)
	require.NoError(t, err)
	rawIDs := make([]int, 0, len(rawTasks))
	for _, task := range rawTasks {
		rawIDs = append(rawIDs, task.TaskID)
	}
	assert.Equal(t, []int{63, 62, 61, 60, 10}, rawIDs, "反向对照：不排除时整页被机器条目占满")

	rawTotal, err := dao.CountAccessiblePriorComments(ctx, 100, nil)
	require.NoError(t, err)
	assert.Equal(t, int64(10), rawTotal, "反向对照：不排除时总数被机器条目虚增")
}

func TestListAccessiblePriorComments_TenantMissing(t *testing.T) {
	db, _, _ := setupPriorCommentDB(t)

	_, err := NewProcessEngineDAO(db).ListAccessiblePriorComments(context.Background(), 100, 5, nil)
	require.Error(t, err, "缺少租户上下文必须失败，不能退化成全量可见")
}
