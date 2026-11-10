package flowx

import (
	"strings"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// StartNodeConverter 开始节点转换器
type StartNodeConverter struct{}

func (c *StartNodeConverter) Type() flowdsl.NodeType { return flowdsl.NodeStart }

func (c *StartNodeConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   node.ID,
		Name: node.DisplayName(),
		Type: model.NodeStart,
	}, nil
}

// EndNodeConverter 结束办结节点转换器
type EndNodeConverter struct{}

func (c *EndNodeConverter) Type() flowdsl.NodeType { return flowdsl.NodeEnd }

func (c *EndNodeConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   node.ID,
		Name: node.DisplayName(),
		Type: model.NodeEnd,
	}, nil
}

// UserNodeConverter 人工审批与抄送节点转换器
type UserNodeConverter struct{}

func (c *UserNodeConverter) Type() flowdsl.NodeType { return flowdsl.NodeUserTask }

func (c *UserNodeConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	userCfg, _ := node.AsUserTask()

	isCC := userCfg != nil && userCfg.IsCC
	isCosigned := userCfg != nil && userCfg.IsCosigned
	var assignees []string
	if userCfg != nil {
		assignees = c.resolveAssignees(userCfg.Assignees)
	}

	nodeType := lo.Ternary(isCC, model.NodeCarbonCopy, model.NodeApproval)
	signRule := lo.Ternary(isCosigned, model.SignAllPass, model.SignOneVote)

	spec := model.NodeSpec{
		ID:        node.ID,
		Name:      node.DisplayName(),
		Type:      nodeType,
		SignRule:  signRule,
		Assignees: assignees,
		Hooks:     make([]string, 0),
		Props:     make(map[string]any),
	}

	return spec, nil
}

// resolveAssignees 将标准 Assignee 分配规则归一化为审批人列表
func (c *UserNodeConverter) resolveAssignees(assignees []flowdsl.Assignee) []string {
	var result []string

	for _, a := range assignees {
		switch a.Rule {
		case flowdsl.APPOINT:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, trimmed)
				}
			}
		case flowdsl.FOUNDER:
			// 内置上下文占位符：由引擎内核调度时直接就地展开为 inst.Starter，无需注入额外 Hook
			result = append(result, model.AssigneeStarter)
		case flowdsl.LEADER, flowdsl.MainLeader:
			result = append(result, "leader:starter")
		case flowdsl.TEAM:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "team:"+trimmed)
				}
			}
		case flowdsl.OnCall:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "oncall:"+trimmed)
				}
			}
		case flowdsl.DEPARTMENT:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "dept:"+trimmed)
				}
			}
		default:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, trimmed)
				}
			}
		}
	}

	return lo.Uniq(result)
}

// ParallelGatewayConverter 并行网关转换器 (AND)
type ParallelGatewayConverter struct{}

func (c *ParallelGatewayConverter) Type() flowdsl.NodeType { return flowdsl.NodeGatewayParallel }

func (c *ParallelGatewayConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   node.ID,
		Name: node.DisplayName(),
		Type: model.NodeGatewayParallel,
	}, nil
}

// InclusionGatewayConverter 包容网关转换器 (OR)
type InclusionGatewayConverter struct{}

func (c *InclusionGatewayConverter) Type() flowdsl.NodeType { return flowdsl.NodeGatewayInclusive }

func (c *InclusionGatewayConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   node.ID,
		Name: node.DisplayName(),
		Type: model.NodeGatewayInclusive,
	}, nil
}

// SelectiveGatewayConverter 排他/条件网关转换器
// NOTE: 当作为多路汇聚点（入边 > 1）时，编译为 FlowX 的 NodeGatewayExclusive（排他汇聚网关），
// 启用首到即放行的直通策略，实现分支间的“或（竞争汇聚）”；
// 当作为分流点（出边 > 1 且入边 <= 1）时，映射为 NodeGatewayInclusive（包容网关），
// 以便满足条件的多路分支可并发激活，未命中条件的分支自动标记为 SkippedTargets 以便汇聚。
type SelectiveGatewayConverter struct{}

func (c *SelectiveGatewayConverter) Type() flowdsl.NodeType { return flowdsl.NodeGatewayExclusive }

func (c *SelectiveGatewayConverter) Convert(ctx *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	gwType := model.NodeGatewayInclusive
	if ctx != nil && ctx.Graph != nil && len(ctx.Graph.GetInEdges(node.ID)) > 1 {
		gwType = model.NodeGatewayExclusive
	}

	return model.NodeSpec{
		ID:   node.ID,
		Name: node.DisplayName(),
		Type: gwType,
	}, nil
}

// ConditionGatewayConverter 兼容 LogicFlow 画布中的条件网关 (condition)
type ConditionGatewayConverter struct{}

func (c *ConditionGatewayConverter) Type() flowdsl.NodeType { return flowdsl.NodeGatewayCondition }

func (c *ConditionGatewayConverter) Convert(ctx *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	name := lo.Ternary(strings.TrimSpace(node.Name) != "", node.Name, "条件分支")
	gwType := model.NodeGatewayInclusive
	if ctx != nil && ctx.Graph != nil && len(ctx.Graph.GetInEdges(node.ID)) > 1 {
		gwType = model.NodeGatewayExclusive
	}

	return model.NodeSpec{
		ID:   node.ID,
		Name: name,
		Type: gwType,
	}, nil
}

// AutomationNodeConverter 自动化作业任务转换器
type AutomationNodeConverter struct{}

func (c *AutomationNodeConverter) Type() flowdsl.NodeType { return flowdsl.NodeAutoTask }

func (c *AutomationNodeConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	name := lo.Ternary(strings.TrimSpace(node.Name) != "", node.Name, "自动化任务")

	props := make(map[string]any)
	if autoCfg, ok := node.AsAutoTask(); ok {
		if autoCfg.CodebookID > 0 {
			props["codebook_id"] = autoCfg.CodebookID
		}
		if autoCfg.RunnerID > 0 {
			props["runner_id"] = autoCfg.RunnerID
		}
		if autoCfg.MaxExecutionSeconds > 0 {
			props["max_execution_seconds"] = autoCfg.MaxExecutionSeconds
		}
	}

	return model.NodeSpec{
		ID:         node.ID,
		Name:       name,
		Type:       model.NodeTask,
		WorkerType: "automation",
		Props:      props,
	}, nil
}

// ChatGroupNodeConverter IM 群聊广播通知节点转换器
type ChatGroupNodeConverter struct{}

func (c *ChatGroupNodeConverter) Type() flowdsl.NodeType { return flowdsl.NodeChatGroup }

func (c *ChatGroupNodeConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	name := lo.Ternary(strings.TrimSpace(node.Name) != "", node.Name, "群通知")

	props := make(map[string]any)
	if chatCfg, ok := node.AsChatGroup(); ok {
		if chatCfg.Mode != "" {
			props["mode"] = chatCfg.Mode
		}
		if len(chatCfg.ChatGroupIDs) > 0 {
			props["chat_group_ids"] = chatCfg.ChatGroupIDs
		}
		if len(chatCfg.OutputMode) > 0 {
			props["output_mode"] = chatCfg.OutputMode
		}
	}

	return model.NodeSpec{
		ID:    node.ID,
		Name:  name,
		Type:  model.NodeChatGroup,
		Props: props,
	}, nil
}
