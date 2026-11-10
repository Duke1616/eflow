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
