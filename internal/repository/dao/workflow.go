package dao

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

// Workflow 工作流流程定义实体
type Workflow struct {
	Id           int64                     `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'流程设计定义唯一自增ID'"`
	TenantID     int64                     `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	TemplateId   int64                     `gorm:"column:template_id;type:bigint;not null;index;comment:'绑定挂载的工单模板ID'"`
	Name         string                    `gorm:"column:name;type:varchar(128);not null;comment:'流程设计器展示名称'"`
	Icon         string                    `gorm:"column:icon;type:varchar(256);comment:'流程设计器图标'"`
	Owner        string                    `gorm:"column:owner;type:varchar(128);comment:'流程设计所有者/管理员'"`
	Desc         string                    `gorm:"column:desc;type:text;comment:'流程说明描述'"`
	ProcessId    int                       `gorm:"column:process_id;type:int;comment:'关联工作流引擎部署的流程模型ID'"`
	FlowData     sqlx.JsonField[LogicFlow] `gorm:"column:flow_data;type:json;comment:'LogicFlow图形化流程拓扑节点关系json'"`
	IsNotify     bool                      `gorm:"column:is_notify;type:tinyint(1);default:0;comment:'流程审批流转时是否触发发送通知'"`
	NotifyMethod uint8                     `gorm:"column:notify_method;type:tinyint unsigned;comment:'流程通知首选投递渠道类型'"`
	Ctime        int64                     `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime        int64                     `gorm:"column:utime;type:bigint;comment:'修改时间(毫秒)'"`
}

// TableName 指定物理表名
func (Workflow) TableName() string {
	return "workflow"
}

// LogicFlow 前端流程设计器对应的 JSON 表达结构
type LogicFlow struct {
	Edges []domain.FlowEdge `json:"edges"`
	Nodes []domain.FlowNode `json:"nodes"`
}

// Snapshot 流程定义发布版本画布快照物理实体
type Snapshot struct {
	Id             int64                     `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'快照唯一自增ID'"`
	TenantID       int64                     `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	WorkflowId     int64                     `gorm:"column:workflow_id;type:bigint;not null;uniqueIndex:uix_wf_process_version;comment:'绑定的工作流定义ID'"`
	ProcessId      int                       `gorm:"column:process_id;type:int;not null;uniqueIndex:uix_wf_process_version;comment:'关联引擎部署的流程模型ID'"`
	ProcessVersion int                       `gorm:"column:process_version;type:int;not null;uniqueIndex:uix_wf_process_version;comment:'对应流程发布版本号'"`
	Name           string                    `gorm:"column:name;type:varchar(128);not null;comment:'快照名字'"`
	FlowData       sqlx.JsonField[LogicFlow] `gorm:"column:flow_data;type:json;comment:'发布快照时对应的画布拓扑结构数据json'"`
	Ctime          int64                     `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
}

// TableName 指定物理表名
func (Snapshot) TableName() string {
	return "workflow_snapshot"
}

// IWorkflowCoreDAO 工作流核心物理数据访问接口
type IWorkflowCoreDAO interface {
	// CreateWorkflow 创建流程定义，返回生成的自增 ID
	CreateWorkflow(ctx context.Context, w Workflow) (int64, error)
	// List 分页查询流程定义列表，按时间逆序
	List(ctx context.Context, offset, limit int64) ([]Workflow, error)
	// Count 统计当前租户空间下有效的流程定义总数
	Count(ctx context.Context) (int64, error)
	// UpdateWorkflow 更新流程定义配置属性，返回受影响的行数
	UpdateWorkflow(ctx context.Context, c Workflow) (int64, error)
	// UpdateProcessId 绑定此流程对应底层引擎生成的流程定义 ID
	UpdateProcessId(ctx context.Context, id int64, processId int) error
	// DeleteWorkflow 根据主键 ID 删除流程定义，返回受影响的行数
	DeleteWorkflow(ctx context.Context, id int64) (int64, error)
	// FindWorkflow 根据主键 ID 精确检索单个工作流模板详情
	FindWorkflow(ctx context.Context, id int64) (Workflow, error)
	// FindByIds 根据主键 ID 列表批量检索流程定义，省略大 JSON 字段
	FindByIds(ctx context.Context, ids []int64) ([]Workflow, error)
	// FindByKeyword 按照关键字模糊匹配流程名称及描述的分页检索
	FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]Workflow, error)
	// CountByKeyword 计算含有对应关键字特征的流程总条数
	CountByKeyword(ctx context.Context, keyword string) (int64, error)
}

