package department

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
)

type Service interface {
	FindById(ctx context.Context, id int64) (domain.Department, error)
}

type service struct {
	repo repository.DepartmentRepository
}

// NewService 构造部门领域服务
func NewService(repo repository.DepartmentRepository) Service {
	return &service{repo: repo}
}

func (s *service) FindById(ctx context.Context, id int64) (domain.Department, error) {
	return s.repo.FindById(ctx, id)
}
