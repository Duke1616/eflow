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
	var (
		assignees       []string
		hasDynamicRules bool
	)
	if userCfg != nil {
		assignees, hasDynamicRules = c.resolveAssignees(userCfg.Assignees)
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

	if hasDynamicRules {
		spec.Hooks = append(spec.Hooks, "dynamic_assignee_resolver")
	}

	return spec, nil
}

// resolveAssignees 将标准 Assignee 分配规则归一化为 URN 标识符
func (c *UserNodeConverter) resolveAssignees(assignees []flowdsl.Assignee) ([]string, bool) {
	var (
		result          []string
		hasDynamicRules bool
	)

	for _, a := range assignees {
		switch a.Rule {
		case flowdsl.APPOINT:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, trimmed)
				}
			}
		case flowdsl.FOUNDER, flowdsl.LEADER, flowdsl.MainLeader:
			result = append(result, "leader:starter")
			hasDynamicRules = true
		case flowdsl.TEAM:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "team:"+trimmed)
					hasDynamicRules = true
				}
			}
		case flowdsl.OnCall:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "oncall:"+trimmed)
					hasDynamicRules = true
				}
			}
		case flowdsl.DEPARTMENT:
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "dept:"+trimmed)
					hasDynamicRules = true
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

	return lo.Uniq(result), hasDynamicRules
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

// SelectiveGatewayConverter 排他网关转换器 (XOR)
type SelectiveGatewayConverter struct{}

func (c *SelectiveGatewayConverter) Type() flowdsl.NodeType { return flowdsl.NodeGatewayExclusive }

func (c *SelectiveGatewayConverter) Convert(_ *ConvertContext, node *flowdsl.Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   node.ID,
		Name: node.DisplayName(),
		Type: model.NodeGatewayExclusive,
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
	}

	return model.NodeSpec{
		ID:         node.ID,
		Name:       name,
		Type:       model.NodeTask,
		WorkerType: "automation",
		Props:      props,
	}, nil
}
