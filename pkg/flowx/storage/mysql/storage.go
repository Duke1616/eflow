package mysql

import (
	"context"
	"errors"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
	"github.com/samber/lo"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Storage 基于 GORM 的 MySQL 工业级持久化驱动
type Storage struct {
	db *gorm.DB
}

// 编译期严格校验是否完整实现了 IStorage 与 ITxStorage 接口
var _ storage.IStorage = (*Storage)(nil)
var _ storage.ITxStorage = (*Storage)(nil)

// NewStorage 实例化 MySQL 存储驱动
func NewStorage(db *gorm.DB) *Storage {
	return &Storage{db: db}
}

// AutoMigrate 自动迁移创建工作流核心表结构
func (s *Storage) AutoMigrate() error {
	return s.db.AutoMigrate(&FlowDefinitionPO{}, &FlowInstancePO{}, &FlowTaskPO{})
}

// WithTx 开启事务穿透，允许与外层业务事务共享同一 DB 物理连接
func (s *Storage) WithTx(tx *gorm.DB) *Storage {
	return &Storage{db: tx}
}

// DoTx 基于 GORM 的事务闭包，若 fn 返回错误或发生 panic 则自动回滚，否则原子提交
func (s *Storage) DoTx(ctx context.Context, fn func(txStorage storage.IStorage) error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(s.WithTx(tx))
	})
}

func (s *Storage) SaveDefinition(ctx context.Context, def model.ProcessDefinition) error {
	po, err := toDefinitionPO(def)
	if err != nil {
		return err
	}
	return s.buildSaveDefinitionQuery(s.db.WithContext(ctx), &po).Error
}

func (s *Storage) buildSaveDefinitionQuery(tx *gorm.DB, po *FlowDefinitionPO) *gorm.DB {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "proc_key"}, {Name: "version"}},
		DoUpdates: clause.AssignmentColumns([]string{"name", "dsl_content"}),
	}).Create(po)
}

func (s *Storage) GetDefinition(ctx context.Context, key string, version int) (model.ProcessDefinition, error) {
	var po FlowDefinitionPO
	err := s.db.WithContext(ctx).
		Where("proc_key = ? AND version = ?", key, version).
		First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.ProcessDefinition{}, storage.ErrDefinitionNotFound
		}
		return model.ProcessDefinition{}, err
	}
	return toDefinitionDO(po)
}

// GetLatestVersion 根据流程业务唯一标识查询已发布的最大版本号，尚未发布过任何版本时返回 0, nil
func (s *Storage) GetLatestVersion(ctx context.Context, key string) (int, error) {
	var maxVersion *int
	err := s.db.WithContext(ctx).
		Model(&FlowDefinitionPO{}).
		Where("proc_key = ?", key).
		Select("MAX(version)").
		Scan(&maxVersion).Error
	if err != nil {
		return 0, err
	}
	if maxVersion == nil {
		return 0, nil
	}
	return *maxVersion, nil
}

func (s *Storage) SaveInstance(ctx context.Context, inst *model.Instance) error {
	po, err := toInstancePO(inst)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status", "current_node", "variables", "ftime"}),
		}).
		Create(&po).Error
}

func (s *Storage) GetInstance(ctx context.Context, id string) (*model.Instance, error) {
	var po FlowInstancePO
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, storage.ErrInstanceNotFound
		}
		return nil, err
	}
	return toInstanceDO(po)
}

// GetInstanceForUpdate 采用行级排他锁获取实例 (FOR UPDATE)
func (s *Storage) GetInstanceForUpdate(ctx context.Context, id string) (*model.Instance, error) {
	var po FlowInstancePO
	err := s.buildGetInstanceForUpdateQuery(s.db.WithContext(ctx), id, &po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, storage.ErrInstanceNotFound
		}
		return nil, err
	}
	return toInstanceDO(po)
}

