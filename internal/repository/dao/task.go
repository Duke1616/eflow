package dao

import (
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
)

// Task 自动化作业执行与调度任务物理表实体
type Task struct {
	Id            int64                           `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'任务自增主键'"`
	TenantID      string                          `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
	OrderId       int64                           `gorm:"column:order_id;type:bigint;not null;index;comment:'关联工单单据ID'"`
	ProcessInstId int                             `gorm:"column:process_inst_id;type:int;index;comment:'关联流程实例ID'"`
	CodebookUid   string                          `gorm:"column:codebook_uid;type:varchar(64);index;comment:'关联脚本库模板UID'"`
	Code          string                          `gorm:"column:code;type:text;comment:'运行脚本源码快照'"`
	Language      string                          `gorm:"column:language;type:varchar(32);comment:'脚本语言(python/shell等)'"`
	Args          sqlx.JsonField[domain.TaskArgs] `gorm:"column:args;type:json;comment:'流程变量透传临时参数json'"`
	Variables     sqlx.JsonField[[]Variables]     `gorm:"column:variables;type:json;comment:'输入的环境变量快照json'"`
	Status        uint8                           `gorm:"column:status;type:tinyint unsigned;index;comment:'状态 1:SUCCESS 2:FAILED 3:RUNNING 4:WAITING 5:BLOCKED 6:SCHEDULED'"`
	Result        string                          `gorm:"column:result;type:text;comment:'执行输出日志/返回结果'"`
	WantResult    string                          `gorm:"column:want_result;type:text;comment:'预期执行结果'"`
	ExternalId    string                          `gorm:"column:external_id;type:varchar(128);index;comment:'外部分布式系统任务实例ID'"`
	StartTime     int64                           `gorm:"column:start_time;type:bigint;comment:'任务实际开始时间(毫秒戳)'"`
	EndTime       int64                           `gorm:"column:end_time;type:bigint;comment:'任务实际结束时间(毫秒戳)'"`
	RetryCount    int                             `gorm:"column:retry_count;type:int;comment:'重试自增计数'"`
	Ctime         int64                           `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime         int64                           `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}
