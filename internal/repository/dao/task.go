package dao

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

// TaskDAO 自动化任务持久化层数据访问接口
type TaskDAO interface {
	// CreateTask 向物理数据表中插入一条新自动化作业记录
	CreateTask(ctx context.Context, req Task) (Task, error)
	// FindByProcessInstID 基于实例 ID 与节点 ID 查询作业表记录
	FindByProcessInstID(ctx context.Context, processInstID int, nodeID string) (Task, error)
	// FindByID 精确抓取指定主键 ID 的作业记录
	FindByID(ctx context.Context, id int64) (Task, error)
	// UpdateTask 覆写物理实体中的所有主要调度字段
	UpdateTask(ctx context.Context, req Task) (int64, error)
	// UpdateTaskStatus 更新任务在物理实体中的执行状态、结果和日志
	UpdateTaskStatus(ctx context.Context, req Task) (int64, error)
	// UpdateVariables 覆写任务对应的环境变量 JSON 字段数据
	UpdateVariables(ctx context.Context, id int64, variables []Variables) (int64, error)
	// UpdateArgs 覆写任务对应的透传参数 JSON 字段数据
	UpdateArgs(ctx context.Context, id int64, args domain.TaskArgs) (int64, error)
	// ListTask 分页抓取全量作业任务表记录
	ListTask(ctx context.Context, offset, limit int64) ([]Task, error)
	// ListTaskByStatus 根据当前作业状态分页列表查询
	ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]Task, error)
	// ListTaskByStatusAndKind 根据执行协议类型与状态分页筛选记录
	ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]Task, error)
	// Count 依据状态统计数据库中符合条件的记录行数
	Count(ctx context.Context, status uint8) (int64, error)
	// CountByStatusAndKind 依据类型与状态统计数据库中符合条件的记录行数
	CountByStatusAndKind(ctx context.Context, status uint8, kind string) (int64, error)
	// ListSuccessTasksByUtime 拉取处于特定更新时间之后且状态成功的所有任务，用于自愈机制过滤
	ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]Task, error)
	// TotalByUtime 统计在特定时间戳之后状态成功的任务行数
	TotalByUtime(ctx context.Context, utime int64) (int64, error)
	// FindTaskByNodeID 查询指定流程实例与节点的最终物理结果实体
	FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (Task, error)
	// ListReadyTasks 抓取所有符合计划执行时间且状态待触发的就绪任务列表
	ListReadyTasks(ctx context.Context, limit int64) ([]Task, error)
	// ListTaskByInstanceID 分页拉取特定工作流实例名下的全部物理子任务
	ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]Task, error)
	// TotalByInstanceID 统计关联指定工作流实例名下的物理子任务行数
	TotalByInstanceID(ctx context.Context, instanceID int) (int64, error)
	// MarkTaskAsAutoPassed 将已推进成功的作业记录进行物理状态更新，以防重复流转
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	// UpdateExternalID 持久化更新与外部调度引擎关联的作业映射实例主键
	UpdateExternalID(ctx context.Context, id int64, externalID string) error
}

type gormTaskDAO struct {
	db *gorm.DB
}

func NewTaskDAO(db *gorm.DB) TaskDAO {
	return &gormTaskDAO{db: db}
}

