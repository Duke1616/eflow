package migrations

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/Duke1616/eiam/pkg/migration"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const legacyTaskStatusSuccess uint8 = 1

// legacyTask 是旧版本 task 表的只读映射，只保留新模型迁移所需字段。
type legacyTask struct {
	ID              int64                           `gorm:"column:id"`
	TenantID        int64                           `gorm:"column:tenant_id"`
	TicketID        int64                           `gorm:"column:ticket_id"`
	ProcessInstID   int                             `gorm:"column:process_inst_id"`
	CurrentNodeID   string                          `gorm:"column:current_node_id"`
	TriggerPosition string                          `gorm:"column:trigger_position"`
	Args            sqlx.JsonField[domain.TaskArgs] `gorm:"column:args;type:json"`
	Status          uint8                           `gorm:"column:status"`
	Result          string                          `gorm:"column:result"`
	StartTime       int64                           `gorm:"column:start_time"`
	EndTime         int64                           `gorm:"column:end_time"`
	ScheduledTime   int64                           `gorm:"column:scheduled_time"`
	CTime           int64                           `gorm:"column:ctime"`
	UTime           int64                           `gorm:"column:utime"`
}

func (legacyTask) TableName() string { return "task" }

type legacyTaskMigrator struct{}

// NewLegacyTaskMigrator 创建旧自动化任务拆表迁移器。
func NewLegacyTaskMigrator() migration.Migrator { return legacyTaskMigrator{} }

func (legacyTaskMigrator) Name() string     { return "automation_task_v2_mysql" }
func (legacyTaskMigrator) Destination() any { return &dao.Task{} }

func (legacyTaskMigrator) Migrate(ctx context.Context,
	env migration.MigrationEnv) (migration.Result, error) {
	// 旧 task 已经由早期 Mongo 迁移写入当前目标 MySQL，并在旧版本运行期间持续产生数据。
	// 本迁移只在目标 MySQL 内拆表，不再回读 Mongo 或 migration.source.mysql。
	if !env.MySQLDst.Migrator().HasTable("task") {
		log.Println("当前 MySQL 未发现旧 task 表，跳过自动化任务迁移")
		return migration.Result{}, nil
	}
	if !env.DryRun && (!env.MySQLDst.Migrator().HasTable(&dao.Task{}) ||
		!env.MySQLDst.Migrator().HasTable(&dao.TaskAttempt{})) {
		return migration.Result{}, fmt.Errorf("目标自动化任务表尚未初始化，请开启 migration.auto_migrate")
	}
	watermark, err := readLegacyTaskWatermark(ctx, env.MySQLDst)
	if err != nil {
		return migration.Result{}, err
	}
	if watermark.MaxID <= 0 {
		log.Println("旧 task 表没有数据，跳过自动化任务迁移")
		return migration.Result{}, nil
	}
	log.Printf("旧 task 表迁移水位: max_id=%d max_utime=%d", watermark.MaxID, watermark.MaxUTime)

	defaultTenantID := DefaultTenantID
	if env.DefaultTenantID != nil && *env.DefaultTenantID > 0 {
		defaultTenantID = *env.DefaultTenantID
	}
	var skipped, existing int64
	invalidSamples := make([]string, 0, 10)
	result := migration.Result{}
	var afterID int64
	for {
		var batch []legacyTask
		err := env.MySQLDst.WithContext(ctx).Table("task").
			Where("id > ? AND id <= ?", afterID, watermark.MaxID).Order("id ASC").
			Limit(env.BatchSize).Find(&batch).Error
		if err != nil {
			return result, fmt.Errorf("读取旧 task 表失败: %w", err)
		}
		if len(batch) == 0 {
			break
		}
		result.Read += int64(len(batch))
		for _, source := range batch {
			invalidReason := validateLegacyTaskIdentity(source)
			if invalidReason != "" {
				skipped++
				if len(invalidSamples) < 10 {
					invalidSamples = append(invalidSamples,
						fmt.Sprintf("%d(%s)", source.ID, invalidReason))
				}
				continue
			}
			task, attempt := convertLegacyTask(source, defaultTenantID)
			result.Converted++
			if env.DryRun {
				continue
			}
			written, err := persistLegacyTask(ctx, env.MySQLDst, task, attempt)
			if err != nil {
				return result, fmt.Errorf("写入旧任务 %d 失败: %w", source.ID, err)
			}
			if written {
				result.Written++
			} else {
				existing++
			}
		}
		afterID = batch[len(batch)-1].ID
	}
	if err := ensureLegacyTaskUnchanged(ctx, env.MySQLDst, watermark); err != nil {
		return result, err
	}
	log.Printf("旧自动化任务迁移汇总: valid=%d written=%d existing=%d skipped=%d skipped_samples=%v dry_run=%t",
		result.Converted, result.Written, existing, skipped, invalidSamples, env.DryRun)
	return result, nil
}

type legacyTaskWatermark struct {
	MaxID    int64 `gorm:"column:max_id"`
	MaxUTime int64 `gorm:"column:max_utime"`
}

