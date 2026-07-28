package dao

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrTicketNotProcessing = errors.New("工单不处于可撤回状态")
	ErrAutomationRunning   = errors.New("存在正在执行且无法取消的自动化任务")
)

// WithdrawalDAO 持久化工单撤回状态机及其关联自动化任务迁移。
type WithdrawalDAO interface {
	Prepare(ctx context.Context, processInstanceID int) error
	ActivateCompensations(ctx context.Context, processInstanceID int, nodeIDs []string) error
	TryFinalize(ctx context.Context, processInstanceID int) (bool, error)
	Rollback(ctx context.Context, processInstanceID int) error
	ListStale(ctx context.Context, before int64, afterID, limit int64) ([]WithdrawalCandidate, error)
}

// WithdrawalCandidate 汇总工单与 easy-workflow 活动、历史实例状态。
type WithdrawalCandidate struct {
	TicketID          int64 `gorm:"column:ticket_id"`
	TenantID          int64 `gorm:"column:tenant_id"`
	ProcessInstanceID int   `gorm:"column:process_instance_id"`
	EngineActive      bool  `gorm:"column:engine_active"`
	EngineRevoked     bool  `gorm:"column:engine_revoked"`
}

type gormWithdrawalDAO struct{ db *gorm.DB }

func NewWithdrawalDAO(db *gorm.DB) WithdrawalDAO { return &gormWithdrawalDAO{db: db} }

func (g *gormWithdrawalDAO) Prepare(ctx context.Context, processInstanceID int) error {
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("process_instance_id = ?", processInstanceID).First(&ticket).Error; err != nil {
			return err
		}
		if domain.Status(ticket.Status) != domain.PROCESS {
			return fmt.Errorf("%w: status=%d", ErrTicketNotProcessing, ticket.Status)
		}

		// 只有成功动作所关联的补偿节点可以在撤回时继续运行。
		var active int64
		if err := tx.Model(&Task{}).
			Where(`process_instance_id = ? AND status IN (?, ?) AND NOT EXISTS (
				SELECT 1 FROM automation_tasks AS source
				WHERE source.process_instance_id = automation_tasks.process_instance_id
					AND source.status = ?
					AND source.compensation_node_id = automation_tasks.node_id
			)`,
				processInstanceID, domain.TaskStatusSubmitting.ToUint8(),
				domain.TaskStatusRunning.ToUint8(), domain.TaskStatusSuccess.ToUint8()).
			Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return ErrAutomationRunning
		}

		if err := tx.Model(&Ticket{}).Where("id = ? AND status = ?", ticket.Id, domain.PROCESS.ToUint8()).
			Updates(map[string]any{
				"status": domain.WITHDRAWING.ToUint8(), "utime": time.Now().UnixMilli(),
			}).Error; err != nil {
			return err
		}
		// 冻结阶段不允许任何待执行任务穿过撤回边界；补偿计划在引擎撤回后重新激活。
		return tx.Model(&Task{}).Where("process_instance_id = ?", processInstanceID).
			Update("execution_kind", domain.TaskExecutionProcess).Error
	})
}

func (g *gormWithdrawalDAO) ActivateCompensations(ctx context.Context,
	processInstanceID int, nodeIDs []string) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("process_instance_id = ?", processInstanceID).First(&ticket).Error; err != nil {
			return err
		}
		if domain.Status(ticket.Status) == domain.WITHDRAW {
			return nil
		}
		if domain.Status(ticket.Status) != domain.WITHDRAWING {
			return fmt.Errorf("%w: status=%d", ErrTicketNotProcessing, ticket.Status)
		}

		pending := withdrawalPendingStatuses()
		normalTasks := tx.Model(&Task{}).
			Where("process_instance_id = ? AND status IN ?", processInstanceID, pending)
		if len(nodeIDs) > 0 {
			normalTasks = normalTasks.Where("node_id NOT IN ?", nodeIDs)
		}
		if err := normalTasks.
			Updates(map[string]any{
				"status": domain.TaskStatusCancelled.ToUint8(), "phase": domain.TaskPhaseCancelled,
				"cancelled_at": now, "last_error": "", "utime": now,
			}).Error; err != nil {
			return err
		}
		if len(nodeIDs) == 0 {
			return nil
		}
		// execution_kind=COMPENSATION 同时充当激活标记，恢复任务重复执行时不会重置失败状态。
		if err := tx.Model(&Task{}).
			Where("process_instance_id = ? AND node_id IN ? AND execution_kind <> ? AND status IN ?",
				processInstanceID, nodeIDs, domain.TaskExecutionCompensation, pending).
			Updates(map[string]any{
				"status": domain.TaskStatusWaiting.ToUint8(), "phase": domain.TaskPhaseReady,
				"original_scheduled_at": gorm.Expr(
					"CASE WHEN original_scheduled_at = 0 THEN scheduled_at ELSE original_scheduled_at END"),
				"scheduled_at": now, "last_error": "", "utime": now,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&Task{}).
			Where("process_instance_id = ? AND node_id IN ?", processInstanceID, nodeIDs).
			Update("execution_kind", domain.TaskExecutionCompensation).Error
	})
}

