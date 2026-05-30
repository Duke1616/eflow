package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrCodebookNotFound = gorm.ErrRecordNotFound

type CodebookRepository interface {
	// Create 调用 DAO 存盘一条新的脚本模板，并生成 UUID Secret 密钥
	Create(ctx context.Context, req domain.Codebook) (int64, error)
	// GetByID 根据主键 ID 获取转换为领域对象的脚本模板
	GetByID(ctx context.Context, id int64) (domain.Codebook, error)
	// List 分页获取领域脚本模板集合
	List(ctx context.Context, offset, limit int64) ([]domain.Codebook, error)
	// Total 统计当前租户下总共有效的脚本模板记录数
	Total(ctx context.Context) (int64, error)
	// Update 更新已有的脚本模板内容，仅允许修改部分核心属性
	Update(ctx context.Context, req domain.Codebook) (int64, error)
	// Delete 根据主键 ID 物理删除仓储中的脚本模板记录
	Delete(ctx context.Context, id int64) (int64, error)
	// FindBySecret 根据标识码和 Secret 双密钥锁定脚本模板
	FindBySecret(ctx context.Context, identifier string, secret string) (domain.Codebook, error)
	// GetByIdentifier 根据唯一业务标识码获取单一脚本领域模型
	GetByIdentifier(ctx context.Context, identifier string) (domain.Codebook, error)
	// ListByIdentifiers 批量拉取符合给定标识码列表的脚本实体集合
	ListByIdentifiers(ctx context.Context, identifiers []string) ([]domain.Codebook, error)
}

type codebookRepository struct {
	dao dao.CodebookDAO
}

func NewCodebookRepository(dao dao.CodebookDAO) CodebookRepository {
	return &codebookRepository{
		dao: dao,
	}
}

func (repo *codebookRepository) ListByIdentifiers(ctx context.Context, identifiers []string) ([]domain.Codebook, error) {
	codes, err := repo.dao.ListByIdentifiers(ctx, identifiers)
	if err != nil {
		return nil, err
	}
	return slice.Map(codes, func(idx int, src dao.Codebook) domain.Codebook {
		return repo.toDomain(src)
	}), nil
}

func (repo *codebookRepository) GetByIdentifier(ctx context.Context, identifier string) (domain.Codebook, error) {
	c, err := repo.dao.GetByIdentifier(ctx, identifier)
	if err != nil {
		return domain.Codebook{}, err
	}
	return repo.toDomain(c), nil
}

func (repo *codebookRepository) Create(ctx context.Context, req domain.Codebook) (int64, error) {
	return repo.dao.Create(ctx, repo.toEntity(req))
}

func (repo *codebookRepository) GetByID(ctx context.Context, id int64) (domain.Codebook, error) {
	t, err := repo.dao.GetByID(ctx, id)
	if err != nil {
		return domain.Codebook{}, err
	}
	return repo.toDomain(t), nil
}

func (repo *codebookRepository) List(ctx context.Context, offset, limit int64) ([]domain.Codebook, error) {
	ts, err := repo.dao.List(ctx, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src dao.Codebook) domain.Codebook {
		return repo.toDomain(src)
	}), nil
}

func (repo *codebookRepository) Update(ctx context.Context, req domain.Codebook) (int64, error) {
	return repo.dao.Update(ctx, repo.toEntity(req))
}

func (repo *codebookRepository) Delete(ctx context.Context, id int64) (int64, error) {
	return repo.dao.Delete(ctx, id)
}

func (repo *codebookRepository) FindBySecret(ctx context.Context, identifier string, secret string) (domain.Codebook, error) {
	c, err := repo.dao.FindBySecret(ctx, identifier, secret)
	if err != nil {
		return domain.Codebook{}, err
	}
	return repo.toDomain(c), nil
}

func (repo *codebookRepository) Total(ctx context.Context) (int64, error) {
	return repo.dao.Count(ctx)
}

func (repo *codebookRepository) toEntity(req domain.Codebook) dao.Codebook {
	return dao.Codebook{
		Id:         req.Id,
		TenantID:   req.TenantID,
		Name:       req.Name,
		Owner:      req.Owner,
		Code:       req.Code,
		Language:   req.Language,
		Secret:     uuid.NewString(),
		Identifier: req.Identifier,
	}
}

func (repo *codebookRepository) toDomain(req dao.Codebook) domain.Codebook {
	return domain.Codebook{
		Id:         req.Id,
		TenantID:   req.TenantID,
		Name:       req.Name,
		Owner:      req.Owner,
		Code:       req.Code,
		Language:   req.Language,
		Secret:     req.Secret,
		Identifier: req.Identifier,
	}
}
