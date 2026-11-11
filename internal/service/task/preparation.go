package task

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/samber/lo"
)

const taskPreparationOperation = "准备"

type preparation struct {
	automation flowdsl.AutoTaskConfig
	input      domain.TaskArgs
}

type taskDefinition struct {
	scheduledAt        int64
	compensationNodeID string
}

func (s *taskService) prepareTaskDefinition(ctx context.Context, task domain.Task,
	ticket domain.Ticket) (taskDefinition, error) {
	prepared, err := s.resolvePreparation(ctx, task, ticket)
	if err != nil {
		return taskDefinition{compensationNodeID: prepared.automation.CompensationNodeID}, err
	}
	if err = s.applyTemplateScheduleOverride(ctx, task.NodeID, ticket.TemplateId, &prepared.automation); err != nil {
		return taskDefinition{compensationNodeID: prepared.automation.CompensationNodeID}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	scheduledAt, err := s.calculateScheduledAt(prepared.automation, prepared.input, ticket.TemplateId)
	if err != nil {
		return taskDefinition{compensationNodeID: prepared.automation.CompensationNodeID}, err
	}
	return taskDefinition{scheduledAt: scheduledAt, compensationNodeID: prepared.automation.CompensationNodeID}, nil
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
	prepared.input["process_inst_id"] = task.InstanceID.String()
	return decision, prepared.input, nil
}

func (s *taskService) resolvePreparation(ctx context.Context, task domain.Task,
	ticket domain.Ticket) (preparation, error) {
	flow, err := s.workflows.FindInstanceFlow(ctx, ticket.WorkflowId, 0, task.ProcessVersion, ticket.Process.ResolveEngineType())
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	graph, err := flowdsl.Parse(flow.FlowData)
	if err != nil {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	node, ok := graph.GetNode(task.NodeID)
	autoCfg, isAuto := node.AsAutoTask()
	if !ok || !isAuto {
		return preparation{}, s.taskError(task.ID, taskPreparationOperation, fmt.Errorf("自动化节点 %s 未配置", task.NodeID))
	}
	automation := *autoCfg
	input, err := s.assembleRuntimeArgs(ctx, ticket)
	if err != nil {
		return preparation{automation: automation}, s.taskError(task.ID, taskPreparationOperation, err)
	}
	return preparation{automation: automation, input: input}, nil
}

func (s *taskService) resolveRunner(ctx context.Context, templateID int64, nodeID string,
	automation flowdsl.AutoTaskConfig, input domain.TaskArgs) (domain.RunnerRouteDecision, error) {
	decision := domain.RunnerRouteDecision{
		DefaultRunnerID:     automation.RunnerID,
		MaxExecutionSeconds: automation.MaxExecutionSeconds,
	}
	if templateID > 0 {
		runner, ruleID, err := s.resolveRunnerByDispatch(ctx, templateID, nodeID, automation.CodebookID, input)
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
	defaultRunner, err := s.etask.FindByID(ctx, automation.RunnerID)
	if err != nil {
		return domain.RunnerRouteDecision{}, fmt.Errorf("查询默认执行单元失败: %w", err)
	}
	if defaultRunner.CodebookID != automation.CodebookID {
		return domain.RunnerRouteDecision{}, fmt.Errorf(
			"默认执行单元 %d 未绑定自动化节点脚本 %d", defaultRunner.ID, automation.CodebookID)
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
	matched, found := lo.Find(dispatches, func(d domain.Dispatch) bool {
		if d.Field == "" {
			return false
		}
		actual, exists := input[d.Field]
		return exists && fmt.Sprint(actual) == d.Value
	})
	if !found {
		return etaskclient.Runner{}, 0, nil
	}
	if matched.RunnerId <= 0 {
		return etaskclient.Runner{}, 0, fmt.Errorf("路由规则 %d 缺少有效执行单元", matched.Id)
	}
	runner, findErr := s.etask.FindByID(ctx, matched.RunnerId)
	if findErr != nil {
		return etaskclient.Runner{}, 0, fmt.Errorf("查询路由规则执行单元失败: %w", findErr)
	}
	if runner.CodebookID != codebookID {
		return etaskclient.Runner{}, 0, fmt.Errorf(
			"路由规则执行单元 %d 未绑定自动化节点脚本 %d", runner.ID, codebookID)
	}
	return runner, matched.Id, nil
}

// assembleRuntimeArgs 组装任务执行时的动态运行时参数。
//
// NOTE(架构兼容契约):
// user_info 包含工单发起人账号与上下文属性，下游存量自动化脚本深度依赖此参数进行权限绑定与身份反查。
// 需保持向后兼容，若远端用户服务异常，必须兜底提供包含基础账号标识的 JSON 结构，防止下游脚本解析中断。
func (s *taskService) assembleRuntimeArgs(ctx context.Context, ticket domain.Ticket) (domain.TaskArgs, error) {
	input := maps.Clone(ticket.Data)
	if input == nil {
		input = make(domain.TaskArgs)
	}
	forms, err := s.tickets.ListTaskFormsByTicketID(ctx, ticket.Id)
	if err != nil {
		return nil, err
	}
	for _, form := range forms {
		input[form.Key] = form.Value
	}

	if userInfo := s.resolveUserInfo(ctx, ticket.CreateBy); userInfo != "" {
		input["user_info"] = userInfo
	}

	return input, nil
}

// resolveUserInfo 解析并序列化发起人凭证信息，提供安全兜底以保障下游脚本的健壮性。
func (s *taskService) resolveUserInfo(ctx context.Context, username string) string {
	if username == "" {
		return ""
	}
	if s.users != nil {
		if user, err := s.users.FindByUsername(ctx, username); err == nil && user != nil {
			if data, marshalErr := json.Marshal(user); marshalErr == nil {
				return string(data)
			}
		}
	}
	// 查询失败或未提供客户端时，兜底生成基础结构保障下游脚本兼容
	fallback, _ := json.Marshal(map[string]any{
		"username":     username,
		"display_name": username,
	})
	return string(fallback)
}
