package dispatch

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/stretchr/testify/require"
)

func TestCreateAndUpdateRejectMissingRunner(t *testing.T) {
	repo := &dispatchRepositoryStub{}
	svc := NewService(repo)

	_, err := svc.Create(context.Background(), domain.Dispatch{TemplateId: 1})
	require.ErrorContains(t, err, "必须选择有效的执行单元")
	_, err = svc.Update(context.Background(), domain.Dispatch{Id: 1, RunnerId: -1})
	require.ErrorContains(t, err, "必须选择有效的执行单元")
	require.Zero(t, repo.writeCalls)
}

func TestSyncRejectsMissingRunner(t *testing.T) {
	repo := &dispatchRepositoryStub{
		count:      1,
		dispatches: []domain.Dispatch{{Id: 16, TemplateId: 10, RunnerId: 0}},
	}
	svc := NewService(repo)

	_, total, err := svc.Sync(context.Background(), 20, 10)

	require.Equal(t, int64(1), total)
	require.ErrorContains(t, err, "来源模板包含无效派发规则 16")
	require.Zero(t, repo.writeCalls)
}

type dispatchRepositoryStub struct {
	repository.DispatchRepository
	count      int64
	dispatches []domain.Dispatch
	writeCalls int
}

func (s *dispatchRepositoryStub) Create(context.Context, domain.Dispatch) (int64, error) {
	s.writeCalls++
	return 1, nil
}

func (s *dispatchRepositoryStub) Update(context.Context, domain.Dispatch) (int64, error) {
	s.writeCalls++
	return 1, nil
}

func (s *dispatchRepositoryStub) CountByTemplateId(context.Context, int64) (int64, error) {
	return s.count, nil
}

func (s *dispatchRepositoryStub) ListByTemplateId(context.Context, int64, int64, int64) ([]domain.Dispatch, error) {
	return s.dispatches, nil
}

func (s *dispatchRepositoryStub) Sync(context.Context, int64, []domain.Dispatch) (int64, error) {
	s.writeCalls++
	return 1, nil
}
