package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
)

// DispatchRepository 执行单元条件路由的数据仓储接口。
type DispatchRepository interface {
	// Create 新建路由规则。
	Create(ctx context.Context, req domain.Dispatch) (int64, error)
	// Update 更新路由规则。
	Update(ctx context.Context, req domain.Dispatch) (int64, error)
	// Delete 依据主键 ID 删除路由规则。
	Delete(ctx context.Context, id int64) (int64, error)
	// ListByTemplateId 依据模板 ID 分页获取其关联的领域模型列表
	ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, error)
	// ListAllByTemplateID 获取模板的全部路由规则。
	ListAllByTemplateID(ctx context.Context, templateID int64) ([]domain.Dispatch, error)
	// ListByTemplateNode 获取自动化节点的全部 Runner 路由规则。
	ListByTemplateNode(ctx context.Context, templateID int64, nodeID string) ([]domain.Dispatch, error)
	// CountByTemplateId 获取指定模板 ID 关联的规则数量
	CountByTemplateId(ctx context.Context, templateId int64) (int64, error)
	// ExistsCondition 判断相同路由条件是否已经存在。
	ExistsCondition(ctx context.Context, excludeID, templateID int64,
		nodeID, field, value string) (bool, error)
	// ReplaceByTemplate 完整替换模板的条件路由规则。
	ReplaceByTemplate(ctx context.Context, templateID int64, rules []domain.Dispatch) (int64, error)
}

type dispatchRepository struct {
	dao dao.DispatchDAO
}

func (repo *dispatchRepository) ReplaceByTemplate(ctx context.Context, templateID int64,
	rules []domain.Dispatch) (int64, error) {
	return repo.dao.ReplaceByTemplate(ctx, templateID, slice.Map(rules, func(_ int, src domain.Dispatch) dao.Dispatch {
		return repo.toEntity(src)
	}))
}

// Delete 删除指定路由规则。
func (repo *dispatchRepository) Delete(ctx context.Context, id int64) (int64, error) {
	return repo.dao.Delete(ctx, id)
}

// Create 写入执行单元路由规则。
func (repo *dispatchRepository) Create(ctx context.Context, req domain.Dispatch) (int64, error) {
	return repo.dao.Create(ctx, repo.toEntity(req))
}

// Update 修改执行单元路由规则。
func (repo *dispatchRepository) Update(ctx context.Context, req domain.Dispatch) (int64, error) {
	return repo.dao.Update(ctx, repo.toEntity(req))
}

// ListByTemplateId 获取路由规则并转换为领域实体。
func (repo *dispatchRepository) ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, error) {
	ds, err := repo.dao.ListByTemplateId(ctx, offset, limit, templateId)
	return slice.Map(ds, func(idx int, src dao.Dispatch) domain.Dispatch {
		return repo.toDomain(src)
	}), err
}

func (repo *dispatchRepository) ListAllByTemplateID(ctx context.Context,
	templateID int64) ([]domain.Dispatch, error) {
	ds, err := repo.dao.ListAllByTemplateID(ctx, templateID)
	return slice.Map(ds, func(_ int, src dao.Dispatch) domain.Dispatch {
		return repo.toDomain(src)
	}), err
}

func (repo *dispatchRepository) ListByTemplateNode(ctx context.Context, templateID int64,
	nodeID string) ([]domain.Dispatch, error) {
	ds, err := repo.dao.ListByTemplateNode(ctx, templateID, nodeID)
	return slice.Map(ds, func(_ int, src dao.Dispatch) domain.Dispatch {
		return repo.toDomain(src)
	}), err
}

// CountByTemplateId 获取统计条数
func (repo *dispatchRepository) CountByTemplateId(ctx context.Context, templateId int64) (int64, error) {
	return repo.dao.CountByTemplateId(ctx, templateId)
}

func (repo *dispatchRepository) ExistsCondition(ctx context.Context, excludeID, templateID int64,
	nodeID, field, value string) (bool, error) {
	return repo.dao.ExistsCondition(ctx, excludeID, templateID, nodeID, field, value)
}

// NewDispatchRepository 初始化仓储层实现
func NewDispatchRepository(dao dao.DispatchDAO) DispatchRepository {
	return &dispatchRepository{
		dao: dao,
	}
}

func (repo *dispatchRepository) toDomain(src dao.Dispatch) domain.Dispatch {
	return domain.Dispatch{
		Id:               src.Id,
		TemplateId:       src.TemplateId,
		AutomationNodeID: src.AutomationNodeID,
		RunnerId:         src.RunnerId,
		Field:            src.Field,
		Value:            src.Value,
		Priority:         src.Priority,
	}
}

func (repo *dispatchRepository) toEntity(src domain.Dispatch) dao.Dispatch {
	return dao.Dispatch{
		Id:               src.Id,
		TemplateId:       src.TemplateId,
		AutomationNodeID: src.AutomationNodeID,
		RunnerId:         src.RunnerId,
		Field:            src.Field,
		Value:            src.Value,
		Priority:         src.Priority,
	}
}
