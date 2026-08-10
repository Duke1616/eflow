package task

import (
	"context"
	"encoding/json"
	"fmt"

	userv1 "github.com/Duke1616/eflow/api/proto/gen/eiam/user/v1"
	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
)

const taskPreparationOperation = "准备"

type preparation struct {
	automation easyflow.AutomationProperty
	input      domain.TaskArgs
}

type taskDefinition struct {
	scheduledAt        int64
	compensationNodeID string
}

func (s *taskService) prepareTaskDefinition(ctx context.Context, task domain.Task,
	ticket domain.Ticket) (taskDefinition, error) {
	prepared, err := s.resolvePreparation(ctx, task, ticket)
	definition := taskDefinition{
		compensationNodeID: prepared.automation.CompensationNodeID,
	}
	if err != nil {
		return definition, err
	}
	if err = s.applyTemplateScheduleOverride(ctx, task.NodeID, ticket.TemplateId, &prepared.automation); err != nil {
		return definition, s.taskError(task.ID, taskPreparationOperation, err)
	}
	scheduledAt, err := s.calculateScheduledAt(prepared.automation, prepared.input, ticket.TemplateId)
	if err != nil {
		return definition, err
	}
	definition.scheduledAt = scheduledAt
	return definition, nil
}

func (s *taskService) prepareAttempt(ctx context.Context,
	task domain.Task) (domain.RunnerRouteDecision, domain.TaskArgs, error) {
	ticket, err := s.tickets.GetByID(ctx, task.TicketID)
	if err != nil {
		return domain.RunnerRouteDecision{}, nil, s.taskError(task.ID, taskPreparationOperation, err)
	}
	prepared, err := s.resolvePreparation(ctx, task, ticket)
	if err != nil {
		return domain.RunnerRouteDecision{}, nil, err
	}
	decision, err := s.resolveRunner(ctx, ticket.TemplateId, task.NodeID, prepared.automation, prepared.input)
	if err != nil {
		return domain.RunnerRouteDecision{}, nil, s.taskError(task.ID, taskPreparationOperation, err)
	}
	prepared.input["ticket_id"] = task.TicketID
	prepared.input["process_inst_id"] = task.ProcessInstanceID
	return decision, prepared.input, nil
}

func (s *taskService) resolvePreparation(ctx context.Context, task domain.Task,
	ticket domain.Ticket) (preparation, error) {
	instance, err := s.engine.GetInstanceByID(ctx, task.ProcessInstanceID)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	flow, err := s.workflows.FindInstanceFlow(ctx, ticket.WorkflowId, instance.ProcID, instance.ProcVersion)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	automation, err := s.workflows.GetAutomationProperty(easyflow.FromDomainWorkflow(flow), task.NodeID)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	input, err := s.assembleRuntimeArgs(ctx, ticket)
	if err != nil {
		return preparation{automation: automation}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	return preparation{automation: automation, input: input}, nil
}

func (s *taskService) resolveRunner(ctx context.Context, templateID int64, nodeID string,
	automation easyflow.AutomationProperty, input domain.TaskArgs) (domain.RunnerRouteDecision, error) {
	decision := domain.RunnerRouteDecision{DefaultRunnerID: automation.RunnerID}
	if templateID > 0 && s.dispatches != nil {
		runner, ruleID, err := s.resolveRunnerByDispatch(ctx, templateID, nodeID, automation.CodebookId, input)
		if err != nil {
			return domain.RunnerRouteDecision{}, err
		}
		if ruleID > 0 {
			decision.SelectedRunnerID = runner.ID
			decision.RuleID = ruleID
			return decision, nil
		}
	}
	if automation.RunnerID <= 0 {
		return domain.RunnerRouteDecision{}, fmt.Errorf("未命中动态路由，且自动化节点未配置默认执行单元")
	}
	defaultRunner, err := s.runners.FindByID(ctx, automation.RunnerID)
	if err != nil {
		return domain.RunnerRouteDecision{}, fmt.Errorf("查询默认执行单元失败: %w", err)
	}
	if defaultRunner.CodebookID != automation.CodebookId {
		return domain.RunnerRouteDecision{}, fmt.Errorf(
			"默认执行单元 %d 未绑定自动化节点脚本 %d", defaultRunner.ID, automation.CodebookId)
	}
	decision.SelectedRunnerID = defaultRunner.ID
	return decision, nil
}

func (s *taskService) resolveRunnerByDispatch(ctx context.Context, templateID int64, nodeID string,
	codebookID int64, input domain.TaskArgs) (etaskclient.Runner, int64, error) {
	dispatches, err := s.dispatches.ListByTemplateNode(ctx, templateID, nodeID)
	if err != nil {
		return etaskclient.Runner{}, 0, fmt.Errorf("查询执行单元路由规则失败: %w", err)
	}
	for _, dispatch := range dispatches {
		actual, exists := input[dispatch.Field]
		if !exists || dispatch.Field == "" || fmt.Sprint(actual) != dispatch.Value {
			continue
		}
		if dispatch.RunnerId <= 0 {
			return etaskclient.Runner{}, 0, fmt.Errorf("路由规则 %d 缺少有效执行单元", dispatch.Id)
		}
		runner, findErr := s.runners.FindByID(ctx, dispatch.RunnerId)
		if findErr != nil {
			return etaskclient.Runner{}, 0, fmt.Errorf("查询路由规则执行单元失败: %w", findErr)
		}
		if runner.CodebookID != codebookID {
			return etaskclient.Runner{}, 0, fmt.Errorf(
				"路由规则执行单元 %d 未绑定自动化节点脚本 %d", runner.ID, codebookID)
		}
		return runner, dispatch.Id, nil
	}
	return etaskclient.Runner{}, 0, nil
}

func (s *taskService) assembleRuntimeArgs(ctx context.Context, ticket domain.Ticket) (domain.TaskArgs, error) {
	input := make(domain.TaskArgs, len(ticket.Data))
	for key, value := range ticket.Data {
		input[key] = value
	}
	forms, err := s.tickets.ListTaskFormsByTicketID(ctx, ticket.Id)
	if err != nil {
		return nil, err
	}
	for _, value := range forms {
		input[value.Key] = value.Value
	}
	response, err := s.users.QueryByUsername(ctx, &userv1.QueryByUsernameReq{Username: ticket.CreateBy})
	if err == nil && response.User != nil {
		data, marshalErr := json.Marshal(response.User)
		if marshalErr == nil {
			input["user_info"] = string(data)
		}
	}
	return input, nil
}