// ISnapshotDAO 流程版本发布画布快照物理数据访问接口
type ISnapshotDAO interface {
	// CreateSnapshot 持久化流程发布画布快照
	CreateSnapshot(ctx context.Context, snapshot Snapshot) error
	// FindSnapshotByProcess 根据底层关联的流程引擎 ID 与发布的对应版本号精确检索快照数据
	FindSnapshotByProcess(ctx context.Context, processID, version int) (Snapshot, error)
}

// IWorkflowDAO 工作流数据层大组合接口 (遵循 ISP 接口隔离原则拆分再优雅嵌入组合)
type IWorkflowDAO interface {
	IWorkflowCoreDAO
	ISnapshotDAO
}

type gormWorkflowDAO struct {
	db *gorm.DB
}

// NewWorkflowDAO 初始化工作流数据访问 GORM DAO
func NewWorkflowDAO(db *gorm.DB) IWorkflowDAO {
	return &gormWorkflowDAO{
		db: db,
	}
}

// --- Workflow 核心流程定义接口实现 ---

func (g *gormWorkflowDAO) CreateWorkflow(ctx context.Context, w Workflow) (int64, error) {
	now := time.Now().UnixMilli()
	w.Ctime = now
	w.Utime = now
	err := g.db.WithContext(ctx).Create(&w).Error
	return w.Id, err
}

func (g *gormWorkflowDAO) List(ctx context.Context, offset, limit int64) ([]Workflow, error) {
	var ws []Workflow
	// NOTE: 使用 Omit 屏蔽大 JSON 字段 of flow_data, 提升网络 IO 性能
	err := g.db.WithContext(ctx).
		Omit("flow_data").
		Order("ctime desc").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&ws).Error
	return ws, err
}

func (g *gormWorkflowDAO) Count(ctx context.Context) (int64, error) {
	var total int64
	err := g.db.WithContext(ctx).Model(&Workflow{}).Count(&total).Error
	return total, err
}

func (g *gormWorkflowDAO) UpdateWorkflow(ctx context.Context, w Workflow) (int64, error) {
	updates := map[string]interface{}{
		"name":          w.Name,
		"desc":          w.Desc,
		"owner":         w.Owner,
		"is_notify":     w.IsNotify,
		"notify_method": w.NotifyMethod,
		"flow_data":     w.FlowData,
		"utime":         time.Now().UnixMilli(),
	}
	result := g.db.WithContext(ctx).Model(&Workflow{}).Where("id = ?", w.Id).Updates(updates)
	return result.RowsAffected, result.Error
}

func (g *gormWorkflowDAO) UpdateProcessId(ctx context.Context, id int64, processId int) error {
	updates := map[string]interface{}{
		"process_id": processId,
		"utime":      time.Now().UnixMilli(),
	}
	return g.db.WithContext(ctx).Model(&Workflow{}).Where("id = ?", id).Updates(updates).Error
}

func (g *gormWorkflowDAO) DeleteWorkflow(ctx context.Context, id int64) (int64, error) {
	result := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Workflow{})
	return result.RowsAffected, result.Error
}

func (g *gormWorkflowDAO) FindWorkflow(ctx context.Context, id int64) (Workflow, error) {
	var w Workflow
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&w).Error
	return w, err
}

func (g *gormWorkflowDAO) FindByIds(ctx context.Context, ids []int64) ([]Workflow, error) {
	var ws []Workflow
	if len(ids) == 0 {
		return ws, nil
	}
	err := g.db.WithContext(ctx).
		Omit("flow_data").
		Where("id IN ?", ids).
		Find(&ws).Error
	return ws, err
}

func (g *gormWorkflowDAO) FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]Workflow, error) {
	var ws []Workflow
	query := g.db.WithContext(ctx).Omit("flow_data")
	if keyword != "" {
		likePattern := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR `desc` LIKE ?", likePattern, likePattern)
	}
	err := query.Order("ctime desc").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&ws).Error
	return ws, err
}

func (g *gormWorkflowDAO) CountByKeyword(ctx context.Context, keyword string) (int64, error) {
	var total int64
	query := g.db.WithContext(ctx).Model(&Workflow{})
	if keyword != "" {
		likePattern := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR `desc` LIKE ?", likePattern, likePattern)
	}
	err := query.Count(&total).Error
	return total, err
}

// --- Snapshot 引擎版本快照接口实现 ---

func (g *gormWorkflowDAO) CreateSnapshot(ctx context.Context, snapshot Snapshot) error {
	snapshot.Ctime = time.Now().UnixMilli()
	return g.db.WithContext(ctx).Create(&snapshot).Error
}

func (g *gormWorkflowDAO) FindSnapshotByProcess(ctx context.Context, processID, version int) (Snapshot, error) {
	var s Snapshot
	err := g.db.WithContext(ctx).
		Where("process_id = ? AND process_version = ?", processID, version).
		First(&s).Error
	return s, err
}
