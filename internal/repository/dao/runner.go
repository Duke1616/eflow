package dao

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

// IRunnerDAO 执行器数据访问接口
type IRunnerDAO interface {
	// Create 在数据库中插入一条新的执行器记录
	Create(ctx context.Context, r Runner) (int64, error)
	// Update 局部更新指定的执行器，内部维护修改时间等信息
	Update(ctx context.Context, req Runner) (int64, error)
	// Delete 根据 ID 删除对应的执行器存盘记录
	Delete(ctx context.Context, id int64) (int64, error)
	// FindById 根据主键 ID 从数据库加载执行器实体
	FindById(ctx context.Context, id int64) (Runner, error)
	// List 返回根据时间戳等规则排序的执行器分页列表
	List(ctx context.Context, offset, limit int64, keyword, kind string) ([]Runner, error)
	// Count 统计数据库中当前有效的执行器总记录数
	Count(ctx context.Context, keyword, kind string) (int64, error)
	// FindByCodebookUidAndTag 通过参数 UID 和内部 tags 匹配查找对应的执行器
	FindByCodebookUidAndTag(ctx context.Context, codebookUid string, tag string) (Runner, error)
	// ListByCodebookUid 查出关联到对应独立脚本 UID 的所有挂载执行器节点
	ListByCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]Runner, error)
	// CountByCodebookUid 统计通过脚本 UID 获取的具有承载特性的数据量
	CountByCodebookUid(ctx context.Context, codebookUid, keyword, kind string) (int64, error)
	// ListExcludeCodebookUid 返回过滤掉指定脚本 UID 后剩余可用的备选执行器列表
	ListExcludeCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]Runner, error)
	// CountExcludeCodebookUid 统计未关联某特征 UID 剩余执行器的池大小
	CountExcludeCodebookUid(ctx context.Context, codebookUid, keyword, kind string) (int64, error)
	// ListByCodebookUids 通过脚本 UID 列表批量获取其对应的执行器实体
	ListByCodebookUids(ctx context.Context, codebookUids []string) ([]Runner, error)
	// ListByIds 使用给定的指定 ID 列表过滤拉取所有的关联实体
	ListByIds(ctx context.Context, ids []int64) ([]Runner, error)
	// AggregateTags 聚合每个脚本绑定的节点和队列标记
	AggregateTags(ctx context.Context) ([]Runner, error)
}

type gormRunnerDAO struct {
	db *gorm.DB
}

// NewRunnerDAO 初始化 GORM 版的执行器 DAO
func NewRunnerDAO(db *gorm.DB) IRunnerDAO {
	return &gormRunnerDAO{
		db: db,
	}
}

// Runner 执行器运行节点物理数据表映射实体
type Runner struct {
	Id             int64                       `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'执行单元自增ID'"`
	TenantID       string                      `gorm:"column:tenant_id;type:varchar(64);not null;index;comment:'多租户隔离标识'"`
	Name           string                      `gorm:"column:name;type:varchar(128);not null;comment:'执行单元名称'"`
	CodebookUid    string                      `gorm:"column:codebook_uid;type:varchar(64);index;comment:'关联脚本库模板UID'"`
	CodebookSecret string                      `gorm:"column:codebook_secret;type:varchar(128);comment:'脚本模板认证密钥'"`
	Kind           string                      `gorm:"column:kind;type:varchar(32);comment:'派发管道协议(KAFKA/GRPC)'"`
	Target         string                      `gorm:"column:target;type:varchar(128);comment:'派发物理目标(Topic/ServiceName)'"`
	Handler        string                      `gorm:"column:handler;type:varchar(128);comment:'执行器承载业务方法'"`
	Tags           sqlx.JsonField[[]string]    `gorm:"column:tags;type:json;comment:'关联匹配的业务属性标签json'"`
	Action         uint8                       `gorm:"column:action;type:tinyint unsigned;comment:'活跃动作状态 1:REGISTER 2:UNREGISTER'"`
	Desc           string                      `gorm:"column:desc;type:text;comment:'备注说明'"`
	Variables      sqlx.JsonField[[]Variables] `gorm:"column:variables;type:json;comment:'执行器环境变量默认值json'"`
	Ctime          int64                       `gorm:"column:ctime;type:bigint;comment:'注册创建时间(毫秒)'"`
	Utime          int64                       `gorm:"column:utime;type:bigint;comment:'最近心跳心跳/更新时间(毫秒)'"`
}

