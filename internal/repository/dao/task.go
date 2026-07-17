package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"gorm.io/gorm"
)

// Task 是流程自动化节点的持久化实体，不保存执行器配置和日志镜像。
type Task struct {
	ID                int64  `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'自动化任务主键'"`
	TenantID          int64  `gorm:"column:tenant_id;type:bigint;not null;uniqueIndex:uk_automation_node,priority:1;index;comment:'租户 ID'"`
	TicketID          int64  `gorm:"column:ticket_id;type:bigint;not null;index;comment:'关联工单 ID'"`
	ProcessInstanceID int    `gorm:"column:process_instance_id;type:int;not null;uniqueIndex:uk_automation_node,priority:2;index;comment:'流程实例 ID'"`
	NodeID            string `gorm:"column:node_id;type:varchar(128);not null;uniqueIndex:uk_automation_node,priority:3;comment:'自动化节点 ID'"`
	NodeName          string `gorm:"column:node_name;type:varchar(250);not null;comment:'自动化节点名称快照'"`
	ProcessVersion    int    `gorm:"column:process_version;type:int;not null;comment:'流程实例版本快照'"`
	Status            uint8  `gorm:"column:status;type:tinyint unsigned;not null;index;comment:'编排状态'"`
	Phase             string `gorm:"column:phase;type:varchar(32);not null;comment:'最近编排阶段'"`
	ScheduledAt       int64  `gorm:"column:scheduled_at;type:bigint;not null;index;comment:'计划提交时间'"`
	CurrentAttemptID  int64  `gorm:"column:current_attempt_id;type:bigint;not null;default:0;index;comment:'当前执行尝试 ID'"`
	AdvancedAt        int64  `gorm:"column:advanced_at;type:bigint;not null;default:0;index;comment:'流程推进完成时间'"`
	LastError         string `gorm:"column:last_error;type:text;comment:'最近编排错误'"`
	CTime             int64  `gorm:"column:ctime;type:bigint;comment:'创建时间'"`
	UTime             int64  `gorm:"column:utime;type:bigint;comment:'更新时间'"`
}

// TableName 返回新自动化任务表名。
func (Task) TableName() string { return "automation_tasks" }

// TaskDAO 定义自动化任务编排状态的持久化能力。
type TaskDAO interface {
	// Create 创建自动化任务。
	Create(ctx context.Context, task Task) (Task, error)
	// FindByProcessNode 根据流程实例和节点查询任务。
	FindByProcessNode(ctx context.Context, processInstanceID int, nodeID string) (Task, error)
	// FindByID 根据主键查询任务。
	FindByID(ctx context.Context, id int64) (Task, error)
	// Block 将任务置为需要人工处理的阻塞状态。
	Block(ctx context.Context, id int64, reason string) error
	// PrepareRetry 将失败或阻塞任务置为等待重试状态。
	PrepareRetry(ctx context.Context, id int64) error
	// List 分页查询任务。
	List(ctx context.Context, offset, limit int64) ([]Task, error)
	// ListByStatusAfterID 按主键游标查询指定状态任务。
	ListByStatusAfterID(ctx context.Context, status uint8, afterID, limit int64) ([]Task, error)
	// Count 统计任务总数。
	Count(ctx context.Context) (int64, error)
	// ListByInstanceID 查询流程实例下的任务。
	ListByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]Task, error)
	// CountByInstanceID 统计流程实例下的任务数量。
	CountByInstanceID(ctx context.Context, instanceID int) (int64, error)
	// ListReady 查询已到计划时间的待提交任务。
	ListReady(ctx context.Context, limit int64) ([]Task, error)
	// ListSucceededUnadvanced 按主键游标查询一批尚未推进流程的成功任务。
	ListSucceededUnadvanced(ctx context.Context, limit, afterID int64) ([]Task, error)
	// MarkAdvanced 标记任务已经推进流程。
	MarkAdvanced(ctx context.Context, id int64) error
}

type gormTaskDAO struct{ db *gorm.DB }

// NewTaskDAO 创建自动化任务 DAO。
func NewTaskDAO(db *gorm.DB) TaskDAO { return &gormTaskDAO{db: db} }

