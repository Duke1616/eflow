package withdrawal

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/internal/repository"
	engineSvc "github.com/Duke1616/eflow/internal/service/engine"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eiam/pkg/ctxutil"
)

// CompensationTarget 是成功动作在流程撤回时需要执行的补偿节点。
type CompensationTarget struct {
	NodeID   string
	NodeName string
}

// CompensationPlan 是流程引擎撤回前生成的只读补偿计划。
type CompensationPlan struct {
	TicketID int64
	TenantID int64
	Targets  []CompensationTarget
}

// CompensationPlanner 根据已成功动作生成补偿计划，并在流程引擎撤回后幂等激活计划。
type CompensationPlanner interface {
	// Build 在流程撤回前生成只读补偿计划，不改变自动化任务状态。
	Build(ctx context.Context, instanceID domain.ID) (CompensationPlan, error)
	// Apply 在流程引擎撤回后幂等激活补偿计划并取消其余待执行任务。
	Apply(ctx context.Context, instanceID domain.ID, plan CompensationPlan) error
}

type compensationPlanner struct {
	taskRepo       repository.TaskRepository
	withdrawalRepo repository.WithdrawalRepository
	tasks          taskSvc.Service
	tickets        ticketSvc.Service
	engine         engineSvc.IEngine
	workflows      workflowSvc.Service
}

func NewCompensationPlanner(taskRepo repository.TaskRepository, withdrawalRepo repository.WithdrawalRepository,
	tasks taskSvc.Service, tickets ticketSvc.Service, engine engineSvc.IEngine,
	workflows workflowSvc.Service) CompensationPlanner {
	return &compensationPlanner{
		taskRepo: taskRepo, withdrawalRepo: withdrawalRepo, tasks: tasks,
		tickets: tickets, engine: engine, workflows: workflows,
	}
}

func (p *compensationPlanner) Build(ctx context.Context, instanceID domain.ID) (CompensationPlan, error) {
	tasks, err := p.listTasks(ctx, instanceID)
	if err != nil {
		return CompensationPlan{}, err
	}

	seen := make(map[string]struct{})
	targetIDs := make([]string, 0)
	var source *domain.Task
	for i := range tasks {
		current := &tasks[i]
		if current.Status != domain.TaskStatusSuccess || current.CompensationNodeID == "" {
			continue
		}
		if _, ok := seen[current.CompensationNodeID]; ok {
			continue
		}
		seen[current.CompensationNodeID] = struct{}{}
		targetIDs = append(targetIDs, current.CompensationNodeID)
		if source == nil {
			source = current
		}
	}
	if source == nil {
		return CompensationPlan{}, nil
	}

	ticket, err := p.tickets.GetByID(ctx, source.TicketID)
	if err != nil {
		return CompensationPlan{}, err
	}
	ctx = withdrawalTenantContext(ctx, ticket.TenantID)
	engineType := ticket.Process.ResolveEngineType()
	// 走统一门面而非 easyflow 专属服务：flowx 实例只以雪花 Key 寻址，
	// 用 easyflow 的 GetInstanceByID 会让带补偿节点的 flowx 撤回必然失败。
	view, err := p.engine.GetInstanceExecutionView(ctx, engineType, instanceID)
	if err != nil {
		return CompensationPlan{}, fmt.Errorf("查询流程实例失败: %w", err)
	}
	flow, err := p.workflows.FindInstanceFlow(ctx, ticket.WorkflowId, view.ProcessID, view.ProcessVersion, engineType)
	if err != nil {
		return CompensationPlan{}, fmt.Errorf("查询流程实例快照失败: %w", err)
	}

	graph, err := flowdsl.Parse(flow.FlowData)
	if err != nil {
		return CompensationPlan{}, fmt.Errorf("解析流程拓扑失败: %w", err)
	}

	plan := CompensationPlan{TicketID: ticket.Id, TenantID: ticket.TenantID}
	for _, nodeID := range targetIDs {
		node, ok := graph.GetNode(nodeID)
		if !ok {
			return CompensationPlan{}, fmt.Errorf("查询补偿节点 %s 失败: 节点不存在", nodeID)
		}
		plan.Targets = append(plan.Targets, CompensationTarget{NodeID: nodeID, NodeName: node.Name})
	}
	return plan, nil
}

func (p *compensationPlanner) Apply(ctx context.Context, instanceID domain.ID, plan CompensationPlan) error {
	ctx = withdrawalTenantContext(ctx, plan.TenantID)
	nodeIDs := make([]string, 0, len(plan.Targets))
	for _, target := range plan.Targets {
		if _, err := p.tasks.CreateTask(ctx, plan.TicketID, instanceID,
			target.NodeID, target.NodeName); err != nil {
			return fmt.Errorf("补建补偿任务 %s 失败: %w", target.NodeID, err)
		}
		nodeIDs = append(nodeIDs, target.NodeID)
	}
	if err := p.withdrawalRepo.ActivateCompensations(ctx, instanceID, nodeIDs); err != nil {
		return fmt.Errorf("激活撤回补偿任务失败: %w", err)
	}
	return nil
}

func (p *compensationPlanner) listTasks(ctx context.Context, instanceID domain.ID) ([]domain.Task, error) {
	const pageSize int64 = 100
	var result []domain.Task
	for offset := int64(0); ; offset += pageSize {
		page, err := p.taskRepo.ListByInstanceID(ctx, offset, pageSize, instanceID.Int64())
		if err != nil {
			return nil, err
		}
		result = append(result, page...)
		if int64(len(page)) < pageSize {
			return result, nil
		}
	}
}

func withdrawalTenantContext(ctx context.Context, tenantID int64) context.Context {
	if tenantID <= 0 {
		return ctx
	}
	ctx = ctxutil.WithTenantID(ctx, tenantID)
	return ctxutil.WithOriginTenantID(ctx, tenantID)
}
