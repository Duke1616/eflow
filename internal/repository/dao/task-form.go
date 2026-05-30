package dao

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

// TaskForm 审批流步骤中的动态任务快照表单实体
type TaskForm struct {
	Id       int64                       `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'任务快照自增ID'"`
	TenantID string                      `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
	TicketId int64                       `gorm:"column:ticket_id;type:bigint;not null;index;comment:'关联的工单单据ID'"`
	TaskId   int                         `gorm:"column:task_id;type:int;not null;index;comment:'关联工作流步骤节点任务ID'"`
	Name     string                      `gorm:"column:name;type:varchar(128);not null;comment:'快照步骤展现名称'"`
	Key      string                      `gorm:"column:key;type:varchar(128);not null;index;comment:'快照表单控件唯一Key'"`
	Type     string                      `gorm:"column:type;type:varchar(64);comment:'表单字段组件渲染类型'"`
	Value    sqlx.JsonField[interface{}] `gorm:"column:value;type:json;comment:'表单存储的具体数值json'"`
	Ctime    int64                       `gorm:"column:ctime;type:bigint;comment:'创建快照时间(毫秒)'"`
}

// TableName 自定义物理数据库表名为 ticket_task_form
func (TaskForm) TableName() string {
	return "ticket_task_form"
}

// TaskFormDAO 任务步骤物理表单快照数据访问接口
type TaskFormDAO interface {
	// Create 在 DB 中物理持久化一组审批节点所提交的表单字段快照数据记录
	Create(ctx context.Context, forms []TaskForm) error
	// FindByTaskIds 根据任务节点 ID 列表高效批量反查已存盘的快照字段记录
	FindByTaskIds(ctx context.Context, taskIds []int) ([]TaskForm, error)
	// FindByTicketID 检索获取指定工单下所有审批节点曾存盘过的快照数据集合
	FindByTicketID(ctx context.Context, ticketID int64) ([]TaskForm, error)
}

type gormTaskFormDAO struct {
	db *gorm.DB
}

// NewTaskFormDAO 构造审批任务表单快照数据访问层
func NewTaskFormDAO(db *gorm.DB) TaskFormDAO {
	return &gormTaskFormDAO{
		db: db,
	}
}

func (g *gormTaskFormDAO) Create(ctx context.Context, forms []TaskForm) error {
	if len(forms) == 0 {
		return nil
	}
	now := time.Now().UnixMilli()
	for i := range forms {
		forms[i].Ctime = now
	}
	return g.db.WithContext(ctx).Create(&forms).Error
}

func (g *gormTaskFormDAO) FindByTaskIds(ctx context.Context, taskIds []int) ([]TaskForm, error) {
	var res []TaskForm
	if len(taskIds) == 0 {
		return res, nil
	}
	err := g.db.WithContext(ctx).Where("task_id IN ?", taskIds).Find(&res).Error
	return res, err
}

func (g *gormTaskFormDAO) FindByTicketID(ctx context.Context, ticketID int64) ([]TaskForm, error) {
	var res []TaskForm
	subQuery := g.db.Model(&TaskForm{}).
		Select("`key`, MAX(ctime) as max_ctime").
		Where("ticket_id = ?", ticketID).
		Group("`key`")

	err := g.db.WithContext(ctx).Model(&TaskForm{}).
		Joins("INNER JOIN (?) as t2 ON task_form.key = t2.key AND task_form.ctime = t2.max_ctime", subQuery).
		Where("task_form.ticket_id = ?", ticketID).
		Find(&res).Error

	return res, err
}
