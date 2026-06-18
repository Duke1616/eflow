package dao

import (
	"context"
	"errors"
	"time"

	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/xen0n/go-workwx"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Rule 局部定义，使 DAO 彻底解耦 Domain 包依赖，保障物理防腐层洁净度
type Rule map[string]interface{}

// TemplateOptions 局部定义，用于物理层表单扩展选项的无损映射
type TemplateOptions map[string]interface{}

// ErrTemplateGroupNotEmpty 删除分组前发现分组内仍存在模板
var ErrTemplateGroupNotEmpty = errors.New("请先删除分组下的模板后再删除分组")

// TemplateFavorite 模版收藏实体定义
type TemplateFavorite struct {
	Id         int64 `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'收藏关系主键ID'"`
	TenantID   int64 `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	UserId     int64 `gorm:"column:user_id;type:bigint;not null;index;comment:'收藏的用户ID'"`
	TemplateId int64 `gorm:"column:template_id;type:bigint;not null;index;comment:'被收藏的工单模版ID'"`
	Ctime      int64 `gorm:"column:ctime;type:bigint;comment:'收藏时间(毫秒戳)'"`
	Utime      int64 `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒戳)'"`
}

// TableName 指定物理表名，规避 GORM 默认复数表名机制可能引发的找表报错
func (TemplateFavorite) TableName() string {
	return "template_favorite"
}

// TemplateGroup 模版分组实体定义
type TemplateGroup struct {
	Id       int64  `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'分组自增主键ID'"`
	TenantID int64  `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	Name     string `gorm:"column:name;type:varchar(128);not null;comment:'分组名称展示'"`
	Icon     string `gorm:"column:icon;type:varchar(256);comment:'分组关联图标'"`
	Ctime    int64  `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime    int64  `gorm:"column:utime;type:bigint;comment:'修改时间(毫秒)'"`
}

// TemplateGroupSummary 模板分组摘要查询结果
type TemplateGroupSummary struct {
	Id    int64  `gorm:"column:id"`
	Name  string `gorm:"column:name"`
	Icon  string `gorm:"column:icon"`
	Total int64  `gorm:"column:total"`
}

// TableName 指定物理表名
func (TemplateGroup) TableName() string {
	return "template_group"
}

// Template 工单模版实体定义
type Template struct {
	Id                 int64                                     `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'工单模版自增ID'"`
	TenantID           int64                                     `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	Name               string                                    `gorm:"column:name;type:varchar(128);not null;comment:'工单模板展示名称'"`
	WorkflowId         int64                                     `gorm:"column:workflow_id;type:bigint;not null;index;comment:'绑定的工作流流程ID'"`
	GroupId            int64                                     `gorm:"column:group_id;type:bigint;not null;index;comment:'归属的模板分组ID'"`
	Icon               string                                    `gorm:"column:icon;type:varchar(256);comment:'工单模板图标'"`
	CreateType         uint8                                     `gorm:"column:create_type;type:tinyint unsigned;not null;comment:'工单创建方式 1:直接创建 2:审批流触发'"`
	Rules              sqlx.JsonField[[]Rule]                    `gorm:"column:rules;type:json;comment:'表单校验与规则控制链json'"`
	Options            sqlx.JsonField[TemplateOptions]           `gorm:"column:options;type:json;comment:'工单扩展选项配置json'"`
	ExternalTemplateId string                                    `gorm:"column:external_template_id;type:varchar(128);index;comment:'外部对接系统模版ID'"`
	UniqueHash         string                                    `gorm:"column:unique_hash;type:varchar(128);index;comment:'内容唯一摘要哈希值'"`
	WechatOAControls   sqlx.JsonField[workwx.OATemplateControls] `gorm:"column:wechat_oa_controls;type:json;comment:'企微审批流程表单控件快照json'"`
	Desc               string                                    `gorm:"column:desc;type:text;comment:'工单模板备注信息'"`
	Ctime              int64                                     `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime              int64                                     `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒)'"`
}

// TableName 指定物理表名
func (Template) TableName() string {
	return "template"
}

