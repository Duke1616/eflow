package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
	"gorm.io/gorm"
)

var ErrTaskNotFound = gorm.ErrRecordNotFound

// TaskRepository 定义流程自动化节点的编排状态持久化能力。
type TaskRepository interface {
	// FindOrCreate 按流程实例和节点幂等创建任务，并返回本次是否实际创建。
	FindOrCreate(ctx context.Context, task domain.Task) (domain.Task, bool, error)
	// FindByID 根据主键查询任务。
	FindByID(ctx context.Context, id int64) (domain.Task, error)
	// FindByProcessNode 根据流程实例和节点查询任务。
	FindByProcessNode(ctx context.Context, processInstanceID int, nodeID string) (domain.Task, error)
	// Block 将任务置为需要人工处理的阻塞状态。
	Block(ctx context.Context, id int64, reason string) error
	// PrepareRetry 将失败或阻塞任务置为等待重试状态。
	PrepareRetry(ctx context.Context, id int64) error
	// List 分页查询任务。
	List(ctx context.Context, offset, limit int64) ([]domain.Task, error)
	// ListByStatusAfterID 按主键游标查询指定状态任务。
	ListByStatusAfterID(ctx context.Context, status domain.TaskStatus,
		afterID, limit int64) ([]domain.Task, error)
	// Count 统计任务总数。
	Count(ctx context.Context) (int64, error)
	// ListByInstanceID 查询流程实例下的任务。
	ListByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]domain.Task, error)
	// CountByInstanceID 统计流程实例下的任务数量。
	CountByInstanceID(ctx context.Context, instanceID int) (int64, error)
	// ListReady 查询已到计划时间的待提交任务。
	ListReady(ctx context.Context, limit int64) ([]domain.Task, error)
	// ListSucceededUnadvanced 按主键游标查询一批尚未推进流程的成功任务。
	ListSucceededUnadvanced(ctx context.Context, limit, afterID int64) ([]domain.Task, error)
	// MarkAdvanced 标记任务已经推进流程。
	MarkAdvanced(ctx context.Context, id int64) error
}

type taskRepository struct{ dao dao.TaskDAO }

// NewTaskRepository 创建自动化任务仓储。
func NewTaskRepository(taskDAO dao.TaskDAO) TaskRepository {
	return &taskRepository{dao: taskDAO}
}

func (r *taskRepository) FindOrCreate(ctx context.Context, task domain.Task) (domain.Task, bool, error) {
	entity, err := r.dao.FindByProcessNode(ctx, task.ProcessInstanceID, task.NodeID)
	if err == nil {
		return toTaskDomain(entity), false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, false, err
	}
	created, err := r.dao.Create(ctx, toTaskEntity(task))
	if err == nil {
		return toTaskDomain(created), true, nil
	}
	// 唯一约束处理并发创建，失败方读取已存在记录。
	entity, findErr := r.dao.FindByProcessNode(ctx, task.ProcessInstanceID, task.NodeID)
	if findErr != nil {
		return domain.Task{}, false, err
	}
	return toTaskDomain(entity), false, nil
}

func (r *taskRepository) FindByID(ctx context.Context, id int64) (domain.Task, error) {
	entity, err := r.dao.FindByID(ctx, id)
	return toTaskDomain(entity), err
}

func (r *taskRepository) FindByProcessNode(ctx context.Context, processInstanceID int,
	nodeID string) (domain.Task, error) {
	entity, err := r.dao.FindByProcessNode(ctx, processInstanceID, nodeID)
	return toTaskDomain(entity), err
}

func (r *taskRepository) Block(ctx context.Context, id int64, reason string) error {
	return r.dao.Block(ctx, id, reason)
}

func (r *taskRepository) PrepareRetry(ctx context.Context, id int64) error {
	return r.dao.PrepareRetry(ctx, id)
}

func (r *taskRepository) List(ctx context.Context, offset, limit int64) ([]domain.Task, error) {
	entities, err := r.dao.List(ctx, offset, limit)
	return mapTasks(entities), err
}

func (r *taskRepository) ListByStatusAfterID(ctx context.Context, status domain.TaskStatus,
	afterID, limit int64) ([]domain.Task, error) {
	entities, err := r.dao.ListByStatusAfterID(ctx, status.ToUint8(), afterID, limit)
	return mapTasks(entities), err
}

func (r *taskRepository) Count(ctx context.Context) (int64, error) {
	return r.dao.Count(ctx)
}

func (r *taskRepository) ListByInstanceID(ctx context.Context, offset, limit int64,
	instanceID int) ([]domain.Task, error) {
	entities, err := r.dao.ListByInstanceID(ctx, offset, limit, instanceID)
	return mapTasks(entities), err
}

func (r *taskRepository) CountByInstanceID(ctx context.Context, instanceID int) (int64, error) {
	return r.dao.CountByInstanceID(ctx, instanceID)
}

func (r *taskRepository) ListReady(ctx context.Context, limit int64) ([]domain.Task, error) {
	entities, err := r.dao.ListReady(ctx, limit)
	return mapTasks(entities), err
}

func (r *taskRepository) ListSucceededUnadvanced(ctx context.Context, limit,
	afterID int64) ([]domain.Task, error) {
	entities, err := r.dao.ListSucceededUnadvanced(ctx, limit, afterID)
	return mapTasks(entities), err
}

func (r *taskRepository) MarkAdvanced(ctx context.Context, id int64) error {
	return r.dao.MarkAdvanced(ctx, id)
}

func mapTasks(entities []dao.Task) []domain.Task {
	return slice.Map(entities, func(_ int, entity dao.Task) domain.Task { return toTaskDomain(entity) })
}

func toTaskEntity(task domain.Task) dao.Task {
	return dao.Task{
		ID: task.ID, TenantID: task.TenantID, TicketID: task.TicketID,
		ProcessInstanceID: task.ProcessInstanceID, NodeID: task.NodeID,
		NodeName: task.NodeName, ProcessVersion: task.ProcessVersion,
		Status: task.Status.ToUint8(), Phase: string(task.Phase), ScheduledAt: task.ScheduledAt,
		CurrentAttemptID: task.CurrentAttemptID, AdvancedAt: task.AdvancedAt,
		LastError: task.LastError, CTime: task.CTime, UTime: task.UTime,
	}
}

func toTaskDomain(task dao.Task) domain.Task {
	return domain.Task{
		ID: task.ID, TenantID: task.TenantID, TicketID: task.TicketID,
		ProcessInstanceID: task.ProcessInstanceID, NodeID: task.NodeID,
		NodeName: task.NodeName, ProcessVersion: task.ProcessVersion,
		Status: domain.TaskStatus(task.Status), Phase: domain.TaskPhase(task.Phase),
		ScheduledAt: task.ScheduledAt, CurrentAttemptID: task.CurrentAttemptID,
		AdvancedAt: task.AdvancedAt, LastError: task.LastError,
		CTime: task.CTime, UTime: task.UTime,
	}
}
