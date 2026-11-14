package dao

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/ticketpbac"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/Duke1616/eiam/pkg/pbac"
	pbacgorm "github.com/Duke1616/eiam/pkg/pbac/gormx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

// setupScopedTicketDB 搭建与生产一致的双闸门环境：单数表名 + 全局租户插件，
// 并铺好跨租户、跨引擎、待办/已办任务数据，用于验证可见范围没有被放宽
func setupScopedTicketDB(t *testing.T) (*gorm.DB, TicketDAO) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormx.NewTenantPlugin()))
	require.NoError(t, db.AutoMigrate(&Ticket{}))
	for _, ddl := range []string{
		"CREATE TABLE flow_task (id INTEGER PRIMARY KEY, instance_id TEXT, assignee TEXT, status INTEGER)",
		"CREATE TABLE proc_task (id INTEGER PRIMARY KEY, proc_inst_id INTEGER, user_id TEXT, is_finished INTEGER)",
		"CREATE TABLE hist_proc_task (task_id INTEGER PRIMARY KEY, proc_inst_id INTEGER, user_id TEXT)",
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}

	tickets := []Ticket{
		{Id: 1, TenantID: 1, CreateBy: "alice", EngineType: "flowx", ProcessInstanceKey: "fx_alice", Status: domain.PROCESS.ToUint8(), Ctime: 5000},
		{Id: 2, TenantID: 1, CreateBy: "bob", EngineType: "flowx", ProcessInstanceKey: "fx_bob", Status: domain.PROCESS.ToUint8(), Ctime: 4000},
		{Id: 3, TenantID: 1, CreateBy: "bob", EngineType: "easy-workflow", ProcessInstanceId: 300, Status: domain.PROCESS.ToUint8(), Ctime: 3000},
		{Id: 4, TenantID: 1, CreateBy: "bob", EngineType: "easy-workflow", ProcessInstanceId: 400, Status: domain.PROCESS.ToUint8(), Ctime: 2000},
		{Id: 5, TenantID: 1, CreateBy: "bob", EngineType: "flowx", ProcessInstanceKey: "fx_bob_done", Status: domain.PROCESS.ToUint8(), Ctime: 1000},
		{Id: 6, TenantID: 2, CreateBy: "alice", EngineType: "flowx", ProcessInstanceKey: "fx_other_tenant", Status: domain.PROCESS.ToUint8(), Ctime: 6000},
		{Id: 7, TenantID: 1, CreateBy: "carol", EngineType: "flowx", ProcessInstanceKey: "fx_carol", Status: domain.END.ToUint8(), Ctime: 7000},
		{Id: 8, TenantID: 2, CreateBy: "alice", EngineType: "easy-workflow", ProcessInstanceId: 800, Status: domain.PROCESS.ToUint8(), Ctime: 8000},
	}
	for i := range tickets {
		require.NoError(t, db.Create(&tickets[i]).Error)
	}
	for _, row := range []struct {
		id         int
		instanceID string
		assignee   string
		status     int
	}{
		{1, "fx_alice", "bob", 0},
		{2, "fx_bob", "alice", 0},
		{3, "fx_bob_done", "alice", 1},
		{4, "fx_other_tenant", "alice", 0},
	} {
		require.NoError(t, db.Exec("INSERT INTO flow_task VALUES (?,?,?,?)", row.id, row.instanceID, row.assignee, row.status).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO proc_task VALUES (1, 300, 'alice', 0)").Error)
	require.NoError(t, db.Exec("INSERT INTO proc_task VALUES (2, 400, 'alice', 1)").Error)
	require.NoError(t, db.Exec("INSERT INTO hist_proc_task VALUES (1, 400, 'alice')").Error)
	return db, NewTicketDAO(db)
}

func scopedCtx(tid int64, decision pbac.Decision) context.Context {
	return ctxutil.WithTenantID(pbac.WithDecision(context.Background(), decision), tid)
}

func todoDecision(scope *pbac.AccessScope) pbac.Decision {
	return pbac.Decision{Allowed: true, FilterProfile: ticketpbac.TodoProfile, AllowAccessScope: scope}
}

func historyDecision(scope *pbac.AccessScope) pbac.Decision {
	return pbac.Decision{Allowed: true, FilterProfile: ticketpbac.HistoryProfile, AllowAccessScope: scope}
}

func assigneeScope(username string) *pbac.AccessScope {
	return &pbac.AccessScope{Predicate: &pbac.Predicate{
		Key: ticketpbac.TaskAssignee, Operator: pbac.StringEquals,
		Values: []pbac.Operand{pbac.Literal(username)},
	}}
}

func relatedScope(username string) *pbac.AccessScope {
	return &pbac.AccessScope{Predicate: &pbac.Predicate{
		Key: ticketpbac.TicketRelatedUsers, Operator: pbac.ForAnyValueStringEquals,
		Values: []pbac.Operand{pbac.Literal(username)},
	}}
}

func creatorScope(username string) *pbac.AccessScope {
	return &pbac.AccessScope{Predicate: &pbac.Predicate{
		Key: ticketpbac.TicketCreateBy, Operator: pbac.StringEquals,
		Values: []pbac.Operand{pbac.Literal(username)},
	}}
}

func ticketIDs(tickets []Ticket) []int64 {
	ids := make([]int64, 0, len(tickets))
	for _, tk := range tickets {
		ids = append(ids, tk.Id)
	}
	return ids
}

func TestListTodo_ScopedVisibility(t *testing.T) {
	_, dao := setupScopedTicketDB(t)
	activeStatuses := []int{
		domain.START.ToInt(), domain.PROCESS.ToInt(),
		domain.WITHDRAWING.ToInt(), domain.START_FAILED.ToInt(),
	}

	testCases := []struct {
		name      string
		ctx       context.Context
		wantIDs   []int64
		wantCount int64
	}{
		{
			name:      "处理人范围只回双引擎当前待办命中工单_已办任务不算待办",
			ctx:       scopedCtx(1, todoDecision(assigneeScope("alice"))),
			wantIDs:   []int64{2, 3},
			wantCount: 2,
		},
		{
			name:      "相关人范围含本人创建与历史参与_跨租户仍被拦截",
			ctx:       scopedCtx(1, todoDecision(relatedScope("alice"))),
			wantIDs:   []int64{1, 2, 3, 4, 5},
			wantCount: 5,
		},
		{
			name:      "提单人范围只回本人创建工单",
			ctx:       scopedCtx(1, todoDecision(creatorScope("alice"))),
			wantIDs:   []int64{1},
			wantCount: 1,
		},
		{
			name:      "未配置范围时仍受租户与状态约束",
			ctx:       scopedCtx(1, todoDecision(nil)),
			wantIDs:   []int64{1, 2, 3, 4, 5},
			wantCount: 5,
		},
		{
			name:      "切换租户上下文只可见本租户工单",
			ctx:       scopedCtx(2, todoDecision(nil)),
			wantIDs:   []int64{8, 6},
			wantCount: 2,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tickets, err := dao.ListTodo(tc.ctx, "", activeStatuses, 0, 10)
			require.NoError(t, err)
			assert.Equal(t, tc.wantIDs, ticketIDs(tickets))

			total, err := dao.CountTodo(tc.ctx, "", activeStatuses)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCount, total)
		})
	}
}

