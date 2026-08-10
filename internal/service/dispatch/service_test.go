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
	svc := NewService(repo, &templateRepositoryStub{})

	_, err := svc.Create(context.Background(), domain.Dispatch{TemplateId: 1, AutomationNodeID: "node-1"})
	require.ErrorContains(t, err, "必须选择有效的执行单元")
	_, err = svc.Update(context.Background(), domain.Dispatch{
		Id: 1, TemplateId: 1, AutomationNodeID: "node-1", RunnerId: -1,
	})
	require.ErrorContains(t, err, "必须选择有效的执行单元")
	require.Zero(t, repo.writeCalls)
}

func TestCreateAndUpdateRejectMissingAutomationNode(t *testing.T) {
	repo := &dispatchRepositoryStub{}
	svc := NewService(repo, &templateRepositoryStub{})

	_, err := svc.Create(context.Background(), domain.Dispatch{TemplateId: 1, RunnerId: 10})
	require.ErrorContains(t, err, "必须关联自动化节点")
	_, err = svc.Update(context.Background(), domain.Dispatch{Id: 1, TemplateId: 1, RunnerId: 10})
	require.ErrorContains(t, err, "必须关联自动化节点")
	require.Zero(t, repo.writeCalls)
}

func TestCreateNormalizesRuleAndDefaultPriority(t *testing.T) {
	repo := &dispatchRepositoryStub{}
	svc := NewService(repo, &templateRepositoryStub{})

	_, err := svc.Create(context.Background(), domain.Dispatch{
		TemplateId: 1, AutomationNodeID: " node-1 ", RunnerId: 10,
		Field: " region ", Value: " cn-north ",
	})

	require.NoError(t, err)
	require.Equal(t, "node-1", repo.created.AutomationNodeID)
	require.Equal(t, "region", repo.created.Field)
	require.Equal(t, "cn-north", repo.created.Value)
	require.Equal(t, domain.DefaultDispatchPriority, repo.created.Priority)
}

func TestCreateRejectsDuplicateCondition(t *testing.T) {
	repo := &dispatchRepositoryStub{conditionExists: true}
	svc := NewService(repo, &templateRepositoryStub{})

	_, err := svc.Create(context.Background(), validDispatch())

	require.ErrorContains(t, err, "已经存在相同匹配条件")
	require.Zero(t, repo.writeCalls)
}

func TestUpdateExcludesCurrentRuleFromConflictCheck(t *testing.T) {
	repo := &dispatchRepositoryStub{}
	svc := NewService(repo, &templateRepositoryStub{})
	rule := validDispatch()
	rule.Id = 42

	_, err := svc.Update(context.Background(), rule)

	require.NoError(t, err)
	require.Equal(t, int64(42), repo.excludedID)
	require.Equal(t, int64(1), repo.checkedTemplateID)
}

func TestReplaceFromTemplateRejectsMissingRunner(t *testing.T) {
	repo := &dispatchRepositoryStub{
		dispatches: []domain.Dispatch{{Id: 16, TemplateId: 10, AutomationNodeID: "node-1", RunnerId: 0}},
	}
	svc := NewService(repo, sameWorkflowTemplates())

	_, err := svc.ReplaceFromTemplate(context.Background(), 20, 10)

	require.ErrorContains(t, err, "来源模板包含无效路由规则 16")
	require.Zero(t, repo.writeCalls)
}

func TestReplaceFromTemplateRejectsDifferentWorkflow(t *testing.T) {
	repo := &dispatchRepositoryStub{}
	templates := &templateRepositoryStub{templates: map[int64]domain.Template{
		10: {Id: 10, WorkflowId: 1},
		20: {Id: 20, WorkflowId: 2},
	}}
	svc := NewService(repo, templates)

	_, err := svc.ReplaceFromTemplate(context.Background(), 20, 10)

	require.ErrorContains(t, err, "只能复制同一工作流")
	require.Zero(t, repo.writeCalls)
}

func TestReplaceFromTemplateReplacesEmptySource(t *testing.T) {
	repo := &dispatchRepositoryStub{}
	svc := NewService(repo, sameWorkflowTemplates())

	count, err := svc.ReplaceFromTemplate(context.Background(), 20, 10)

	require.NoError(t, err)
	require.Zero(t, count)
	require.Equal(t, 1, repo.writeCalls)
	require.Equal(t, int64(20), repo.replacedTemplateID)
}

func TestReplaceFromTemplateRejectsMissingAutomationNode(t *testing.T) {
	repo := &dispatchRepositoryStub{
		dispatches: []domain.Dispatch{{Id: 16, TemplateId: 10, RunnerId: 10}},
	}
	svc := NewService(repo, sameWorkflowTemplates())

	_, err := svc.ReplaceFromTemplate(context.Background(), 20, 10)

	require.ErrorContains(t, err, "来源模板包含无效路由规则 16")
	require.ErrorContains(t, err, "必须关联自动化节点")
	require.Zero(t, repo.writeCalls)
}

func validDispatch() domain.Dispatch {
	return domain.Dispatch{
		TemplateId: 1, AutomationNodeID: "node-1", RunnerId: 10,
		Field: "region", Value: "cn-north", Priority: 100,
	}
}

func sameWorkflowTemplates() *templateRepositoryStub {
	return &templateRepositoryStub{templates: map[int64]domain.Template{
		10: {Id: 10, WorkflowId: 1},
		20: {Id: 20, WorkflowId: 1},
	}}
}

type dispatchRepositoryStub struct {
	repository.DispatchRepository
	dispatches         []domain.Dispatch
	writeCalls         int
	created            domain.Dispatch
	conditionExists    bool
	excludedID         int64
	checkedTemplateID  int64
	replacedTemplateID int64
}

func (s *dispatchRepositoryStub) Create(_ context.Context, req domain.Dispatch) (int64, error) {
	s.writeCalls++
	s.created = req
	return 1, nil
}

func (s *dispatchRepositoryStub) Update(context.Context, domain.Dispatch) (int64, error) {
	s.writeCalls++
	return 1, nil
}

func (s *dispatchRepositoryStub) ListAllByTemplateID(context.Context, int64) ([]domain.Dispatch, error) {
	return s.dispatches, nil
}

func (s *dispatchRepositoryStub) ExistsCondition(_ context.Context, excludeID, templateID int64,
	_, _, _ string) (bool, error) {
	s.excludedID = excludeID
	s.checkedTemplateID = templateID
	return s.conditionExists, nil
}

func (s *dispatchRepositoryStub) ReplaceByTemplate(_ context.Context, templateID int64,
	rules []domain.Dispatch) (int64, error) {
	s.writeCalls++
	s.replacedTemplateID = templateID
	return int64(len(rules)), nil
}

type templateRepositoryStub struct {
	templates map[int64]domain.Template
}

func (s *templateRepositoryStub) DetailTemplate(_ context.Context, id int64) (domain.Template, error) {
	return s.templates[id], nil
}