// Variables 作业/执行器传递的环境变量物理映射结构
type Variables struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Secret bool   `json:"secret"`
}

func (g *gormRunnerDAO) Create(ctx context.Context, r Runner) (int64, error) {
	now := time.Now().UnixMilli()
	r.Ctime, r.Utime = now, now
	err := g.db.WithContext(ctx).Create(&r).Error
	return r.Id, err
}

func (g *gormRunnerDAO) Update(ctx context.Context, req Runner) (int64, error) {
	res := g.db.WithContext(ctx).
		Model(&Runner{}).
		Where("id = ?", req.Id).
		Updates(map[string]any{
			"name":            req.Name,
			"codebook_secret": req.CodebookSecret,
			"kind":            req.Kind,
			"target":          req.Target,
			"handler":         req.Handler,
			"tags":            req.Tags,
			"desc":            req.Desc,
			"variables":       req.Variables,
			"utime":           time.Now().UnixMilli(),
		})
	return res.RowsAffected, res.Error
}

func (g *gormRunnerDAO) Delete(ctx context.Context, id int64) (int64, error) {
	res := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Runner{})
	return res.RowsAffected, res.Error
}

func (g *gormRunnerDAO) FindById(ctx context.Context, id int64) (Runner, error) {
	var res Runner
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}

func (g *gormRunnerDAO) List(ctx context.Context, offset, limit int64, keyword, kind string) ([]Runner, error) {
	var res []Runner
	query := g.db.WithContext(ctx)
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormRunnerDAO) Count(ctx context.Context, keyword, kind string) (int64, error) {
	var count int64
	query := g.db.WithContext(ctx).Model(&Runner{})
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Count(&count).Error
	return count, err
}

func (g *gormRunnerDAO) FindByCodebookUidAndTag(ctx context.Context, codebookUid string, tag string) (Runner, error) {
	var res Runner
	// 针对 Tags JSON 字段在 MySQL 中的查询，使用 JSON_CONTAINS 判定
	err := g.db.WithContext(ctx).
		Where("codebook_uid = ? AND JSON_CONTAINS(tags, ?)", codebookUid, fmt.Sprintf("%q", tag)).
		First(&res).Error
	return res, err
}

func (g *gormRunnerDAO) ListByCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]Runner, error) {
	var res []Runner
	query := g.db.WithContext(ctx).Where("codebook_uid = ?", codebookUid)
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormRunnerDAO) CountByCodebookUid(ctx context.Context, codebookUid, keyword, kind string) (int64, error) {
	var count int64
	query := g.db.WithContext(ctx).Model(&Runner{}).Where("codebook_uid = ?", codebookUid)
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Count(&count).Error
	return count, err
}

func (g *gormRunnerDAO) ListExcludeCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]Runner, error) {
	var res []Runner
	query := g.db.WithContext(ctx).Where("codebook_uid != ?", codebookUid)
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Order("ctime desc").Offset(int(offset)).Limit(int(limit)).Find(&res).Error
	return res, err
}

func (g *gormRunnerDAO) CountExcludeCodebookUid(ctx context.Context, codebookUid, keyword, kind string) (int64, error) {
	var count int64
	query := g.db.WithContext(ctx).Model(&Runner{}).Where("codebook_uid != ?", codebookUid)
	if keyword != "" {
		query = query.Where("name LIKE ?", "%"+keyword+"%")
	}
	if kind != "" {
		query = query.Where("kind = ?", kind)
	}
	err := query.Count(&count).Error
	return count, err
}

func (g *gormRunnerDAO) ListByCodebookUids(ctx context.Context, codebookUids []string) ([]Runner, error) {
	var res []Runner
	err := g.db.WithContext(ctx).Where("codebook_uid IN ?", codebookUids).Find(&res).Error
	return res, err
}

func (g *gormRunnerDAO) ListByIds(ctx context.Context, ids []int64) ([]Runner, error) {
	var res []Runner
	err := g.db.WithContext(ctx).Where("id IN ?", ids).Find(&res).Error
	return res, err
}

func (g *gormRunnerDAO) AggregateTags(ctx context.Context) ([]Runner, error) {
	var res []Runner
	err := g.db.WithContext(ctx).Find(&res).Error
	return res, err
}
