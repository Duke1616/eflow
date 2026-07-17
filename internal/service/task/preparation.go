package task

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

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

func (s *taskService) prepareSchedule(ctx context.Context, task domain.Task,
	ticket domain.Ticket) (int64, error) {
	prepared, err := s.resolvePreparation(ctx, task, ticket)
	if err != nil {
		return 0, err
	}
	return s.calculateScheduledAt(prepared.automation, prepared.input)
}

func (s *taskService) prepareAttempt(ctx context.Context,
	task domain.Task) (int64, domain.TaskArgs, error) {
	ticket, err := s.tickets.GetByID(ctx, task.TicketID)
	if err != nil {
		return 0, nil, s.taskError(task.ID, taskPreparationOperation, err)
	}
	prepared, err := s.resolvePreparation(ctx, task, ticket)
	if err != nil {
		return 0, nil, err
	}
	runner, err := s.resolveRunner(ctx, task, ticket.TemplateId, prepared.automation, prepared.input)
	if err != nil {
		return 0, nil, s.taskError(task.ID, taskPreparationOperation, err)
	}
	prepared.input["ticket_id"] = task.TicketID
	prepared.input["process_inst_id"] = task.ProcessInstanceID
	return runner.ID, prepared.input, nil
}

func (s *taskService) resolvePreparation(ctx context.Context, task domain.Task,
	ticket domain.Ticket) (preparation, error) {
	input, err := s.assembleRuntimeArgs(ctx, ticket)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	instance, err := s.engine.GetInstanceByID(ctx, task.ProcessInstanceID)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	flow, err := s.workflows.FindInstanceFlow(ctx, ticket.WorkflowId, instance.ProcID, instance.ProcVersion)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	automation, err := s.workflows.GetAutomationProperty(toEasyWorkflow(flow), task.NodeID)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	return preparation{automation: automation, input: input}, nil
}

func (s *taskService) resolveRunner(ctx context.Context, task domain.Task, templateID int64,
	automation easyflow.AutomationProperty, input domain.TaskArgs) (etaskclient.Runner, error) {
	if templateID > 0 && s.dispatches != nil {
		runner, matched, err := s.resolveRunnerByDispatch(ctx, templateID, automation, input)
		if err != nil {
			return etaskclient.Runner{}, s.taskError(task.ID, taskPreparationOperation, err)
		}
		if matched {
			return runner, nil
		}
	}
	return s.runners.FindByCodebookAndTag(ctx, automation.CodebookId, automation.Tag)
}

func (s *taskService) resolveRunnerByDispatch(ctx context.Context, templateID int64,
	automation easyflow.AutomationProperty, input domain.TaskArgs) (etaskclient.Runner, bool, error) {
	dispatches, _, err := s.dispatches.ListByTemplateId(ctx, 0, 1000, templateID)
	if err != nil {
		return etaskclient.Runner{}, false, fmt.Errorf("查询自动派发规则失败: %w", err)
	}
	for _, dispatch := range dispatches {
		actual, exists := input[dispatch.Field]
		if !exists || dispatch.Field == "" || fmt.Sprint(actual) != dispatch.Value {
			continue
		}
		if dispatch.RunnerId <= 0 {
			return etaskclient.Runner{}, false, fmt.Errorf("匹配的自动派发规则缺少执行单元")
		}
		runner, findErr := s.runners.FindByID(ctx, dispatch.RunnerId)
		if findErr != nil {
			return etaskclient.Runner{}, false, fmt.Errorf("查询派发规则执行单元失败: %w", findErr)
		}
		if runner.CodebookID != automation.CodebookId {
			return etaskclient.Runner{}, false, fmt.Errorf("派发规则执行单元与自动化节点 Codebook 不匹配")
		}
		return runner, true, nil
	}
	return etaskclient.Runner{}, false, nil
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

func (s *taskService) calculateScheduledAt(automation easyflow.AutomationProperty,
	input domain.TaskArgs) (int64, error) {
	if !automation.IsTiming {
		return time.Now().UnixMilli(), nil
	}
	var quantity int64
	unit := uint8(2)
	switch automation.ExecMethod {
	case "template":
		if automation.TemplateField == "" {
			return 0, fmt.Errorf("动态定时配置缺少模版字段")
		}
		var err error
		quantity, err = parseQuantity(input[automation.TemplateField])
		if err != nil {
			return 0, fmt.Errorf("动态定时字段 %s 非法: %w", automation.TemplateField, err)
		}
	case "hand":
		quantity = automation.Quantity
		unit = automation.Unit
	default:
		return 0, fmt.Errorf("不支持的定时配置方式: %s", automation.ExecMethod)
	}
	if quantity <= 0 {
		return 0, fmt.Errorf("定时间隔必须大于 0")
	}
	duration := time.Duration(quantity) * time.Hour
	switch unit {
	case 1:
		duration = time.Duration(quantity) * time.Minute
	case 2:
	case 3:
		duration = time.Duration(quantity) * 24 * time.Hour
	default:
		return 0, fmt.Errorf("不支持的定时时间单位: %d", unit)
	}
	return time.Now().Add(duration).UnixMilli(), nil
}

func parseQuantity(value any) (int64, error) {
	switch current := value.(type) {
	case int:
		return int64(current), nil
	case int64:
		return current, nil
	case float64:
		if current != float64(int64(current)) {
			return 0, fmt.Errorf("必须是整数")
		}
		return int64(current), nil
	case string:
		parsed, err := strconv.ParseInt(current, 10, 64)
		if err == nil {
			return parsed, nil
		}
	}
	return 0, fmt.Errorf("必须是有效整数")
}

func toEasyWorkflow(workflow domain.Workflow) easyflow.Workflow {
	edges := make([]map[string]any, len(workflow.FlowData.Edges))
	for index, edge := range workflow.FlowData.Edges {
		edges[index] = map[string]any(edge)
	}
	nodes := make([]map[string]any, len(workflow.FlowData.Nodes))
	for index, node := range workflow.FlowData.Nodes {
		nodes[index] = map[string]any(node)
	}
	return easyflow.Workflow{
		Id: workflow.Id, Name: workflow.Name, Owner: workflow.Owner,
		FlowData: easyflow.LogicFlow{Edges: edges, Nodes: nodes},
	}
}
