package dao

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)

	err = db.AutoMigrate(&Ticket{})
	require.NoError(t, err)
	return db
}

func TestRegisterProcessInstance_TerminalProtection(t *testing.T) {
	db := setupTestDB(t)
	dao := NewTicketDAO(db)
	ctx := context.Background()

	testCases := []struct {
		name          string
		initialStatus uint8
		isFlowx       bool
		expectedFinal uint8
	}{
		{
			name:          "初始工单(START)_注册Flowx实例_成功流转为PROCESS",
			initialStatus: domain.START.ToUint8(),
			isFlowx:       true,
			expectedFinal: domain.PROCESS.ToUint8(),
		},
		{
			name:          "已办结工单(END)_注册Flowx实例_终态保护状态保持为END",
			initialStatus: domain.END.ToUint8(),
			isFlowx:       true,
			expectedFinal: domain.END.ToUint8(),
		},
		{
			name:          "已撤回工单(WITHDRAW)_注册Flowx实例_终态保护状态保持为WITHDRAW",
			initialStatus: domain.WITHDRAW.ToUint8(),
			isFlowx:       true,
			expectedFinal: domain.WITHDRAW.ToUint8(),
		},
		{
			name:          "初始工单(START)_注册EasyFlow实例_成功流转为PROCESS",
			initialStatus: domain.START.ToUint8(),
			isFlowx:       false,
			expectedFinal: domain.PROCESS.ToUint8(),
		},
		{
			name:          "已办结工单(END)_注册EasyFlow实例_终态保护状态保持为END",
			initialStatus: domain.END.ToUint8(),
			isFlowx:       false,
			expectedFinal: domain.END.ToUint8(),
		},
	}

	for idx, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ticketID := int64(1000 + idx)
			now := time.Now().UnixMilli()

			// 预置工单
			err := db.Create(&Ticket{
				Id:         ticketID,
				TenantID:   1,
				BizID:      1,
				Key:        "TICK-TEST",
				TemplateId: 1,
				WorkflowId: 1,
				Status:     tc.initialStatus,
				Ctime:      now,
				Utime:      now,
			}).Error
			require.NoError(t, err)

			// 触发注册绑定
			if tc.isFlowx {
				err = dao.RegisterProcessInstanceKey(ctx, ticketID, "flowx_key_test", domain.PROCESS.ToUint8())
			} else {
				err = dao.RegisterProcessInstanceId(ctx, ticketID, 9999, domain.PROCESS.ToUint8())
			}
			require.NoError(t, err)

			// 校验最终落盘数据
			var saved Ticket
			err = db.First(&saved, ticketID).Error
			require.NoError(t, err)

			assert.Equal(t, tc.expectedFinal, saved.Status, "工单状态必须符合终态保护预期")

			if tc.isFlowx {
				assert.Equal(t, "flowx_key_test", saved.ProcessInstanceKey)
				assert.Equal(t, "flowx", saved.EngineType)
			} else {
				assert.Equal(t, 9999, saved.ProcessInstanceId)
				assert.Equal(t, "easy-workflow", saved.EngineType)
			}
		})
	}
}

