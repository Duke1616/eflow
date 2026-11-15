package dao

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eiam/pkg/gormx"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

func TestWithdrawalPendingStatusesExpandAsSQLList(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:3306)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&Task{}).
			Where("ticket_id = ? AND status IN ?", 2663, withdrawalPendingStatuses()).
			Update("status", 7)
	})

	require.Contains(t, sql, "status IN (4,5,2)")
	require.NotContains(t, sql, "<binary>")
}

// TestListStaleResolvesBothEngines 用真实 SQL 验证撤回恢复候选的引擎状态判定：
// easyflow 运行中在 proc_inst、撤销后迁到 hist_proc_inst(status=2)；
// flowx 始终在 flow_instance，靠 status 区分流转中(1)与已撤回(4)，且实例标识只有 Key。
func TestListStaleResolvesBothEngines(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	require.NoError(t, err)
	require.NoError(t, db.Use(gormx.NewTenantPlugin()))
	require.NoError(t, db.AutoMigrate(&Ticket{}))
	for _, ddl := range []string{
		"CREATE TABLE proc_inst (id INTEGER PRIMARY KEY)",
		"CREATE TABLE hist_proc_inst (proc_inst_id INTEGER, status INTEGER)",
		"CREATE TABLE flow_instance (id TEXT PRIMARY KEY, status INTEGER)",
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}

	tickets := []Ticket{
		{Id: 1, TenantID: 1, EngineType: "easy-workflow", ProcessInstanceId: 101, Status: domain.WITHDRAWING.ToUint8(), Utime: 1000},
		{Id: 2, TenantID: 1, EngineType: "easy-workflow", ProcessInstanceId: 102, Status: domain.WITHDRAWING.ToUint8(), Utime: 1000},
		{Id: 3, TenantID: 2, EngineType: "flowx", ProcessInstanceKey: "fx_running", Status: domain.WITHDRAWING.ToUint8(), Utime: 1000},
		{Id: 4, TenantID: 1, EngineType: "flowx", ProcessInstanceKey: "fx_cancelled", Status: domain.WITHDRAWING.ToUint8(), Utime: 1000},
		{Id: 5, TenantID: 1, EngineType: "flowx", ProcessInstanceKey: "fx_finished", Status: domain.WITHDRAWING.ToUint8(), Utime: 1000},
		{Id: 6, TenantID: 1, EngineType: "flowx", ProcessInstanceKey: "fx_missing", Status: domain.WITHDRAWING.ToUint8(), Utime: 1000},
		{Id: 7, TenantID: 1, EngineType: "easy-workflow", ProcessInstanceId: 101, Status: domain.PROCESS.ToUint8(), Utime: 1000},
		{Id: 8, TenantID: 1, EngineType: "flowx", ProcessInstanceKey: "fx_running", Status: domain.WITHDRAWING.ToUint8(), Utime: 999999},
	}
	for i := range tickets {
		require.NoError(t, db.Create(&tickets[i]).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO proc_inst (id) VALUES (101)").Error)
	require.NoError(t, db.Exec("INSERT INTO hist_proc_inst (proc_inst_id, status) VALUES (102, 2)").Error)
	for _, row := range []struct {
		id     string
		status int
	}{
		{"fx_running", 1}, {"fx_cancelled", 4}, {"fx_finished", 2},
	} {
		require.NoError(t, db.Exec("INSERT INTO flow_instance (id, status) VALUES (?, ?)", row.id, row.status).Error)
	}

	candidates, err := NewWithdrawalDAO(db).ListStale(context.Background(), 5000, 0, 100)
	require.NoError(t, err)

	byTicket := make(map[int64]WithdrawalCandidate, len(candidates))
	for _, candidate := range candidates {
		byTicket[candidate.TicketID] = candidate
	}

	testCases := []struct {
		ticketID     int64
		wantIncluded bool
		wantActive   bool
		wantRevoked  bool
		wantKey      string
	}{
		{ticketID: 1, wantIncluded: true, wantActive: true},
		{ticketID: 2, wantIncluded: true, wantRevoked: true},
		{ticketID: 3, wantIncluded: true, wantActive: true, wantKey: "fx_running"},
		{ticketID: 4, wantIncluded: true, wantRevoked: true, wantKey: "fx_cancelled"},
		{ticketID: 5, wantIncluded: true, wantKey: "fx_finished"},
		{ticketID: 6, wantIncluded: true, wantKey: "fx_missing"},
		{ticketID: 7, wantIncluded: false},
		{ticketID: 8, wantIncluded: false},
	}

	for _, tc := range testCases {
		candidate, ok := byTicket[tc.ticketID]
		require.Equal(t, tc.wantIncluded, ok, "工单 %d 的候选纳入判定不符", tc.ticketID)
		if !tc.wantIncluded {
			continue
		}
		require.Equal(t, tc.wantActive, candidate.EngineActive, "工单 %d engine_active", tc.ticketID)
		require.Equal(t, tc.wantRevoked, candidate.EngineRevoked, "工单 %d engine_revoked", tc.ticketID)
		require.Equal(t, tc.wantKey, candidate.ProcessInstanceKey, "工单 %d 实例 Key", tc.ticketID)
	}
}
