package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Dispatch struct {
	Id               int64  `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'自增ID'"`
	TenantID         int64  `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	TemplateId       int64  `gorm:"column:template_id;type:bigint;not null;index;comment:'模板ID'"`
	AutomationNodeID string `gorm:"column:automation_node_id;type:varchar(128);not null;index;comment:'自动化节点ID'"`
	RunnerId         int64  `gorm:"column:runner_id;type:bigint;not null;index;comment:'执行器ID'"`
	Field            string `gorm:"column:field;type:varchar(128);not null;comment:'字段'"`
	Value            string `gorm:"column:value;type:varchar(512);not null;comment:'匹配值'"`
	Priority         int    `gorm:"column:priority;type:int;not null;default:100;index;comment:'规则优先级，数值越大越优先'"`
	Ctime            int64  `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime            int64  `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}

// DispatchDAO 执行单元条件路由的物理存储接口。
type DispatchDAO interface {
	// Create 持久化一条路由规则并返回自增主键 ID。
	Create(ctx context.Context, d Dispatch) (int64, error)
	// Update 更新指定模板下的路由规则。
	Update(ctx context.Context, req Dispatch) (int64, error)
	// Delete 根据物理主键 ID 删除指定路由规则。
	Delete(ctx context.Context, id int64) (int64, error)
	// ListByTemplateId 按显式优先级分页获取模板路由规则。
	ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]Dispatch, error)
	// ListAllByTemplateID 获取模板的全部路由规则。
	ListAllByTemplateID(ctx context.Context, templateID int64) ([]Dispatch, error)
	// ListByTemplateNode 获取运行时需要的节点规则，按显式优先级稳定排序。
	ListByTemplateNode(ctx context.Context, templateID int64, nodeID string) ([]Dispatch, error)
	// CountByTemplateId 统计模板路由规则总数。
	CountByTemplateId(ctx context.Context, templateId int64) (int64, error)
	// ExistsCondition 判断同一节点是否已经存在相同匹配条件。
	ExistsCondition(ctx context.Context, excludeID, templateID int64, nodeID, field, value string) (bool, error)
	// ReplaceByTemplate 在同一事务中完整替换模板的条件路由规则。
	ReplaceByTemplate(ctx context.Context, templateID int64, rules []Dispatch) (int64, error)
}

type gormDispatchDAO struct {
	db *gorm.DB
}

// NewDispatchDAO 初始化 GORM 路由规则 DAO。
func NewDispatchDAO(db *gorm.DB) DispatchDAO {
	return &gormDispatchDAO{
		db: db,
	}
}

// Create 插入一条路由规则。
func (g *gormDispatchDAO) Create(ctx context.Context, d Dispatch) (int64, error) {
	now := time.Now().UnixMilli()
	d.Ctime, d.Utime = now, now
	err := g.db.WithContext(ctx).Create(&d).Error
	return d.Id, err
}

// Update 更新路由规则。
func (g *gormDispatchDAO) Update(ctx context.Context, d Dispatch) (int64, error) {
	res := g.db.WithContext(ctx).Model(&Dispatch{}).
		Where("id = ? AND template_id = ?", d.Id, d.TemplateId).
		Updates(map[string]any{
			"runner_id":          d.RunnerId,
			"automation_node_id": d.AutomationNodeID,
			"field":              d.Field,
			"value":              d.Value,
			"priority":           d.Priority,
			"utime":              time.Now().UnixMilli(),
		})
	return res.RowsAffected, res.Error
}

// Delete 依据 ID 删除路由规则。
func (g *gormDispatchDAO) Delete(ctx context.Context, id int64) (int64, error) {
	res := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Dispatch{})
	return res.RowsAffected, res.Error
}

// ListByTemplateId 分页获取指定模板的路由规则。
func (g *gormDispatchDAO) ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]Dispatch, error) {
	var res []Dispatch
	err := g.db.WithContext(ctx).
		Where("template_id = ?", templateId).
		Order("priority DESC, id ASC").
		Offset(int(offset)).
		Limit(int(limit)).
		Find(&res).Error
	return res, err
}

func (g *gormDispatchDAO) ListAllByTemplateID(ctx context.Context, templateID int64) ([]Dispatch, error) {
	var result []Dispatch
	err := g.db.WithContext(ctx).
		Where("template_id = ?", templateID).
		Order("priority DESC, id ASC").
		Find(&result).Error
	return result, err
}

func (g *gormDispatchDAO) ListByTemplateNode(ctx context.Context, templateID int64,
	nodeID string) ([]Dispatch, error) {
	var result []Dispatch
	err := g.db.WithContext(ctx).
		Where("template_id = ? AND automation_node_id = ?", templateID, nodeID).
		Order("priority DESC, id ASC").
		Find(&result).Error
	return result, err
}

// CountByTemplateId 统计指定模板的路由规则数。
func (g *gormDispatchDAO) CountByTemplateId(ctx context.Context, templateId int64) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).
		Model(&Dispatch{}).
		Where("template_id = ?", templateId).
		Count(&count).Error
	return count, err
}

func (g *gormDispatchDAO) ExistsCondition(ctx context.Context, excludeID, templateID int64,
	nodeID, field, value string) (bool, error) {
	query := g.db.WithContext(ctx).Model(&Dispatch{}).
		Where("template_id = ? AND automation_node_id = ? AND field = ? AND value = ?",
			templateID, nodeID, field, value)
	if excludeID > 0 {
		query = query.Where("id <> ?", excludeID)
	}
	var count int64
	err := query.Count(&count).Error
	return count > 0, err
}

func (g *gormDispatchDAO) ReplaceByTemplate(ctx context.Context, templateID int64,
	rules []Dispatch) (int64, error) {
	now := time.Now().UnixMilli()
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("template_id = ?", templateID).Delete(&Dispatch{}).Error; err != nil {
			return err
		}
		for i := range rules {
			rules[i].Id = 0
			rules[i].TemplateId = templateID
			rules[i].Ctime, rules[i].Utime = now, now
		}
		if len(rules) == 0 {
			return nil
		}
		return tx.Create(&rules).Error
	})
	return int64(len(rules)), err
}
