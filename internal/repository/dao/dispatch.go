package dao

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type Dispatch struct {
	Id         int64  `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'自增ID'"`
	TenantID   int64  `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	TemplateId int64  `gorm:"column:template_id;type:bigint;not null;index;comment:'模板ID'"`
	RunnerId   int64  `gorm:"column:runner_id;type:bigint;not null;index;comment:'执行器ID'"`
	Field      string `gorm:"column:field;type:varchar(128);not null;comment:'字段'"`
	Value      string `gorm:"column:value;type:varchar(512);not null;comment:'值'"`
	Ctime      int64  `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime      int64  `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}

// DispatchDAO 自动派发数据物理存储接口
type DispatchDAO interface {
	// Create 在物理表中持久化一条新的自动派发规则，并返回自增主键 ID
	Create(ctx context.Context, d Dispatch) (int64, error)
	// Update 局部更新指定的派发规则，主要修改执行器 ID (RunnerId)、匹配字段名 (Field) 和值 (Value)
	Update(ctx context.Context, req Dispatch) (int64, error)
	// Delete 根据物理主键 ID 删除指定的自动派发规则
	Delete(ctx context.Context, id int64) (int64, error)
	// ListByTemplateId 依据模板 ID 分页获取其关联的所有自动派发规则，按创建时间降序排序
	ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]Dispatch, error)
	// CountByTemplateId 统计指定模板 ID 下关联的自动派发规则总条数
	CountByTemplateId(ctx context.Context, templateId int64) (int64, error)
	// Sync 批量同步指定模板的派发规则列表。使用数据库事务保证幂等写入：
	// 遇到在 template_id/runner_id/field/value 下完全一致的规则时仅更新修改时间，不存在时则新建记录
	Sync(ctx context.Context, templateId int64, docs []Dispatch) (int64, error)
}

type gormDispatchDAO struct {
	db *gorm.DB
}

// NewDispatchDAO 初始化 GORM 版的自动派发 DAO
func NewDispatchDAO(db *gorm.DB) DispatchDAO {
	return &gormDispatchDAO{
		db: db,
	}
}

// Create 物理插入一条自动派发规则
func (g *gormDispatchDAO) Create(ctx context.Context, d Dispatch) (int64, error) {
	now := time.Now().UnixMilli()
	d.Ctime, d.Utime = now, now
	err := g.db.WithContext(ctx).Create(&d).Error
	return d.Id, err
}

// Update 更新派发规则中的执行器关联及条件参数
func (g *gormDispatchDAO) Update(ctx context.Context, d Dispatch) (int64, error) {
	res := g.db.WithContext(ctx).Model(&Dispatch{}).Where("id = ?", d.Id).Updates(map[string]any{
		"runner_id": d.RunnerId,
		"field":     d.Field,
		"value":     d.Value,
		"utime":     time.Now().UnixMilli(),
	})
	return res.RowsAffected, res.Error
}

// Delete 依据 ID 物理删除派发规则
func (g *gormDispatchDAO) Delete(ctx context.Context, id int64) (int64, error) {
	res := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Dispatch{})
	return res.RowsAffected, res.Error
}

// ListByTemplateId 分页拉取指定工单模板下的派发规则列表
func (g *gormDispatchDAO) ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]Dispatch, error) {
	var res []Dispatch
	err := g.db.WithContext(ctx).
		Where("template_id = ?", templateId).
		Order("ctime desc").
		Offset(int(offset)).
		Limit(int(limit)).
		Find(&res).Error
	return res, err
}

// CountByTemplateId 统计指定模板下的派发规则条数
func (g *gormDispatchDAO) CountByTemplateId(ctx context.Context, templateId int64) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).
		Model(&Dispatch{}).
		Where("template_id = ?", templateId).
		Count(&count).Error
	return count, err
}

// Sync 通过事务机制批量差集同步最新自动派发规则，防止数据重复
func (g *gormDispatchDAO) Sync(ctx context.Context, templateId int64, docs []Dispatch) (int64, error) {
	now := time.Now().UnixMilli()
	var inserted int64 = 0

	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, doc := range docs {
			var existing Dispatch
			err := tx.Where("template_id = ? AND runner_id = ? AND field = ? AND value = ?",
				templateId, doc.RunnerId, doc.Field, doc.Value).
				First(&existing).Error

			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					// 不存在，插入
					doc.TemplateId = templateId
					doc.Ctime, doc.Utime = now, now
					if err := tx.Create(&doc).Error; err != nil {
						return err
					}
					inserted++
				} else {
					return err
				}
			} else {
				// 存在，只更新 utime
				if err := tx.Model(&Dispatch{}).Where("id = ?", existing.Id).Update("utime", now).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})

	return inserted, err
}