// withdrawalPendingStatuses 必须返回 []int。GORM 会将 []uint8 识别为二进制数据，
// 进而生成 status IN '<binary>'，而不是展开成 SQL IN 列表。
func withdrawalPendingStatuses() []int {
	return []int{
		int(domain.TaskStatusWaiting), int(domain.TaskStatusBlocked), int(domain.TaskStatusFailed),
	}
}

func (g *gormWithdrawalDAO) TryFinalize(ctx context.Context, processInstanceID int) (bool, error) {
	completed := false
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var ticket Ticket
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("process_instance_id = ?", processInstanceID).First(&ticket).Error; err != nil {
			return err
		}
		if domain.Status(ticket.Status) == domain.WITHDRAW {
			completed = true
			return nil
		}
		if domain.Status(ticket.Status) != domain.WITHDRAWING {
			return fmt.Errorf("%w: status=%d", ErrTicketNotProcessing, ticket.Status)
		}

		var unfinished int64
		if err := tx.Model(&Task{}).
			Where("process_instance_id = ? AND execution_kind = ? AND status <> ?",
				processInstanceID, domain.TaskExecutionCompensation, domain.TaskStatusSuccess.ToUint8()).
			Count(&unfinished).Error; err != nil {
			return err
		}
		if unfinished > 0 {
			return nil
		}
		now := time.Now().UnixMilli()
		result := tx.Model(&Ticket{}).
			Where("id = ? AND status = ?", ticket.Id, domain.WITHDRAWING.ToUint8()).
			Updates(map[string]any{"status": domain.WITHDRAW.ToUint8(), "utime": now, "wtime": now})
		completed = result.Error == nil && result.RowsAffected > 0
		return result.Error
	})
	return completed, err
}

func (g *gormWithdrawalDAO) Rollback(ctx context.Context, processInstanceID int) error {
	return g.db.WithContext(ctx).Model(&Ticket{}).
		Where("process_instance_id = ? AND status = ?", processInstanceID, domain.WITHDRAWING.ToUint8()).
		Updates(map[string]any{
			"status": domain.PROCESS.ToUint8(), "utime": time.Now().UnixMilli(),
		}).Error
}

func (g *gormWithdrawalDAO) ListStale(ctx context.Context, before int64,
	afterID, limit int64) ([]WithdrawalCandidate, error) {
	var candidates []WithdrawalCandidate
	err := g.db.WithContext(ctx).Model(&Ticket{}).
		Select(`ticket.id AS ticket_id, ticket.tenant_id, ticket.process_instance_id,
			EXISTS(SELECT 1 FROM proc_inst pi WHERE pi.id = ticket.process_instance_id) AS engine_active,
			EXISTS(SELECT 1 FROM hist_proc_inst hpi
				WHERE hpi.proc_inst_id = ticket.process_instance_id AND hpi.status = 2) AS engine_revoked`).
		Where("ticket.status = ? AND ticket.utime <= ? AND ticket.id > ?",
			domain.WITHDRAWING.ToUint8(), before, afterID).
		Order("ticket.id ASC").Limit(int(limit)).Scan(&candidates).Error
	return candidates, err
}