// Task 自动化作业执行与调度任务物理表实体
type Task struct {
	Id              int64                              `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'任务自增主键'"`
	TenantID        string                             `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
	TicketID        int64                              `gorm:"column:ticket_id;type:bigint;not null;index;comment:'关联工单单据ID'"`
	ProcessInstId   int                                `gorm:"column:process_inst_id;type:int;index;comment:'关联流程实例ID'"`
	CurrentNodeId   string                             `gorm:"column:current_node_id;type:varchar(128);index;comment:'当前自动化节点ID'"`
	TriggerPosition string                             `gorm:"column:trigger_position;type:varchar(255);comment:'最近状态触发位置'"`
	WorkflowId      int64                              `gorm:"column:workflow_id;type:bigint;index;comment:'关联工作流定义ID'"`
	CodebookUid     string                             `gorm:"column:codebook_uid;type:varchar(64);index;comment:'关联脚本库模板UID'"`
	Code            string                             `gorm:"column:code;type:text;comment:'运行脚本源码快照'"`
	Language        string                             `gorm:"column:language;type:varchar(32);comment:'脚本语言(python/shell等)'"`
	Args            sqlx.JsonField[domain.TaskArgs]    `gorm:"column:args;type:json;comment:'流程变量透传临时参数json'"`
	Variables       sqlx.JsonField[[]domain.Variables] `gorm:"column:variables;type:json;comment:'输入的环境变量快照json'"`
	Status          uint8                              `gorm:"column:status;type:tinyint unsigned;index;comment:'状态 1:SUCCESS 2:FAILED 3:RUNNING 4:WAITING 5:BLOCKED 6:SCHEDULED'"`
	Result          string                             `gorm:"column:result;type:text;comment:'执行输出日志/返回结果'"`
	WantResult      string                             `gorm:"column:want_result;type:text;comment:'预期执行结果'"`
	ExternalId      string                             `gorm:"column:external_id;type:varchar(128);index;comment:'外部分布式系统任务实例ID'"`
	StartTime       int64                              `gorm:"column:start_time;type:bigint;comment:'任务实际开始时间(毫秒戳)'"`
	EndTime         int64                              `gorm:"column:end_time;type:bigint;comment:'任务实际结束时间(毫秒戳)'"`
	RetryCount      int                                `gorm:"column:retry_count;type:int;comment:'重试自增计数'"`
	IsTiming        bool                               `gorm:"column:is_timing;type:boolean;index;comment:'是否为定时任务'"`
	ScheduledTime   int64                              `gorm:"column:scheduled_time;type:bigint;index;comment:'计划执行时间(毫秒戳)'"`
	Kind            string                             `gorm:"column:kind;type:varchar(32);index;comment:'派发管道协议(KAFKA/GRPC)'"`
	Target          string                             `gorm:"column:target;type:varchar(128);comment:'派发物理目标'"`
	Handler         string                             `gorm:"column:handler;type:varchar(128);comment:'执行器业务方法'"`
	AutoPassed      bool                               `gorm:"column:auto_passed;type:boolean;index;comment:'成功任务是否已自动推进流程'"`
	Ctime           int64                              `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime           int64                              `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
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

func (g *gormTaskDAO) FindByProcessInstID(ctx context.Context, processInstID int, nodeID string) (Task, error) {
	var res Task
	err := g.db.WithContext(ctx).
		Where("process_inst_id = ? AND current_node_id = ?", processInstID, nodeID).
		First(&res).Error
	return res, err
}

func (g *gormTaskDAO) FindByID(ctx context.Context, id int64) (Task, error) {
	var res Task
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}

func (g *gormTaskDAO) UpdateTask(ctx context.Context, req Task) (int64, error) {
	res := g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", req.Id).Updates(map[string]any{
		"ticket_id":        req.TicketID,
		"process_inst_id":  req.ProcessInstId,
		"current_node_id":  req.CurrentNodeId,
		"trigger_position": req.TriggerPosition,
		"workflow_id":      req.WorkflowId,
		"codebook_uid":     req.CodebookUid,
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

func (g *gormTaskDAO) FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (Task, error) {
	return g.FindByProcessInstID(ctx, instanceID, nodeID)
}

func (g *gormTaskDAO) ListReadyTasks(ctx context.Context, limit int64) ([]Task, error) {
	var res []Task
	now := time.Now().UnixMilli()
	err := g.db.WithContext(ctx).
		Where("status = ? AND (is_timing = ? OR scheduled_time <= ?)", domain.WAITING.ToUint8(), false, now).
		Order("scheduled_time asc, ctime asc").Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]Task, error) {
	var res []Task
	err := g.db.WithContext(ctx).Where("process_inst_id = ?", instanceID).
		Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormTaskDAO) TotalByInstanceID(ctx context.Context, instanceID int) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Task{}).Where("process_inst_id = ?", instanceID).Count(&count).Error
	return count, err
}

func (g *gormTaskDAO) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"auto_passed": true,
		"utime":       time.Now().UnixMilli(),
	}).Error
}

func (g *gormTaskDAO) UpdateExternalID(ctx context.Context, id int64, externalID string) error {
	return g.db.WithContext(ctx).Model(&Task{}).Where("id = ?", id).Updates(map[string]any{
		"external_id": externalID,
		"utime":       time.Now().UnixMilli(),
	}).Error
}