func TestListTodo_FailClosed(t *testing.T) {
	_, dao := setupScopedTicketDB(t)
	activeStatuses := []int{domain.PROCESS.ToInt()}

	testCases := []struct {
		name    string
		ctx     context.Context
		wantErr error
	}{
		{
			name:    "缺失PBAC决策拒绝查询",
			ctx:     ctxutil.WithTenantID(context.Background(), 1),
			wantErr: pbacgorm.ErrDecisionMissing,
		},
		{
			name: "决策拒绝访问时拒绝查询",
			ctx:  scopedCtx(1, pbac.Decision{Allowed: false, FilterProfile: ticketpbac.TodoProfile}),
		},
		{
			name: "决策profile与查询不匹配时拒绝查询",
			ctx:  scopedCtx(1, historyDecision(nil)),
		},
		{
			name: "缺失租户上下文拒绝查询",
			ctx:  pbac.WithDecision(context.Background(), todoDecision(nil)),
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := dao.ListTodo(tc.ctx, "", activeStatuses, 0, 10)
			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}

			_, err = dao.CountTodo(tc.ctx, "", activeStatuses)
			require.Error(t, err)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			}
		})
	}
}

func TestDetailByProcessInstanceScoped(t *testing.T) {
	_, dao := setupScopedTicketDB(t)

	testCases := []struct {
		name       string
		ctx        context.Context
		instanceID string
		wantID     int64
		wantErr    error
	}{
		{
			name:       "提单人范围命中FlowX实例Key",
			ctx:        scopedCtx(1, historyDecision(creatorScope("alice"))),
			instanceID: "fx_alice",
			wantID:     1,
		},
		{
			name:       "提单人范围拒绝他人工单_对外表现为不存在",
			ctx:        scopedCtx(1, historyDecision(creatorScope("alice"))),
			instanceID: "fx_bob",
			wantErr:    gorm.ErrRecordNotFound,
		},
		{
			name:       "跨租户实例Key被租户闸门拦截",
			ctx:        scopedCtx(1, historyDecision(creatorScope("alice"))),
			instanceID: "fx_other_tenant",
			wantErr:    gorm.ErrRecordNotFound,
		},
		{
			name:       "历史整型ID兜底同样处于双闸门之下",
			ctx:        scopedCtx(1, historyDecision(relatedScope("alice"))),
			instanceID: "300",
			wantID:     3,
		},
		{
			name:       "跨租户整型ID兜底被拦截",
			ctx:        scopedCtx(1, historyDecision(relatedScope("alice"))),
			instanceID: "800",
			wantErr:    gorm.ErrRecordNotFound,
		},
		{
			name:       "缺失PBAC决策时fail-closed",
			ctx:        ctxutil.WithTenantID(context.Background(), 1),
			instanceID: "fx_alice",
			wantErr:    pbacgorm.ErrDecisionMissing,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ticket, err := dao.DetailByProcessInstanceScoped(tc.ctx, tc.instanceID)
			if tc.wantErr != nil {
				assert.True(t, errors.Is(err, tc.wantErr), "期望错误 %v, 实际 %v", tc.wantErr, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantID, ticket.Id)
		})
	}
}
