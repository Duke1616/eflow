package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
)

type DepartmentRepository interface {
	FindById(ctx context.Context, id int64) (domain.Department, error)
}

type departmentRepository struct {
	dao dao.DepartmentDAO
}

// NewDepartmentRepository 构造部门数据仓库
func NewDepartmentRepository(dao dao.DepartmentDAO) DepartmentRepository {
	return &departmentRepository{dao: dao}
}

func (repo *departmentRepository) FindById(ctx context.Context, id int64) (domain.Department, error) {
	d, err := repo.dao.FindById(ctx, id)
	return repo.toDomain(d), err
}

func (repo *departmentRepository) toDomain(src dao.Department) domain.Department {
	return domain.Department{
		Id:         src.Id,
		Pid:        src.Pid,
		Name:       src.Name,
		Sort:       src.Sort,
		Enabled:    src.Enabled,
		Leaders:    src.Leaders.Val,
		MainLeader: src.MainLeader,
	}
}
