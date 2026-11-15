package withdrawal

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestCompensationPlannerBuildIsReadOnly(t *testing.T) {
	testCases := []struct {
		name           string
		instanceID     domain.ID
		engineType     domain.EngineType
		wantListedInst int64
	}{
		{
			name:           "easyflow 自增实例标识",
			instanceID:     domain.NewID(101),
			engineType:     domain.EngineTypeEasyFlow,
			wantListedInst: 101,
		},
		{
			name:           "flowx 雪花实例标识",
			instanceID:     domain.NewID("7395001234567890123"),
			engineType:     domain.EngineTypeFlowx,
			wantListedInst: 7395001234567890123,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			taskRepo := &plannerTaskRepositoryStub{tasks: []domain.Task{{
				ID: 1, TenantID: 7, TicketID: 42, InstanceID: tc.instanceID,
				Status: domain.TaskStatusSuccess, CompensationNodeID: "compensation",
			}}}
			tasks := &plannerTaskServiceStub{}
			withdrawals := &plannerWithdrawalRepositoryStub{}
			engine := &plannerEngineStub{view: domain.InstanceExecutionView{ProcessID: 10, ProcessVersion: 2}}
			workflows := &plannerWorkflowServiceStub{nodeName: "权限回收"}
			planner := NewCompensationPlanner(
				taskRepo, withdrawals, tasks,
				&plannerTicketServiceStub{ticket: domain.Ticket{
					Id: 42, TenantID: 7, WorkflowId: 9,
					Process: domain.Process{EngineType: tc.engineType, InstanceID: tc.instanceID},
				}},
				engine, workflows,
			)

			plan, err := planner.Build(context.Background(), tc.instanceID)

			require.NoError(t, err)
			require.Equal(t, CompensationPlan{
				TicketID: 42, TenantID: 7,
				Targets: []CompensationTarget{{NodeID: "compensation", NodeName: "权限回收"}},
			}, plan)
			require.Zero(t, tasks.calls)
			require.Zero(t, withdrawals.activateCalls)
			// 实例标识必须原样下推到任务查询与引擎门面，截断成 int 或退化成 0 都会让 flowx 补偿计划查空
			require.Equal(t, tc.wantListedInst, taskRepo.listedInstanceID)
			require.Equal(t, tc.instanceID, engine.viewInstanceID)
			require.Equal(t, tc.engineType, engine.viewEngineType)
			require.Equal(t, tc.engineType, workflows.engineType)
			require.Equal(t, 2, workflows.version)
		})
	}
}

func TestCompensationPlannerApplyCreatesAndActivatesCompensation(t *testing.T) {
	testCases := []struct {
		name       string
		instanceID domain.ID
	}{
		{name: "easyflow 自增实例标识", instanceID: domain.NewID(101)},
		{name: "flowx 雪花实例标识", instanceID: domain.NewID("7395001234567890123")},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tasks := &plannerTaskServiceStub{}
			withdrawals := &plannerWithdrawalRepositoryStub{}
			planner := NewCompensationPlanner(&plannerTaskRepositoryStub{}, withdrawals, tasks,
				&plannerTicketServiceStub{}, &plannerEngineStub{}, &plannerWorkflowServiceStub{})
			plan := CompensationPlan{TicketID: 42, TenantID: 7,
				Targets: []CompensationTarget{{NodeID: "compensation", NodeName: "权限回收"}}}

			err := planner.Apply(context.Background(), tc.instanceID, plan)

			require.NoError(t, err)
			require.Equal(t, 1, tasks.calls)
			require.Equal(t, "compensation", tasks.nodeID)
			require.Equal(t, int64(7), tasks.tenantID)
			require.Equal(t, tc.instanceID, tasks.instanceID)
			require.Equal(t, tc.instanceID, withdrawals.instanceID)
			require.Equal(t, []string{"compensation"}, withdrawals.nodeIDs)
		})
	}
}

type plannerTaskRepositoryStub struct {
	repository.TaskRepository
	tasks            []domain.Task
	listedInstanceID int64
}

func (s *plannerTaskRepositoryStub) ListByInstanceID(_ context.Context, offset, _ int64,
	instanceID int64) ([]domain.Task, error) {
	s.listedInstanceID = instanceID
	if offset > 0 {
		return nil, nil
	}
	return s.tasks, nil
}

type plannerWithdrawalRepositoryStub struct {
	repository.WithdrawalRepository
	activateCalls int
	nodeIDs       []string
	instanceID    domain.ID
}

func (s *plannerWithdrawalRepositoryStub) ActivateCompensations(_ context.Context,
	instanceID domain.ID, nodeIDs []string) error {
	s.activateCalls++
	s.instanceID = instanceID
	s.nodeIDs = nodeIDs
	return nil
}

type plannerTaskServiceStub struct {
	taskSvc.Service
	calls      int
	nodeID     string
	tenantID   int64
	instanceID domain.ID
}

func (s *plannerTaskServiceStub) CreateTask(ctx context.Context, _ int64, instanceID domain.ID,
	nodeID, _ string, _ ...int) (domain.Task, error) {
	s.calls++
	s.nodeID = nodeID
	s.instanceID = instanceID
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

type plannerEngineStub struct {
	engineSvc.IEngine
	view           domain.InstanceExecutionView
	viewInstanceID domain.ID
	viewEngineType domain.EngineType
}

func (s *plannerEngineStub) GetInstanceExecutionView(_ context.Context,
	engineType domain.EngineType, instanceID domain.ID) (domain.InstanceExecutionView, error) {
	s.viewEngineType = engineType
	s.viewInstanceID = instanceID
	return s.view, nil
}

type plannerWorkflowServiceStub struct {
	workflowSvc.Service
	nodeName   string
	engineType domain.EngineType
	version    int
}

func (s *plannerWorkflowServiceStub) FindInstanceFlow(_ context.Context, _ int64, _ int,
	version int, engineType ...domain.EngineType) (domain.Workflow, error) {
	s.version = version
	if len(engineType) > 0 {
		s.engineType = engineType[0]
	}
	return domain.Workflow{
		FlowData: map[string]any{
			"nodes": []map[string]any{
				{
					"id":   "compensation",
					"type": "automation",
					"properties": map[string]any{
						"name": s.nodeName,
					},
				},
			},
			"edges": []map[string]any{},
		},
	}, nil
}