// ITemplateCoreDAO 工单模板核心物理数据访问接口
type ITemplateCoreDAO interface {
	// CreateTemplate 在物理数据层创建一条模板记录，返回新插入记录的主键 ID
	CreateTemplate(ctx context.Context, t Template) (int64, error)
	// FindByHash 通过内容的唯一摘要哈希值检索对应的模板记录，多用于防重校验
	FindByHash(ctx context.Context, hash string) (Template, error)
	// FindByExternalTemplateId 通过绑定的第三方外部系统模板 ID（如企微 OA 模板 ID）获取模板信息
	FindByExternalTemplateId(ctx context.Context, externalTemplateId string) (Template, error)
	// DetailTemplate 获取对应主键 ID 的单个工单模板的详细配置属性
	DetailTemplate(ctx context.Context, id int64) (Template, error)
	// DetailTemplateByExternalTemplateId 通过外部第三方模板 ID 检索单个模板的详细属性
	DetailTemplateByExternalTemplateId(ctx context.Context, externalId string) (Template, error)
	// DeleteTemplate 根据主键 ID 删除指定的工单模板，返回受影响的物理行数
	DeleteTemplate(ctx context.Context, id int64) (int64, error)
	// UpdateTemplate 覆盖更新物理层中的模板基本字段配置，返回受影响行数
	UpdateTemplate(ctx context.Context, t Template) (int64, error)
	// ListTemplate 根据分页大小及偏移量获取工单模板列表，groupId 大于 0 时按分组过滤，keyword 非空时按名称或描述模糊搜索（按创建时间逆序）
	ListTemplate(ctx context.Context, groupId int64, keyword string, offset, limit int64) ([]Template, error)
	// Count 统计当前租户空间下物理模板的总记录数，groupId 大于 0 时按分组过滤，keyword 非空时按名称或描述模糊搜索
	Count(ctx context.Context, groupId int64, keyword string) (int64, error)
	// FindByTemplateIds 根据一批指定的主键 ID 批量检索对应的模板记录列表
	FindByTemplateIds(ctx context.Context, ids []int64) ([]Template, error)
	// GetByWorkflowId 检索与指定的工作流流程定义 ID 绑定的所有工单模板列表
	GetByWorkflowId(ctx context.Context, workflowId int64) ([]Template, error)
}

// ITemplateGroupDAO 模板分类分组物理数据访问接口
type ITemplateGroupDAO interface {
	// CreateGroup 新建一个模板所属的分类分组实体记录，返回生成的自增 ID
	CreateGroup(ctx context.Context, g TemplateGroup) (int64, error)
	// UpdateGroup 更新模板分组基本信息，返回受影响行数
	UpdateGroup(ctx context.Context, g TemplateGroup) (int64, error)
	// DeleteGroup 删除模板分组，删除前校验分组下没有模板
	DeleteGroup(ctx context.Context, id int64) (int64, error)
	// ListGroup 分页获取模板的分类分组列表（按创建时间逆序排列）
	ListGroup(ctx context.Context, offset, limit int64) ([]TemplateGroup, error)
	// CountGroup 统计系统当前可用的模板分类分组总条数
	CountGroup(ctx context.Context) (int64, error)
	// ListGroupSummaries 获取模板分组摘要及每组模板数量
	ListGroupSummaries(ctx context.Context) ([]TemplateGroupSummary, error)
}

// ITemplateFavoriteDAO 模板收藏物理数据访问接口
type ITemplateFavoriteDAO interface {
	// ToggleFavorite 切换当前用户针对指定工单模板的收藏状态（若已存在则取消收藏，若不存在则添加），并采用事务行级锁排它防范并发死锁
	ToggleFavorite(ctx context.Context, userId int64, templateId int64) (bool, error)
	// ListTemplateIdsByUserId 获取指定用户 ID 在当前租户空间下收藏的所有模板的主键 ID 列表
	ListTemplateIdsByUserId(ctx context.Context, userId int64) ([]int64, error)
}

