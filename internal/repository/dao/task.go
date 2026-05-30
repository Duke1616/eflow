package dao

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

type TaskDAO interface {
	CreateTask(ctx context.Context, req Task) (Task, error)
	FindByProcessInstId(ctx context.Context, processInstId int, nodeId string) (Task, error)
	FindById(ctx context.Context, id int64) (Task, error)
	UpdateTask(ctx context.Context, req Task) (int64, error)
	UpdateTaskStatus(ctx context.Context, req Task) (int64, error)
	UpdateVariables(ctx context.Context, id int64, variables []Variables) (int64, error)
	UpdateArgs(ctx context.Context, id int64, args domain.TaskArgs) (int64, error)
	ListTask(ctx context.Context, offset, limit int64) ([]Task, error)
	ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]Task, error)
	ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]Task, error)
	Count(ctx context.Context, status uint8) (int64, error)
	CountByStatusAndKind(ctx context.Context, status uint8, kind string) (int64, error)
	ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]Task, error)
	TotalByUtime(ctx context.Context, utime int64) (int64, error)
	FindTaskResult(ctx context.Context, instanceId int, nodeId string) (Task, error)
	ListReadyTasks(ctx context.Context, limit int64) ([]Task, error)
	ListTaskByInstanceId(ctx context.Context, offset, limit int64, instanceId int) ([]Task, error)
	TotalByInstanceId(ctx context.Context, instanceId int) (int64, error)
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	UpdateExternalId(ctx context.Context, id int64, externalId string) error
}

type gormTaskDAO struct {
	db *gorm.DB
}

func NewTaskDAO(db *gorm.DB) TaskDAO {
	return &gormTaskDAO{db: db}
}

