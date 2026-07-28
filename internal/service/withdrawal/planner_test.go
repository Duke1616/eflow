package withdrawal

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestCompensationPlannerBuildIsReadOnly(t *testing.T) {
	taskRepo := &plannerTaskRepositoryStub{tasks: []domain.Task{{
		ID: 1, TenantID: 7, TicketID: 42, ProcessInstanceID: 101,
		Status: domain.TaskStatusSuccess, CompensationNodeID: "compensation",
	}}}
	tasks := &plannerTaskServiceStub{}
	withdrawals := &plannerWithdrawalRepositoryStub{}
	planner := NewCompensationPlanner(
		taskRepo, withdrawals, tasks,
		&plannerTicketServiceStub{ticket: domain.Ticket{Id: 42, TenantID: 7, WorkflowId: 9}},
		&plannerEngineServiceStub{instance: domain.Instance{ProcID: 10, ProcVersion: 2}},
		&plannerWorkflowServiceStub{property: easyflow.AutomationProperty{Name: "权限回收"}},
	)

	plan, err := planner.Build(context.Background(), 101)

	require.NoError(t, err)
	require.Equal(t, CompensationPlan{
		TicketID: 42, TenantID: 7,
		Targets: []CompensationTarget{{NodeID: "compensation", NodeName: "权限回收"}},
	}, plan)
	require.Zero(t, tasks.calls)
	require.Zero(t, withdrawals.activateCalls)
}

func TestCompensationPlannerApplyCreatesAndActivatesCompensation(t *testing.T) {
	tasks := &plannerTaskServiceStub{}
	withdrawals := &plannerWithdrawalRepositoryStub{}
	planner := NewCompensationPlanner(&plannerTaskRepositoryStub{}, withdrawals, tasks,
		&plannerTicketServiceStub{}, &plannerEngineServiceStub{}, &plannerWorkflowServiceStub{})
	plan := CompensationPlan{TicketID: 42, TenantID: 7,
		Targets: []CompensationTarget{{NodeID: "compensation", NodeName: "权限回收"}}}

	err := planner.Apply(context.Background(), 101, plan)

	require.NoError(t, err)
	require.Equal(t, 1, tasks.calls)
	require.Equal(t, "compensation", tasks.nodeID)
	require.Equal(t, int64(7), tasks.tenantID)
	require.Equal(t, []string{"compensation"}, withdrawals.nodeIDs)
}

type plannerTaskRepositoryStub struct {
	repository.TaskRepository
	tasks []domain.Task
}

func (s *plannerTaskRepositoryStub) ListByInstanceID(_ context.Context, offset, _ int64,
	_ int) ([]domain.Task, error) {
	if offset > 0 {
		return nil, nil
	}
	return s.tasks, nil
}

type plannerWithdrawalRepositoryStub struct {
	repository.WithdrawalRepository
	activateCalls int
	nodeIDs       []string
}

func (s *plannerWithdrawalRepositoryStub) ActivateCompensations(_ context.Context,
	_ int, nodeIDs []string) error {
	s.activateCalls++
	s.nodeIDs = nodeIDs
	return nil
}

type plannerTaskServiceStub struct {
	taskSvc.Service
	calls    int
	nodeID   string
	tenantID int64
}

func (s *plannerTaskServiceStub) CreateTask(ctx context.Context, _ int64, _ int,
	nodeID, _ string) (domain.Task, error) {
	s.calls++
	s.nodeID = nodeID
	s.tenantID = ctxutil.GetTenantID(ctx).Int64()
	return domain.Task{}, nil
}

type plannerTicketServiceStub struct {
	ticketSvc.Service
	ticket domain.Ticket
}

func (s *plannerTicketServiceStub) GetByID(context.Context, int64) (domain.Ticket, error) {
	return s.ticket, nil
}

type plannerEngineServiceStub struct {
	engineSvc.Service
	instance domain.Instance
}

func (s *plannerEngineServiceStub) GetInstanceByID(context.Context, int) (domain.Instance, error) {
	return s.instance, nil
}

type plannerWorkflowServiceStub struct {
	workflowSvc.Service
	property easyflow.AutomationProperty
}

func (s *plannerWorkflowServiceStub) FindInstanceFlow(context.Context, int64, int, int) (domain.Workflow, error) {
	return domain.Workflow{}, nil
}

func (s *plannerWorkflowServiceStub) GetAutomationProperty(easyflow.Workflow,
	string) (easyflow.AutomationProperty, error) {
	return s.property, nil
}
