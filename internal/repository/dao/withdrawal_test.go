package dao

import (
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestWithdrawalPendingStatusesExpandAsSQLList(t *testing.T) {
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(localhost:3306)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	require.NoError(t, err)

	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Model(&Task{}).
			Where("process_instance_id = ? AND status IN ?", 2663, withdrawalPendingStatuses()).
			Update("status", 7)
	})

	require.Contains(t, sql, "status IN (4,5,2)")
	require.NotContains(t, sql, "<binary>")
}