// Task 自动化作业执行与调度任务物理表实体
type Task struct {
	Id              int64                           `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'任务自增主键'"`
	TenantID        string                          `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
	OrderId         int64                           `gorm:"column:order_id;type:bigint;not null;index;comment:'关联工单单据ID'"`
	ProcessInstId   int                             `gorm:"column:process_inst_id;type:int;index;comment:'关联流程实例ID'"`
	CurrentNodeId   string                          `gorm:"column:current_node_id;type:varchar(128);index;comment:'当前自动化节点ID'"`
	TriggerPosition string                          `gorm:"column:trigger_position;type:varchar(255);comment:'最近状态触发位置'"`
	WorkflowId      int64                           `gorm:"column:workflow_id;type:bigint;index;comment:'关联工作流定义ID'"`
	CodebookUid     string                          `gorm:"column:codebook_uid;type:varchar(64);index;comment:'关联脚本库模板UID'"`
	CodebookName    string                          `gorm:"column:codebook_name;type:varchar(128);comment:'脚本库名称快照'"`
	Code            string                          `gorm:"column:code;type:text;comment:'运行脚本源码快照'"`
	Language        string                          `gorm:"column:language;type:varchar(32);comment:'脚本语言(python/shell等)'"`
	Args            sqlx.JsonField[domain.TaskArgs] `gorm:"column:args;type:json;comment:'流程变量透传临时参数json'"`
	Variables       sqlx.JsonField[[]Variables]     `gorm:"column:variables;type:json;comment:'输入的环境变量快照json'"`
	Status          uint8                           `gorm:"column:status;type:tinyint unsigned;index;comment:'状态 1:SUCCESS 2:FAILED 3:RUNNING 4:WAITING 5:BLOCKED 6:SCHEDULED'"`
	Result          string                          `gorm:"column:result;type:text;comment:'执行输出日志/返回结果'"`
	WantResult      string                          `gorm:"column:want_result;type:text;comment:'预期执行结果'"`
	ExternalId      string                          `gorm:"column:external_id;type:varchar(128);index;comment:'外部分布式系统任务实例ID'"`
	StartTime       int64                           `gorm:"column:start_time;type:bigint;comment:'任务实际开始时间(毫秒戳)'"`
	EndTime         int64                           `gorm:"column:end_time;type:bigint;comment:'任务实际结束时间(毫秒戳)'"`
	RetryCount      int                             `gorm:"column:retry_count;type:int;comment:'重试自增计数'"`
	IsTiming        bool                            `gorm:"column:is_timing;type:boolean;index;comment:'是否为定时任务'"`
	ScheduledTime   int64                           `gorm:"column:scheduled_time;type:bigint;index;comment:'计划执行时间(毫秒戳)'"`
	Kind            string                          `gorm:"column:kind;type:varchar(32);index;comment:'派发管道协议(KAFKA/GRPC)'"`
	Target          string                          `gorm:"column:target;type:varchar(128);comment:'派发物理目标'"`
	Handler         string                          `gorm:"column:handler;type:varchar(128);comment:'执行器业务方法'"`
	AutoPassed      bool                            `gorm:"column:auto_passed;type:boolean;index;comment:'成功任务是否已自动推进流程'"`
	Ctime           int64                           `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime           int64                           `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}

func (g *gormTaskDAO) CreateTask(ctx context.Context, req Task) (Task, error) {
	now := time.Now().UnixMilli()
	req.Ctime, req.Utime = now, now
	if req.Status == 0 {
		req.Status = domain.WAITING.ToUint8()
	}
	err := g.db.WithContext(ctx).Create(&req).Error
	return req, err
}

func (g *gormTaskDAO) FindByProcessInstId(ctx context.Context, processInstId int, nodeId string) (Task, error) {
	var res Task
	err := g.db.WithContext(ctx).
		Where("process_inst_id = ? AND current_node_id = ?", processInstId, nodeId).
		First(&res).Error
	return res, err
}

func (g *gormTaskDAO) FindById(ctx context.Context, id int64) (Task, error) {
	var res Task
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}

func (g *gormTaskDAO) UpdateTask(ctx context.Context, req Task) (int64, error) {
	res := g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", req.Id).Updates(map[string]any{
		"order_id":         req.OrderId,
		"process_inst_id":  req.ProcessInstId,
		"current_node_id":  req.CurrentNodeId,
		"trigger_position": req.TriggerPosition,
		"workflow_id":      req.WorkflowId,
		"codebook_uid":     req.CodebookUid,
		"codebook_name":    req.CodebookName,
		"code":             req.Code,
		"language":         req.Language,
		"args":             req.Args,
		"variables":        req.Variables,
		"status":           req.Status,
		"want_result":      req.WantResult,
		"is_timing":        req.IsTiming,
		"scheduled_time":   req.ScheduledTime,
		"kind":             req.Kind,
		"target":           req.Target,
		"handler":          req.Handler,
		"external_id":      req.ExternalId,
		"utime":            time.Now().UnixMilli(),
	})
	return res.RowsAffected, res.Error
}

func (g *gormTaskDAO) UpdateTaskStatus(ctx context.Context, req Task) (int64, error) {
	updates := map[string]any{
		"trigger_position": req.TriggerPosition,
		"result":           req.Result,
		"want_result":      req.WantResult,
		"utime":            time.Now().UnixMilli(),
	}
	if req.Status > 0 {
		updates["status"] = req.Status
	}
	if req.StartTime > 0 {
		updates["start_time"] = req.StartTime
	}
	if req.EndTime > 0 {
		updates["end_time"] = req.EndTime
	}
	if req.RetryCount == -1 {
		updates["retry_count"] = 0
	} else if req.RetryCount > 0 {
		updates["retry_count"] = gorm.Expr("retry_count + ?", req.RetryCount)
	}
	res := g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", req.Id).Updates(updates)
	return res.RowsAffected, res.Error
}

func (g *gormTaskDAO) UpdateVariables(ctx context.Context, id int64, variables []Variables) (int64, error) {
	res := g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"variables": sqlx.JsonField[[]Variables]{Val: variables, Valid: true},
		"utime":     time.Now().UnixMilli(),
	})
	return res.RowsAffected, res.Error
}

func (g *gormTaskDAO) UpdateArgs(ctx context.Context, id int64, args domain.TaskArgs) (int64, error) {
	res := g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"args":  sqlx.JsonField[domain.TaskArgs]{Val: args, Valid: true},
		"utime": time.Now().UnixMilli(),
	})
	return res.RowsAffected, res.Error
}

func (g *gormTaskDAO) ListTask(ctx context.Context, offset, limit int64) ([]Task, error) {
	var res []Task
	err := g.db.WithContext(ctx).Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]Task, error) {
	var res []Task
	err := g.db.WithContext(ctx).Where("status = ?", status).Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]Task, error) {
	var res []Task
	err := g.db.WithContext(ctx).Where("status = ? AND kind = ?", status, kind).Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) Count(ctx context.Context, status uint8) (int64, error) {
	var count int64
	query := g.db.WithContext(ctx).Model(&Task{})
	if status > 0 {
		query = query.Where("status = ?", status)
	}
	err := query.Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) CountByStatusAndKind(ctx context.Context, status uint8, kind string) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).Where("status = ? AND kind = ?", status, kind).Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]Task, error) {
	var res []Task
	err := g.db.WithContext(ctx).
		Where("status = ? AND utime <= ? AND auto_passed = ?", domain.SUCCESS.ToUint8(), utime, false).
		Order("utime asc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) TotalByUtime(ctx context.Context, utime int64) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).
		Where("status = ? AND utime <= ? AND auto_passed = ?", domain.SUCCESS.ToUint8(), utime, false).
		Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) FindTaskResult(ctx context.Context, instanceId int, nodeId string) (Task, error) {
	return g.FindByProcessInstId(ctx, instanceId, nodeId)
}

func (g *gormTaskDAO) ListReadyTasks(ctx context.Context, limit int64) ([]Task, error) {
	var res []Task
	now := time.Now().UnixMilli()
	err := g.db.WithContext(ctx).
		Where("status = ? AND (is_timing = ? OR scheduled_time <= ?)", domain.WAITING.ToUint8(), false, now).
		Order("scheduled_time asc, ctime asc").Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) ListTaskByInstanceId(ctx context.Context, offset, limit int64, instanceId int) ([]Task, error) {
	var res []Task
	err := g.db.WithContext(ctx).Where("process_inst_id = ?", instanceId).
		Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) TotalByInstanceId(ctx context.Context, instanceId int) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).Where("process_inst_id = ?", instanceId).Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"auto_passed": true,
		"utime":       time.Now().UnixMilli(),
	}).Error
}

func (g *gormTaskDAO) UpdateExternalId(ctx context.Context, id int64, externalId string) error {
	return g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"external_id": externalId,
		"utime":       time.Now().UnixMilli(),
	}).Error
}