func (s *Storage) buildGetInstanceForUpdateQuery(tx *gorm.DB, id string, out *FlowInstancePO) *gorm.DB {
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(out)
}

func (s *Storage) SaveTasks(ctx context.Context, tasks []*model.Task) error {
	if len(tasks) == 0 {
		return nil
	}
	pos := lo.Map(tasks, func(t *model.Task, _ int) FlowTaskPO {
		return toTaskPO(t)
	})
	return s.db.WithContext(ctx).Create(&pos).Error
}

func (s *Storage) GetTask(ctx context.Context, id string) (*model.Task, error) {
	var po FlowTaskPO
	err := s.db.WithContext(ctx).Where("id = ?", id).First(&po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, storage.ErrTaskNotFound
		}
		return nil, err
	}
	return toTaskDO(po), nil
}

// GetTaskForUpdate 采用行级排他锁获取任务，防止高并发重复审批
func (s *Storage) GetTaskForUpdate(ctx context.Context, id string) (*model.Task, error) {
	var po FlowTaskPO
	err := s.buildGetTaskForUpdateQuery(s.db.WithContext(ctx), id, &po).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, storage.ErrTaskNotFound
		}
		return nil, err
	}
	return toTaskDO(po), nil
}

func (s *Storage) buildGetTaskForUpdateQuery(tx *gorm.DB, id string, out *FlowTaskPO) *gorm.DB {
	return tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(out)
}

func (s *Storage) ListTasksByInstance(ctx context.Context, instID string) ([]*model.Task, error) {
	var pos []FlowTaskPO
	err := s.db.WithContext(ctx).
		Where("instance_id = ?", instID).
		Order("batch_code ASC, ctime ASC").
		Find(&pos).Error
	if err != nil {
		return nil, err
	}
	return lo.Map(pos, func(po FlowTaskPO, _ int) *model.Task {
		return toTaskDO(po)
	}), nil
}

func (s *Storage) buildListPendingTasksQuery(tx *gorm.DB, assignee string) *gorm.DB {
	query := tx.Model(&FlowTaskPO{}).Where("status = ?", uint8(model.TaskPending))
	if assignee != "" {
		query = query.Where("assignee = ?", assignee)
	}
	return query
}

func (s *Storage) ListPendingTasksByAssignee(ctx context.Context, assignee string, offset, limit int) ([]*model.Task, int64, error) {
	var (
		pos   []FlowTaskPO
		total int64
	)
	query := s.buildListPendingTasksQuery(s.db.WithContext(ctx), assignee)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if total == 0 {
		return nil, 0, nil
	}

	err := query.Order("ctime DESC").Offset(offset).Limit(limit).Find(&pos).Error
	if err != nil {
		return nil, 0, err
	}

	tasks := lo.Map(pos, func(po FlowTaskPO, _ int) *model.Task {
		return toTaskDO(po)
	})
	return tasks, total, nil
}

func (s *Storage) UpdateTask(ctx context.Context, task *model.Task) error {
	po := toTaskPO(task)
	res := s.db.WithContext(ctx).Model(&FlowTaskPO{}).
		Where("id = ?", po.ID).
		Updates(map[string]any{
			"status":   po.Status,
			"comment":  po.Comment,
			"ftime":    po.Ftime,
			"assignee": po.Assignee,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return storage.ErrTaskNotFound
	}
	return nil
}

func (s *Storage) UpdateTasksStatus(ctx context.Context, taskIDs []string, status model.TaskStatus) error {
	if len(taskIDs) == 0 {
		return nil
	}
	now := time.Now()
	updates := map[string]any{
		"status": uint8(status),
	}
	// 终态任务同步持久化完成时间，防止重新从 DB 加载时丢失审计完成时间戳 (毫秒时间戳)
	if status != model.TaskPending {
		updates["ftime"] = now.UnixMilli()
	}
	return s.db.WithContext(ctx).Model(&FlowTaskPO{}).
		Where("id IN ?", taskIDs).
		Updates(updates).Error
}
