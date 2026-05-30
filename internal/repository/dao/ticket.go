package dao

import (
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// Ticket 工单记录实体
type Ticket struct {
	Id                int64                             `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'工单自增ID'"`
	TenantID          string                            `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
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

// TaskForm 审批流步骤中的动态任务快照表单实体
type TaskForm struct {
	Id       int64                       `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'任务快照自增ID'"`
	TenantID string                      `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
	OrderId  int64                       `gorm:"column:order_id;type:bigint;not null;index;comment:'关联的工单单据ID'"`
	TaskId   int                         `gorm:"column:task_id;type:int;not null;index;comment:'关联工作流步骤节点任务ID'"`
	Name     string                      `gorm:"column:name;type:varchar(128);not null;comment:'快照步骤展现名称'"`
	Key      string                      `gorm:"column:key;type:varchar(128);not null;index;comment:'快照表单控件唯一Key'"`
	Type     string                      `gorm:"column:type;type:varchar(64);comment:'表单字段组件渲染类型'"`
	Value    sqlx.JsonField[interface{}] `gorm:"column:value;type:json;comment:'表单存储的具体数值json'"`
	Ctime    int64                       `gorm:"column:ctime;type:bigint;comment:'创建快照时间(毫秒)'"`
}