func (g *gormTaskDAO) Create(ctx context.Context, task Task) (Task, error) {
	now := time.Now().UnixMilli()
	task.CTime, task.UTime = now, now
	if task.Status == 0 || task.Phase == "" {
		return Task{}, fmt.Errorf("自动化任务初始状态不能为空")
	}
	err := g.db.WithContext(ctx).Create(&task).Error
	return task, err
}

func (g *gormTaskDAO) FindByProcessNode(ctx context.Context, processInstanceID int, nodeID string) (Task, error) {
	var task Task
	err := g.db.WithContext(ctx).
		Where("process_instance_id = ? AND node_id = ?", processInstanceID, nodeID).
		First(&task).Error
	return task, err
}

func (g *gormTaskDAO) FindByID(ctx context.Context, id int64) (Task, error) {
	var task Task
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&task).Error
	return task, err
}

func (g *gormTaskDAO) Block(ctx context.Context, id int64, reason string) error {
	return g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"status": domain.TaskStatusBlocked.ToUint8(), "phase": domain.TaskPhaseBlocked,
		"last_error": reason, "utime": time.Now().UnixMilli(),
	}).Error
}

func (g *gormTaskDAO) PrepareRetry(ctx context.Context, id int64) error {
	result := g.db.WithContext(ctx).Model(&Task{}).
		Where("id = ? AND status IN (?, ?)", id,
			domain.TaskStatusFailed.ToUint8(), domain.TaskStatusBlocked.ToUint8()).
		Updates(map[string]any{
			"status": domain.TaskStatusWaiting.ToUint8(), "phase": domain.TaskPhaseRetrying,
			"last_error": "", "utime": time.Now().UnixMilli(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("自动化任务 %d 状态已变化，无法创建重试", id)
	}
	return nil
}

func (g *gormTaskDAO) List(ctx context.Context, offset, limit int64) ([]Task, error) {
	var tasks []Task
	err := g.db.WithContext(ctx).Order("ctime DESC").Offset(int(offset)).Limit(int(limit)).Find(&tasks).Error
	return tasks, err
}

func (g *gormTaskDAO) ListByStatusAfterID(ctx context.Context, status uint8,
	afterID, limit int64) ([]Task, error) {
	var tasks []Task
	err := g.db.WithContext(ctx).Where("status = ? AND id > ?", status, afterID).
		Order("id ASC").Limit(int(limit)).Find(&tasks).Error
	return tasks, err
}

func (g *gormTaskDAO) Count(ctx context.Context) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) ListByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]Task, error) {
	var tasks []Task
	err := g.db.WithContext(ctx).Where("process_instance_id = ?", instanceID).
		Order("ctime DESC").Offset(int(offset)).Limit(int(limit)).Find(&tasks).Error
	return tasks, err
}

func (g *gormTaskDAO) CountByInstanceID(ctx context.Context, instanceID int) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).Where("process_instance_id = ?", instanceID).Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) ListReady(ctx context.Context, limit int64) ([]Task, error) {
	var tasks []Task
	err := g.db.WithContext(ctx).
		Where("status = ? AND scheduled_at <= ?", domain.TaskStatusWaiting.ToUint8(), time.Now().UnixMilli()).
		Order("scheduled_at ASC, ctime ASC").Limit(int(limit)).Find(&tasks).Error
	return tasks, err
}

func (g *gormTaskDAO) ListSucceededUnadvanced(ctx context.Context, limit, afterID int64) ([]Task, error) {
	var tasks []Task
	err := g.db.WithContext(ctx).
		Where("status = ? AND advanced_at = 0 AND id > ?", domain.TaskStatusSuccess.ToUint8(), afterID).
		Order("id ASC").Limit(int(limit)).Find(&tasks).Error
	return tasks, err
}

func (g *gormTaskDAO) MarkAdvanced(ctx context.Context, id int64) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Model(&Task{}).Where("id = ? AND advanced_at = 0", id).
		Updates(map[string]any{"advanced_at": now, "utime": now}).Error
}