// ITemplateDAO 工单模板多表物理数据访问组合接口 (通过接口隔离拆分，再经接口嵌入优雅组合)
type ITemplateDAO interface {
	ITemplateCoreDAO
	ITemplateGroupDAO
	ITemplateFavoriteDAO
}

type gormTemplateDAO struct {
	db *gorm.DB
}

// NewTemplateDAO 初始化工单模板 GORM DAO
func NewTemplateDAO(db *gorm.DB) ITemplateDAO {
	return &gormTemplateDAO{
		db: db,
	}
}

// --- Template 物理访问实现 ---

func (g *gormTemplateDAO) CreateTemplate(ctx context.Context, t Template) (int64, error) {
	now := time.Now().UnixMilli()
	t.Ctime = now
	t.Utime = now
	err := g.db.WithContext(ctx).Create(&t).Error
	return t.Id, err
}

func (g *gormTemplateDAO) FindByHash(ctx context.Context, hash string) (Template, error) {
	var t Template
	err := g.db.WithContext(ctx).Where("unique_hash = ?", hash).First(&t).Error
	return t, err
}

func (g *gormTemplateDAO) FindByExternalTemplateId(ctx context.Context, externalTemplateId string) (Template, error) {
	var t Template
	err := g.db.WithContext(ctx).Where("external_template_id = ?", externalTemplateId).First(&t).Error
	return t, err
}

func (g *gormTemplateDAO) DetailTemplate(ctx context.Context, id int64) (Template, error) {
	var t Template
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&t).Error
	return t, err
}

func (g *gormTemplateDAO) DetailTemplateByExternalTemplateId(ctx context.Context, externalId string) (Template, error) {
	var t Template
	err := g.db.WithContext(ctx).Where("external_template_id = ?", externalId).First(&t).Error
	return t, err
}

func (g *gormTemplateDAO) DeleteTemplate(ctx context.Context, id int64) (int64, error) {
	result := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Template{})
	return result.RowsAffected, result.Error
}

func (g *gormTemplateDAO) UpdateTemplate(ctx context.Context, t Template) (int64, error) {
	updates := map[string]interface{}{
		"name":        t.Name,
		"workflow_id": t.WorkflowId,
		"group_id":    t.GroupId,
		"icon":        t.Icon,
		"desc":        t.Desc,
		"rules":       t.Rules,
		"options":     t.Options,
		"utime":       time.Now().UnixMilli(),
	}
	result := g.db.WithContext(ctx).Model(&Template{}).Where("id = ?", t.Id).Updates(updates)
	return result.RowsAffected, result.Error
}

func (g *gormTemplateDAO) ListTemplate(ctx context.Context, groupId int64, keyword string, offset, limit int64) ([]Template, error) {
	var ts []Template
	query := g.db.WithContext(ctx)
	if groupId > 0 {
		query = query.Where("group_id = ?", groupId)
	}
	if keyword != "" {
		likePattern := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR `desc` LIKE ?", likePattern, likePattern)
	}
	err := query.Order("ctime desc").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&ts).Error
	return ts, err
}

func (g *gormTemplateDAO) Count(ctx context.Context, groupId int64, keyword string) (int64, error) {
	var total int64
	query := g.db.WithContext(ctx).Model(&Template{})
	if groupId > 0 {
		query = query.Where("group_id = ?", groupId)
	}
	if keyword != "" {
		likePattern := "%" + keyword + "%"
		query = query.Where("name LIKE ? OR `desc` LIKE ?", likePattern, likePattern)
	}
	err := query.Count(&total).Error
	return total, err
}

func (g *gormTemplateDAO) FindByTemplateIds(ctx context.Context, ids []int64) ([]Template, error) {
	var ts []Template
	if len(ids) == 0 {
		return ts, nil
	}
	err := g.db.WithContext(ctx).Where("id IN ?", ids).Find(&ts).Error
	return ts, err
}

func (g *gormTemplateDAO) GetByWorkflowId(ctx context.Context, workflowId int64) ([]Template, error) {
	var ts []Template
	err := g.db.WithContext(ctx).
		Where("workflow_id = ?", workflowId).
		Order("ctime desc").
		Find(&ts).Error
	return ts, err
}