// TestUpdateStatusByProcessInstance_TerminalGuard 校验引擎生命周期事件回写工单状态时的终态守卫
// 撤回是发起人主动表达的更强意图，流程办结事件严禁把撤回中/已撤回的工单覆盖回 END
func TestUpdateStatusByProcessInstance_TerminalGuard(t *testing.T) {
	db := setupTestDB(t)
	dao := NewTicketDAO(db)
	ctx := context.Background()

	testCases := []struct {
		name          string
		initialStatus uint8
		targetStatus  uint8
		expectedFinal uint8
	}{
		{
			name:          "流转中工单_收到办结事件_正常收敛为END",
			initialStatus: domain.PROCESS.ToUint8(),
			targetStatus:  domain.END.ToUint8(),
			expectedFinal: domain.END.ToUint8(),
		},
		{
			name:          "撤回中工单_收到办结事件_END不得覆盖撤回意图",
			initialStatus: domain.WITHDRAWING.ToUint8(),
			targetStatus:  domain.END.ToUint8(),
			expectedFinal: domain.WITHDRAWING.ToUint8(),
		},
		{
			name:          "已撤回工单_收到办结事件_终态保持为WITHDRAW",
			initialStatus: domain.WITHDRAW.ToUint8(),
			targetStatus:  domain.END.ToUint8(),
			expectedFinal: domain.WITHDRAW.ToUint8(),
		},
		{
			name:          "已办结工单_收到重复办结事件_终态保持为END",
			initialStatus: domain.END.ToUint8(),
			targetStatus:  domain.END.ToUint8(),
			expectedFinal: domain.END.ToUint8(),
		},
		{
			name:          "撤回中工单_收到驳回撤回事件_允许收敛为WITHDRAW",
			initialStatus: domain.WITHDRAWING.ToUint8(),
			targetStatus:  domain.WITHDRAW.ToUint8(),
			expectedFinal: domain.WITHDRAW.ToUint8(),
		},
		{
			name:          "流转中工单_收到驳回撤回事件_正常收敛为WITHDRAW",
			initialStatus: domain.PROCESS.ToUint8(),
			targetStatus:  domain.WITHDRAW.ToUint8(),
			expectedFinal: domain.WITHDRAW.ToUint8(),
		},
		{
			name:          "已办结工单_收到撤回事件_已归档终态不可改写",
			initialStatus: domain.END.ToUint8(),
			targetStatus:  domain.WITHDRAW.ToUint8(),
			expectedFinal: domain.END.ToUint8(),
		},
	}

	for idx, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ticketID := int64(2000 + idx)
			instanceKey := "flowx_guard_" + tc.name
			now := time.Now().UnixMilli()

			require.NoError(t, db.Create(&Ticket{
				Id:                 ticketID,
				TenantID:           1,
				BizID:              1,
				Key:                "TICK-GUARD",
				TemplateId:         1,
				WorkflowId:         1,
				EngineType:         "flowx",
				ProcessInstanceKey: instanceKey,
				Status:             tc.initialStatus,
				Ctime:              now,
				Utime:              now,
			}).Error)

			require.NoError(t, dao.UpdateStatusByProcessInstance(ctx, instanceKey, tc.targetStatus, now))

			var saved Ticket
			require.NoError(t, db.First(&saved, ticketID).Error)
			assert.Equal(t, tc.expectedFinal, saved.Status, "工单状态必须符合终态守卫预期")
		})
	}
}

// TestRevokeProcessBindingByKey_PreciseMatch 校验启动失败回滚只允许解绑本次自己写入的实例 Key
func TestRevokeProcessBindingByKey_PreciseMatch(t *testing.T) {
	db := setupTestDB(t)
	dao := NewTicketDAO(db)
	ctx := context.Background()

	testCases := []struct {
		name             string
		boundKey         string
		initialStatus    uint8
		revokeKey        string
		expectedStatus   uint8
		expectedBoundKey string
	}{
		{
			name:             "精确匹配预绑定Key_解绑并复位为待启动态",
			boundKey:         "flowx_revoke_a",
			initialStatus:    domain.PROCESS.ToUint8(),
			revokeKey:        "flowx_revoke_a",
			expectedStatus:   domain.START.ToUint8(),
			expectedBoundKey: "",
		},
		{
			name:             "Key不匹配_严禁误清其他正在流转的流程绑定",
			boundKey:         "flowx_revoke_b",
			initialStatus:    domain.PROCESS.ToUint8(),
			revokeKey:        "flowx_revoke_other",
			expectedStatus:   domain.PROCESS.ToUint8(),
			expectedBoundKey: "flowx_revoke_b",
		},
		{
			name:             "工单已秒级办结_解绑死链但终态状态不得回退",
			boundKey:         "flowx_revoke_c",
			initialStatus:    domain.END.ToUint8(),
			revokeKey:        "flowx_revoke_c",
			expectedStatus:   domain.END.ToUint8(),
			expectedBoundKey: "",
		},
	}

	for idx, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ticketID := int64(3000 + idx)
			now := time.Now().UnixMilli()

			require.NoError(t, db.Create(&Ticket{
				Id:                 ticketID,
				TenantID:           1,
				BizID:              1,
				Key:                "TICK-REVOKE",
				TemplateId:         1,
				WorkflowId:         1,
				EngineType:         "flowx",
				ProcessInstanceKey: tc.boundKey,
				Status:             tc.initialStatus,
				Ctime:              now,
				Utime:              now,
			}).Error)

			require.NoError(t, dao.RevokeProcessBindingByKey(ctx, ticketID, tc.revokeKey))

			var saved Ticket
			require.NoError(t, db.First(&saved, ticketID).Error)
			assert.Equal(t, tc.expectedStatus, saved.Status)
			assert.Equal(t, tc.expectedBoundKey, saved.ProcessInstanceKey)
		})
	}
}
