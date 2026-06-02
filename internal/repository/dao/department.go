package dao

import (
	"context"

	"github.com/Duke1616/eflow/pkg/sqlx"
	"gorm.io/gorm"
)

// Department 部门 GORM 物理表实体结构
type Department struct {
	Id         int64                    `gorm:"primaryKey;column:id;type:bigint;autoIncrement;comment:'部门自增ID'"`
	TenantID   int64                    `gorm:"column:tenant_id;type:bigint;not null;index;comment:'多租户隔离标识'"`
	Pid        int64                    `gorm:"column:pid;type:bigint;not null;comment:'父级部门ID'"`
	Name       string                   `gorm:"column:name;type:varchar(128);not null;comment:'部门名称'"`
	Sort       int64                    `gorm:"column:sort;type:bigint;comment:'排序值'"`
	Enabled    bool                     `gorm:"column:enabled;type:tinyint(1);not null;default:1;comment:'是否启用启用'"`
	Leaders    sqlx.JsonField[[]string] `gorm:"column:leaders;type:json;comment:'部门负责人 username 列表json'"`
	MainLeader string                   `gorm:"column:main_leader;type:varchar(128);comment:'分管领导唯一 username'"`
	Ctime      int64                    `gorm:"column:ctime;type:bigint;comment:'创建时间(毫秒)'"`
	Utime      int64                    `gorm:"column:utime;type:bigint;comment:'修改时间(毫秒)'"`
}

type DepartmentDAO interface {
	FindById(ctx context.Context, id int64) (Department, error)
}

type gormDepartmentDAO struct {
	db *gorm.DB
}

// NewDepartmentDAO 构造部门物理数据访问层
func NewDepartmentDAO(db *gorm.DB) DepartmentDAO {
	return &gormDepartmentDAO{db: db}
}

func (g *gormDepartmentDAO) FindById(ctx context.Context, id int64) (Department, error) {
	var res Department
	err := g.db.WithContext(ctx).Where("id = ?", id).First(&res).Error
	return res, err
}
