package dispatch

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"golang.org/x/sync/errgroup"
)

// Service 自动派发服务业务逻辑接口
type Service interface {
	// Create 新建自动派发规则并存盘
	Create(ctx context.Context, req domain.Dispatch) (int64, error)
	// Update 局部修改指定的自动派发规则
	Update(ctx context.Context, req domain.Dispatch) (int64, error)
	// Delete 删除指定的自动派发规则
	Delete(ctx context.Context, id int64) (int64, error)
	// ListByTemplateId 依据模板 ID 分页获取所有的自动派发规则与总计数，采用 errgroup 并发优化查询速度
	ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, int64, error)
	// Sync 全量同步来源模板的自动派发规则到目标模板，返回新增条数和来源总条数
	Sync(ctx context.Context, templateId, syncTemplateId int64) (int64, int64, error)
}

type service struct {
	repo repository.DispatchRepository
}

// Sync 执行批量数据同步
func (s *service) Sync(ctx context.Context, templateId, syncTemplateId int64) (int64, int64, error) {
	total, err := s.repo.CountByTemplateId(ctx, syncTemplateId)
	if err != nil {
		return 0, 0, err
	}
	if total == 0 {
		return 0, 0, nil
	}

	ds, err := s.repo.ListByTemplateId(ctx, 0, total, syncTemplateId)
	if err != nil {
		return 0, total, err
	}
	count, err := s.repo.Sync(ctx, templateId, ds)
	return count, total, err
}

// Delete 删除指定自动派发规则
func (s *service) Delete(ctx context.Context, id int64) (int64, error) {
	return s.repo.Delete(ctx, id)
}

// Create 创建自动派发规则
func (s *service) Create(ctx context.Context, req domain.Dispatch) (int64, error) {
	return s.repo.Create(ctx, req)
}

// Update 修改自动派发规则
func (s *service) Update(ctx context.Context, req domain.Dispatch) (int64, error) {
	return s.repo.Update(ctx, req)
}

// ListByTemplateId 分页获取并统计派发规则条数，利用 errgroup 异步加速
func (s *service) ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Dispatch
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListByTemplateId(ctx, offset, limit, templateId)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountByTemplateId(ctx, templateId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

// NewService 初始化自动派发 Service 实例
func NewService(repo repository.DispatchRepository) Service {
	return &service{
		repo: repo,
	}
}
