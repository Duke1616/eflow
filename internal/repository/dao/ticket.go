package dao

import (
	"context"
	"errors"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

// Ticket 工单记录实体
type Ticket struct {
	Id                int64                             `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'工单自增ID'"`
	TenantID          int64                             `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	BizID             int64                             `gorm:"column:biz_id;type:bigint;index;comment:'关联业务场景ID'"`
	Key               string                            `gorm:"column:key;type:varchar(64);index;comment:'工单业务唯一单据号Key'"`
	TemplateId        int64                             `gorm:"column:template_id;type:bigint;not null;index;comment:'绑定工单模板ID'"`
	WorkflowId        int64                             `gorm:"column:workflow_id;type:bigint;not null;index;comment:'绑定工作流流程ID'"`
	ProcessInstanceId int                               `gorm:"column:process_instance_id;type:int;index;comment:'关联流程实例执行ID'"`
	CreateBy          string                            `gorm:"column:create_by;type:varchar(128);index;comment:'工单创建人用户名'"`
	Provide           uint8                             `gorm:"column:provide;type:tinyint unsigned;comment:'业务提供属性'"`
	Data              sqlx.JsonField[domain.TicketData] `gorm:"column:data;type:json;comment:'工单用户填写的动态数据json'"`
	Status            uint8                             `gorm:"column:status;type:tinyint unsigned;index;comment:'工单当前状态'"`
	NotificationConf  sqlx.JsonField[NotificationConf]  `gorm:"column:notification_conf;type:json;comment:'工单通知偏好参数配置json'"`
	Ctime             int64                             `gorm:"column:ctime;type:bigint;comment:'创建发起时间(毫秒)'"`
	Wtime             int64                             `gorm:"column:wtime;type:bigint;comment:'办结归档时间(毫秒)'"`
	Utime             int64                             `gorm:"column:utime;type:bigint;comment:'最近修改时间(毫秒)'"`
}

// NotificationConf 工单相关的通知模板与参数配置
type NotificationConf struct {
	TemplateID     int64                     `json:"template_id"`
	TemplateParams domain.NotificationParams `json:"template_params"`
	Channel        string                    `json:"channel"`
}


// TicketDAO 工单物理数据访问接口
type TicketDAO interface {
	// CreateBizTicket 在 DB 中物理持久化一条带有外部场景属性和唯一键值的工单记录
	CreateBizTicket(ctx context.Context, ticket Ticket) (Ticket, error)
	// CreateTicket 在 DB 中物理持久化一条基础工单并返回自增主键 ID
	CreateTicket(ctx context.Context, req Ticket) (int64, error)
	// DetailByProcessInstId 依据绑定的工作流引擎流程实例 ID 从物理表获取单据
	DetailByProcessInstId(ctx context.Context, instanceId int) (Ticket, error)
	// Detail 依据物理主键 ID 获取单条工单详情
	Detail(ctx context.Context, id int64) (Ticket, error)
	// RegisterProcessInstanceId 为指定工单登记绑定的引擎实例 ID 并且同步置为流转状态
	RegisterProcessInstanceId(ctx context.Context, id int64, instanceId int, status uint8) error
	// ListTicketByProcessInstanceIds 根据引擎实例 ID 列表高效批量反查工单物理记录
	ListTicketByProcessInstanceIds(ctx context.Context, instanceIds []int) ([]Ticket, error)
	// UpdateStatusByInstanceId 根据工作流实例 ID 物理更新工单单据流转状态
	UpdateStatusByInstanceId(ctx context.Context, instanceId int, status uint8) error
	// ListTicket 分页条件过滤获取与指定用户相关且在状态集合内的物理记录列表
	ListTicket(ctx context.Context, userId string, status []int, offset, limit int64) ([]Ticket, error)
	// CountTicket 统计与指定用户相关且在指定状态集合内的工单物理记录总条数
	CountTicket(ctx context.Context, userId string, status []int) (int64, error)
	// FindByBizIdAndKey 依据场景 ID、单据号和允许的状态列表查找满足条件的唯一活跃工单记录
	FindByBizIdAndKey(ctx context.Context, bizId int64, key string, status []uint8) (Ticket, error)
	// MergeTicketData 高效利用原子更新将新表单属性合并至已持久化的 JSON 列中
	MergeTicketData(ctx context.Context, id int64, data map[string]interface{}) error
}

type gormTicketDAO struct {
	db *gorm.DB
}

func NewTicketDAO(db *gorm.DB) TicketDAO {
	return &gormTicketDAO{
		db: db,
	}
}

func (g *gormTicketDAO) CreateBizTicket(ctx context.Context, ticket Ticket) (Ticket, error) {
	now := time.Now().UnixMilli()
	ticket.Ctime, ticket.Utime = now, now
	err := g.db.WithContext(ctx).Create(&ticket).Error
	return ticket, err
}

func (g *gormTicketDAO) CreateTicket(ctx context.Context, req Ticket) (int64, error) {
	now := time.Now().UnixMilli()
	req.Ctime, req.Utime = now, now
	err := g.db.WithContext(ctx).Create(&req).Error
	return req.Id, err
}

func (g *gormTicketDAO) DetailByProcessInstId(ctx context.Context, instanceId int) (Ticket, error) {
	var res Ticket
	err := g.db.WithContext(ctx).Where("process_instance_id = ?", instanceId).First(&res).Error
	return res, err
}

func (g *gormTicketDAO) Detail(ctx context.Context, id int64) (Ticket, error) {
	var res Ticket
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}

func (g *gormTicketDAO) RegisterProcessInstanceId(ctx context.Context, id int64, instanceId int, status uint8) error {
	return g.db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", id).Updates(map[string]interface{}{
		"process_instance_id": instanceId,
		"status":              status,
		"utime":               time.Now().UnixMilli(),
	}).Error
}

func (g *gormTicketDAO) ListTicketByProcessInstanceIds(ctx context.Context, instanceIds []int) ([]Ticket, error) {
	var res []Ticket
	if len(instanceIds) == 0 {
		return res, nil
	}
	err := g.db.WithContext(ctx).Where("process_instance_id IN ?", instanceIds).Find(&res).Error
	return res, err
}

func (g *gormTicketDAO) UpdateStatusByInstanceId(ctx context.Context, instanceId int, status uint8) error {
	now := time.Now().UnixMilli()
	return g.db.WithContext(ctx).Model(&Ticket{}).Where("process_instance_id = ?", instanceId).Updates(map[string]interface{}{
		"status": status,
		"utime":  now,
		"wtime":  now,
	}).Error
}

func (g *gormTicketDAO) ListTicket(ctx context.Context, userId string, status []int, offset, limit int64) ([]Ticket, error) {
	var res []Ticket
	query := g.db.WithContext(ctx)
	if userId != "" {
		query = query.Where("create_by = ?", userId)
	}
	if len(status) > 0 {
		query = query.Where("status IN ?", status)
	}
	err := query.Order("ctime desc").Limit(int(limit)).Offset(int(offset)).Find(&res).Error
	return res, err
}

func (g *gormTicketDAO) CountTicket(ctx context.Context, userId string, status []int) (int64, error) {
	var total int64
	query := g.db.WithContext(ctx).Model(&Ticket{})
	if userId != "" {
		query = query.Where("create_by = ?", userId)
	}
	if len(status) > 0 {
		query = query.Where("status IN ?", status)
	}
	err := query.Count(&total).Error
	return total, err
}

func (g *gormTicketDAO) FindByBizIdAndKey(ctx context.Context, bizId int64, key string, status []uint8) (Ticket, error) {
	var res Ticket
	query := g.db.WithContext(ctx).Where("biz_id = ? AND `key` = ?", bizId, key)
	if len(status) > 0 {
		query = query.Where("status IN ?", status)
	}
	err := query.Order("ctime desc").First(&res).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Ticket{}, nil
	}
	return res, err
}

func (g *gormTicketDAO) MergeTicketData(ctx context.Context, id int64, data map[string]interface{}) error {
	var t Ticket
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	if err != nil {
		return err
	}
	if t.Data.Val == nil {
		t.Data.Val = make(map[string]interface{})
	}
	for k, v := range data {
		t.Data.Val[k] = v
	}
	t.Utime = time.Now().UnixMilli()
	return g.db.WithContext(ctx).Model(&Ticket{}).Where("id = ?", id).Updates(map[string]interface{}{
		"data":  t.Data,
		"utime": t.Utime,
	}).Error
}

