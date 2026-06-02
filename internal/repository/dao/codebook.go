package dao

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type CodebookDAO interface {
	// Create 插入一条新的脚本模板记录，返回生成的主键 ID
	Create(ctx context.Context, c Codebook) (int64, error)
	// GetByID 根据主键 ID 获取唯一的脚本模板实体
	GetByID(ctx context.Context, id int64) (Codebook, error)
	// List 分页拉取脚本模板列表，按创建时间降序排序
	List(ctx context.Context, offset, limit int64) ([]Codebook, error)
	// Count 统计当前租户下脚本模板的总记录数
	Count(ctx context.Context) (int64, error)
	// Update 局部更新指定的脚本模板实体（仅修改 name, code, owner 和 utime）
	Update(ctx context.Context, c Codebook) (int64, error)
	// Delete 根据主键 ID 物理删除指定的脚本模板
	Delete(ctx context.Context, id int64) (int64, error)
	// FindBySecret 根据唯一标识码与密钥验证匹配 of 脚本模板实体
	FindBySecret(ctx context.Context, identifier string, secret string) (Codebook, error)
	// GetByIdentifier 根据脚本唯一标识码获取脚本实体
	GetByIdentifier(ctx context.Context, identifier string) (Codebook, error)
	// ListByIdentifiers 依据给定的标识码列表批量获取对应的脚本实体
	ListByIdentifiers(ctx context.Context, identifiers []string) ([]Codebook, error)
}

type gormCodebookDAO struct {
	db *gorm.DB
}

func NewCodebookDAO(db *gorm.DB) CodebookDAO {
	return &gormCodebookDAO{db: db}
}

// Codebook 脚本库实体定义
type Codebook struct {
	Id         int64  `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'脚本模板自增ID'"`
	TenantID   int64  `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	Name       string `gorm:"column:name;type:varchar(128);not null;comment:'脚本模板名称'"`
	Owner      string `gorm:"column:owner;type:varchar(128);not null;comment:'模板所有者'"`
	Identifier string `gorm:"column:identifier;type:varchar(64);not null;uniqueIndex;comment:'脚本唯一标识码'"`
	Code       string `gorm:"column:code;type:text;comment:'脚本源码快照内容'"`
	Language   string `gorm:"column:language;type:varchar(32);comment:'脚本编写语言(python/shell等)'"`
	Secret     string `gorm:"column:secret;type:varchar(128);comment:'敏感参数保护密钥'"`
	Ctime      int64  `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒戳)'"`
	Utime      int64  `gorm:"column:utime;type:bigint;comment:'更新时间(毫秒戳)'"`
}

func (g *gormCodebookDAO) Create(ctx context.Context, c Codebook) (int64, error) {
	now := time.Now().UnixMilli()
	c.Ctime, c.Utime = now, now
	err := g.db.WithContext(ctx).Create(&c).Error
	return c.Id, err
}

func (g *gormCodebookDAO) GetByID(ctx context.Context, id int64) (Codebook, error) {
	var res Codebook
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}

func (g *gormCodebookDAO) List(ctx context.Context, offset, limit int64) ([]Codebook, error) {
	var res []Codebook
	err := g.db.WithContext(ctx).
		Order("ctime desc").
		Offset(int(offset)).
		Limit(int(limit)).
		Find(&res).Error
	return res, err
}

func (g *gormCodebookDAO) Count(ctx context.Context) (int64, error) {
	var count int64
	err := g.db.WithContext(ctx).Model(&Codebook{}).Count(&count).Error
	return count, err
}

func (g *gormCodebookDAO) Update(ctx context.Context, c Codebook) (int64, error) {
	res := g.db.WithContext(ctx).
		Model(&Codebook{}).
		Where("id = ?", c.Id).
		Updates(map[string]any{
			"name":  c.Name,
			"code":  c.Code,
			"owner": c.Owner,
			"utime": time.Now().UnixMilli(),
		})
	return res.RowsAffected, res.Error
}

func (g *gormCodebookDAO) Delete(ctx context.Context, id int64) (int64, error) {
	res := g.db.WithContext(ctx).Where("id = ?", id).Delete(&Codebook{})
	return res.RowsAffected, res.Error
}

func (g *gormCodebookDAO) FindBySecret(ctx context.Context, identifier string, secret string) (Codebook, error) {
	var res Codebook
	err := g.db.WithContext(ctx).
		Where("identifier = ? AND secret = ?", identifier, secret).
		First(&res).Error
	return res, err
}

func (g *gormCodebookDAO) GetByIdentifier(ctx context.Context, identifier string) (Codebook, error) {
	var res Codebook
	err := g.db.WithContext(ctx).Where("identifier = ?", identifier).First(&res).Error
	return res, err
}

func (g *gormCodebookDAO) ListByIdentifiers(ctx context.Context, identifiers []string) ([]Codebook, error) {
	var res []Codebook
	err := g.db.WithContext(ctx).Where("identifier IN ?", identifiers).Find(&res).Error
	return res, err
}
