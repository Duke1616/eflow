package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
)

// DispatchRepository 自动派发规则数据仓储接口
type DispatchRepository interface {
	// Create 新建并保存派发规则实体
	Create(ctx context.Context, req domain.Dispatch) (int64, error)
	// Update 局部更新派发规则实体
	Update(ctx context.Context, req domain.Dispatch) (int64, error)
	// Delete 依据主键 ID 删除对应的派发规则
	Delete(ctx context.Context, id int64) (int64, error)
	// ListByTemplateId 依据模板 ID 分页获取其关联的领域模型列表
	ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, error)
	// CountByTemplateId 获取指定模板 ID 关联的规则数量
	CountByTemplateId(ctx context.Context, templateId int64) (int64, error)
	// Sync 批量事务同步模板的自动派发规则
	Sync(ctx context.Context, templateId int64, ds []domain.Dispatch) (int64, error)
}

type dispatchRepository struct {
	dao dao.DispatchDAO
}

// Sync 同步领域模型派发规则，内部会自动将其转化为 DAO 实体
func (repo *dispatchRepository) Sync(ctx context.Context, templateId int64, ds []domain.Dispatch) (int64, error) {
	return repo.dao.Sync(ctx, templateId, slice.Map(ds, func(idx int, src domain.Dispatch) dao.Dispatch {
		return repo.toEntity(src)
	}))
}

// Delete 删除指定派发规则
func (repo *dispatchRepository) Delete(ctx context.Context, id int64) (int64, error) {
	return repo.dao.Delete(ctx, id)
}

// Create 写入自动派发规则
func (repo *dispatchRepository) Create(ctx context.Context, req domain.Dispatch) (int64, error) {
	return repo.dao.Create(ctx, repo.toEntity(req))
}

// Update 修改自动派发规则
func (repo *dispatchRepository) Update(ctx context.Context, req domain.Dispatch) (int64, error) {
	return repo.dao.Update(ctx, repo.toEntity(req))
}

// ListByTemplateId 拉取派发规则并转换为 domain 实体
func (repo *dispatchRepository) ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, error) {
	ds, err := repo.dao.ListByTemplateId(ctx, offset, limit, templateId)
	return slice.Map(ds, func(idx int, src dao.Dispatch) domain.Dispatch {
		return repo.toDomain(src)
	}), err
}

// CountByTemplateId 获取统计条数
func (repo *dispatchRepository) CountByTemplateId(ctx context.Context, templateId int64) (int64, error) {
	return repo.dao.CountByTemplateId(ctx, templateId)
}

// NewDispatchRepository 初始化仓储层实现
func NewDispatchRepository(dao dao.DispatchDAO) DispatchRepository {
	return &dispatchRepository{
		dao: dao,
	}
}

func (repo *dispatchRepository) toDomain(src dao.Dispatch) domain.Dispatch {
	return domain.Dispatch{
		Id:         src.Id,
		TemplateId: src.TemplateId,
		RunnerId:   src.RunnerId,
		Field:      src.Field,
		Value:      src.Value,
	}
}

func (repo *dispatchRepository) toEntity(src domain.Dispatch) dao.Dispatch {
	return dao.Dispatch{
		Id:         src.Id,
		TemplateId: src.TemplateId,
		RunnerId:   src.RunnerId,
		Field:      src.Field,
		Value:      src.Value,
	}
}