func readLegacyTaskWatermark(ctx context.Context, db *gorm.DB) (legacyTaskWatermark, error) {
	var watermark legacyTaskWatermark
	err := db.WithContext(ctx).Table("task").
		Select("COALESCE(MAX(id), 0) AS max_id, COALESCE(MAX(utime), 0) AS max_utime").
		Scan(&watermark).Error
	if err != nil {
		return legacyTaskWatermark{}, fmt.Errorf("读取旧 task 表迁移水位失败: %w", err)
	}
	return watermark, nil
}

func ensureLegacyTaskUnchanged(ctx context.Context, db *gorm.DB, before legacyTaskWatermark) error {
	after, err := readLegacyTaskWatermark(ctx, db)
	if err != nil {
		return err
	}
	if after.MaxID != before.MaxID || after.MaxUTime != before.MaxUTime {
		return fmt.Errorf("迁移期间旧 task 表仍在写入，请停止旧服务后重新执行迁移: before=(%d,%d) after=(%d,%d)",
			before.MaxID, before.MaxUTime, after.MaxID, after.MaxUTime)
	}
	return nil
}

func convertLegacyTask(source legacyTask, defaultTenantID int64) (dao.Task, dao.TaskAttempt) {
	tenantID := source.TenantID
	if tenantID <= 0 {
		tenantID = defaultTenantID
	}
	status, phase, advancedAt, reason := mapLegacyTaskState(source)
	attemptStatus := domain.AttemptStatusFailed
	if source.Status == legacyTaskStatusSuccess {
		attemptStatus = domain.AttemptStatusSuccess
	}
	task := dao.Task{
		TenantID: tenantID, TicketID: source.TicketID,
		ProcessInstanceID: source.ProcessInstID, NodeID: strings.TrimSpace(source.CurrentNodeID),
		NodeName: "自动化任务", Status: status.ToUint8(), Phase: string(phase),
		ScheduledAt: source.ScheduledTime, AdvancedAt: advancedAt, LastError: reason,
		CTime: source.CTime, UTime: firstPositive(source.UTime, source.CTime),
	}
	attempt := dao.TaskAttempt{
		TenantID: tenantID, AttemptNo: 1,
		RequestID: fmt.Sprintf("legacy:%d:%d", tenantID, source.ID),
		Status:    string(attemptStatus),
		Input:     sqlx.JsonField[domain.TaskArgs]{Val: source.Args.Val, Valid: source.Args.Valid},
		Output:    source.Result, Error: reason, SubmittedAt: source.StartTime,
		CompletedAt: firstPositive(source.EndTime, source.UTime, source.CTime),
		CTime:       firstPositive(source.StartTime, source.CTime),
		UTime:       firstPositive(source.UTime, source.EndTime, source.CTime),
	}
	return task, attempt
}

func mapLegacyTaskState(source legacyTask) (domain.TaskStatus, domain.TaskPhase, int64, string) {
	if source.Status == legacyTaskStatusSuccess {
		advancedAt := firstPositive(source.EndTime, source.UTime, source.CTime, 1)
		return domain.TaskStatusSuccess, domain.TaskPhaseSucceeded, advancedAt, ""
	}
	reason := strings.TrimSpace(source.Result)
	if reason == "" {
		reason = strings.TrimSpace(source.TriggerPosition)
	}
	if reason == "" {
		reason = "旧任务没有保存错误详情"
	}
	reason = truncateRunes("历史任务迁移："+reason, 16000)
	return domain.TaskStatusBlocked, domain.TaskPhaseBlocked, 0, reason
}

func persistLegacyTask(ctx context.Context, db *gorm.DB, task dao.Task,
	attempt dao.TaskAttempt) (bool, error) {
	written := false
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&task)
		if created.Error != nil {
			return created.Error
		}
		if created.RowsAffected == 0 {
			return nil
		}
		attempt.TaskID = task.ID
		if err := tx.Create(&attempt).Error; err != nil {
			return err
		}
		if err := tx.Model(&dao.Task{}).Where("id = ?", task.ID).
			Update("current_attempt_id", attempt.ID).Error; err != nil {
			return err
		}
		written = true
		return nil
	})
	return written, err
}

func validateLegacyTaskIdentity(task legacyTask) string {
	missing := make([]string, 0, 3)
	if task.TicketID <= 0 {
		missing = append(missing, "缺少工单")
	}
	if task.ProcessInstID <= 0 {
		missing = append(missing, "缺少流程实例")
	}
	if strings.TrimSpace(task.CurrentNodeID) == "" {
		missing = append(missing, "缺少节点 ID")
	}
	return strings.Join(missing, "、")
}

func firstPositive(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

// TruncateLegacyTaskAttempts 在全量重建模式下先清空新 Attempt 表。
func TruncateLegacyTaskAttempts(ctx context.Context, env migration.MigrationEnv) error {
	if env.DryRun || !env.MySQLDst.Migrator().HasTable(&dao.TaskAttempt{}) {
		return nil
	}
	if err := env.MySQLDst.WithContext(ctx).Exec("TRUNCATE TABLE automation_task_attempts").Error; err != nil {
		return fmt.Errorf("清空 automation_task_attempts 失败: %w", err)
	}
	return nil
}