// --- TemplateGroup 分组物理访问实现 ---

func (g *gormTemplateDAO) CreateGroup(ctx context.Context, group TemplateGroup) (int64, error) {
	now := time.Now().UnixMilli()
	group.Ctime = now
	group.Utime = now
	err := g.db.WithContext(ctx).Create(&group).Error
	return group.Id, err
}

func (g *gormTemplateDAO) UpdateGroup(ctx context.Context, group TemplateGroup) (int64, error) {
	updates := map[string]interface{}{
		"name":  group.Name,
		"icon":  group.Icon,
		"utime": time.Now().UnixMilli(),
	}
	result := g.db.WithContext(ctx).Model(&TemplateGroup{}).Where("id = ?", group.Id).Updates(updates)
	return result.RowsAffected, result.Error
}

func (g *gormTemplateDAO) DeleteGroup(ctx context.Context, id int64) (int64, error) {
	var rowsAffected int64
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var total int64
		if err := tx.Model(&Template{}).Where("group_id = ?", id).Count(&total).Error; err != nil {
			return err
		}
		if total > 0 {
			return ErrTemplateGroupNotEmpty
		}

		result := tx.Where("id = ?", id).Delete(&TemplateGroup{})
		rowsAffected = result.RowsAffected
		return result.Error
	})
	return rowsAffected, err
}

func (g *gormTemplateDAO) ListGroup(ctx context.Context, offset, limit int64) ([]TemplateGroup, error) {
	var gs []TemplateGroup
	err := g.db.WithContext(ctx).
		Order("ctime desc").
		Limit(int(limit)).
		Offset(int(offset)).
		Find(&gs).Error
	return gs, err
}

func (g *gormTemplateDAO) CountGroup(ctx context.Context) (int64, error) {
	var total int64
	err := g.db.WithContext(ctx).Model(&TemplateGroup{}).Count(&total).Error
	return total, err
}

func (g *gormTemplateDAO) ListGroupSummaries(ctx context.Context) ([]TemplateGroupSummary, error) {
	var summaries []TemplateGroupSummary
	err := g.db.WithContext(ctx).
		Table("template_group AS tg").
		Select("tg.id, tg.name, tg.icon, COUNT(t.id) AS total").
		Joins("LEFT JOIN template AS t ON t.group_id = tg.id").
		Group("tg.id, tg.name, tg.icon").
		Order("tg.ctime desc").
		Scan(&summaries).Error
	return summaries, err
}

// --- Favorite 收藏物理访问实现 ---

func (g *gormTemplateDAO) ToggleFavorite(ctx context.Context, userId int64, templateId int64) (bool, error) {
	var fav TemplateFavorite
	// NOTE: 使用 GORM 的数据库物理事务，防止高并发下 First-or-Create 造成的脏写或唯一索引冲突
	err := g.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 使用 SELECT ... FOR UPDATE 对对应记录加行级锁，确保并发下数据的绝对安全性
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND template_id = ?", userId, templateId).
			First(&fav).Error

		if err == nil {
			// 存在则取消收藏，行锁会在事务提交后自动释放
			return tx.Delete(&fav).Error
		}

		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 不存在则创建关联
			now := time.Now().UnixMilli()
			newFav := TemplateFavorite{
				UserId:     userId,
				TemplateId: templateId,
				Ctime:      now,
				Utime:      now,
			}
			return tx.Create(&newFav).Error
		}
		return err
	})

	if err != nil {
		return false, err
	}

	// 如果 fav.Id > 0，说明此前查到了记录并在事务中删除了它，即最新状态为“未收藏”(false)
	return fav.Id == 0, nil
}

func (g *gormTemplateDAO) ListTemplateIdsByUserId(ctx context.Context, userId int64) ([]int64, error) {
	var ids []int64
	err := g.db.WithContext(ctx).
		Model(&TemplateFavorite{}).
		Where("user_id = ?", userId).
		Pluck("template_id", &ids).Error
	return ids, err
}
