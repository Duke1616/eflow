package service

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/errs"
	"github.com/Duke1616/eflow/internal/repository"
	"golang.org/x/sync/errgroup"
)

// IRunner 执行器（Runner）服务接口
type IRunner interface {
	// Create 注册一个新的执行器节点
	Create(ctx context.Context, req domain.Runner) (int64, error)
	// Update 更新现有执行器的属性与配置信息
	Update(ctx context.Context, req domain.Runner) (int64, error)
	// FindById 获取单个执行器的详细配置信息
	FindById(ctx context.Context, id int64) (domain.Runner, error)
	// Delete 根据 ID 删除指定的执行器
	Delete(ctx context.Context, id int64) (int64, error)
	// List 分页获取执行器节点列表及总数
	List(ctx context.Context, offset, limit int64, keyword, kind string) ([]domain.Runner, int64, error)
	// FindByCodebookUidAndTag 根据绑定的脚本 UID 和特定策略标签匹配指定的执行器节点
	FindByCodebookUidAndTag(ctx context.Context, codebookUid string, tag string) (domain.Runner, error)
	// ListByCodebookUid 根据脚本 UID 获取其所有具备承载能力的执行器节点（支持分页）
	ListByCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]domain.Runner, int64, error)
	// ListExcludeCodebookUid 获取由于未绑定特定脚本 UID，剩余支持额外添加的执行器节点（支持分页）
	ListExcludeCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]domain.Runner, int64, error)
	// ListByCodebookUids 根据多个脚本 UID 批量拉取能承载相关任务的执行器节点
	ListByCodebookUids(ctx context.Context, codebookUids []string) ([]domain.Runner, error)
	// ListByIds 根据一组内建的 ID 列表批量拉取执行器对象
	ListByIds(ctx context.Context, ids []int64) ([]domain.Runner, error)
	// AggregateTags 提取并聚合所有剧本关联的 Runner 标签拓扑体系
	AggregateTags(ctx context.Context) ([]domain.RunnerTags, error)
}

type runnerService struct {
	repo repository.IRunnerRepository
}

// NewRunnerService 初始化执行器服务实例
func NewRunnerService(repo repository.IRunnerRepository) IRunner {
	return &runnerService{
		repo: repo,
	}
}

func (s *runnerService) Create(ctx context.Context, req domain.Runner) (int64, error) {
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return s.repo.Create(ctx, req)
}

func (s *runnerService) Update(ctx context.Context, req domain.Runner) (int64, error) {
	if req.Id <= 0 {
		return 0, fmt.Errorf("%w: Id = %d", errs.ErrInvalidParameter, req.Id)
	}
	if err := req.Validate(); err != nil {
		return 0, err
	}
	return s.repo.Update(ctx, req)
}

func (s *runnerService) FindById(ctx context.Context, id int64) (domain.Runner, error) {
	if id <= 0 {
		return domain.Runner{}, fmt.Errorf("%w: Id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.FindById(ctx, id)
}

func (s *runnerService) Delete(ctx context.Context, id int64) (int64, error) {
	if id <= 0 {
		return 0, fmt.Errorf("%w: Id = %d", errs.ErrInvalidParameter, id)
	}
	return s.repo.Delete(ctx, id)
}

func (s *runnerService) List(ctx context.Context, offset, limit int64, keyword, kind string) ([]domain.Runner, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Runner
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.List(ctx, offset, limit, keyword, kind)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.Count(ctx, keyword, kind)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *runnerService) FindByCodebookUidAndTag(ctx context.Context, codebookUid string, tag string) (domain.Runner, error) {
	return s.repo.FindByCodebookUidAndTag(ctx, codebookUid, tag)
}

func (s *runnerService) ListByCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]domain.Runner, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Runner
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListByCodebookUid(ctx, offset, limit, codebookUid, keyword, kind)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountByCodebookUid(ctx, codebookUid, keyword, kind)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *runnerService) ListExcludeCodebookUid(ctx context.Context, offset, limit int64, codebookUid, keyword, kind string) ([]domain.Runner, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Runner
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListExcludeCodebookUid(ctx, offset, limit, codebookUid, keyword, kind)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountExcludeCodebookUid(ctx, codebookUid, keyword, kind)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *runnerService) ListByCodebookUids(ctx context.Context, codebookUids []string) ([]domain.Runner, error) {
	return s.repo.ListByCodebookUids(ctx, codebookUids)
}

func (s *runnerService) ListByIds(ctx context.Context, ids []int64) ([]domain.Runner, error) {
	return s.repo.ListByIds(ctx, ids)
}

func (s *runnerService) AggregateTags(ctx context.Context) ([]domain.RunnerTags, error) {
	return s.repo.AggregateTags(ctx)
}
