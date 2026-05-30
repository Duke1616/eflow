package codebook

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	"github.com/Duke1616/eflow/internal/repository"
	"golang.org/x/sync/errgroup"
)

type Service interface {
	// Create 创建一条全新的脚本模板业务数据
	Create(ctx context.Context, req domain.Codebook) (int64, error)
	// GetByID 根据 ID 查询详细 of 脚本模板数据
	GetByID(ctx context.Context, id int64) (domain.Codebook, error)
	// List 分页加载脚本模板信息，并并发聚合获取数据总条数
	List(ctx context.Context, offset, limit int64) ([]domain.Codebook, int64, error)
	// Update 变更已存盘的脚本模板信息
	Update(ctx context.Context, req domain.Codebook) (int64, error)
	// Delete 删除指定的脚本模板记录
	Delete(ctx context.Context, id int64) (int64, error)
	// VerifySecret 校验唯一标识码与其 Secret 密钥的匹配性是否合法
	VerifySecret(ctx context.Context, identifier string, secret string) (bool, error)
	// GetByIdentifier 依据业务唯一标识符反查脚本
	GetByIdentifier(ctx context.Context, identifier string) (domain.Codebook, error)
	// ListByIdentifiers 批量查询标识符范围内的所有可用脚本
	ListByIdentifiers(ctx context.Context, identifiers []string) ([]domain.Codebook, error)
}

type service struct {
	repo repository.CodebookRepository
}

func NewService(repo repository.CodebookRepository) Service {
	return &service{
		repo: repo,
	}
}

func (s *service) ListByIdentifiers(ctx context.Context, identifiers []string) ([]domain.Codebook, error) {
	return s.repo.ListByIdentifiers(ctx, identifiers)
}

func (s *service) GetByIdentifier(ctx context.Context, identifier string) (domain.Codebook, error) {
	return s.repo.GetByIdentifier(ctx, identifier)
}

func (s *service) VerifySecret(ctx context.Context, identifier string, secret string) (bool, error) {
	_, err := s.repo.FindBySecret(ctx, identifier, secret)
	if !errors.Is(err, repository.ErrCodebookNotFound) {
		return true, err
	}

	return false, err
}

func (s *service) Create(ctx context.Context, req domain.Codebook) (int64, error) {
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return s.repo.Create(ctx, req)
}

func (s *service) GetByID(ctx context.Context, id int64) (domain.Codebook, error) {
	if id <= 0 {
		return domain.Codebook{}, fmt.Errorf("%w: Id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.GetByID(ctx, id)
}

func (s *service) List(ctx context.Context, offset, limit int64) ([]domain.Codebook, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Codebook
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.List(ctx, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.Total(ctx)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *service) Update(ctx context.Context, req domain.Codebook) (int64, error) {
	if req.Id <= 0 {
		return 0, fmt.Errorf("%w: Id = %d", errs.ErrInvalidParameter, req.Id)
	}
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return s.repo.Update(ctx, req)
}

func (s *service) Delete(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("%w: Id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.Delete(ctx, id)
}
