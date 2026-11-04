package flowx

import (
	"fmt"
	"strings"

	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// StartNodeConverter 开始节点转换器
type StartNodeConverter struct{}

func (c *StartNodeConverter) Type() string { return NodeTypeStart }

func (c *StartNodeConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	prop, _ := DecodeProperty[easyflow.StartProperty](raw.Properties)
	name := DefaultNameStart
	if strings.TrimSpace(prop.Name) != "" {
		name = prop.Name
	}
	return model.NodeSpec{
		ID:   raw.ID,
		Name: name,
		Type: model.NodeStart,
	}, nil
}

// EndNodeConverter 结束办结节点转换器
type EndNodeConverter struct{}

func (c *EndNodeConverter) Type() string { return NodeTypeEnd }

func (c *EndNodeConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	prop, _ := DecodeProperty[easyflow.EndProperty](raw.Properties)
	name := DefaultNameEnd
	if strings.TrimSpace(prop.Name) != "" {
		name = prop.Name
	}
	return model.NodeSpec{
		ID:   raw.ID,
		Name: name,
		Type: model.NodeEnd,
	}, nil
}

// UserNodeConverter 人工审批与抄送节点转换器
type UserNodeConverter struct{}

func (c *UserNodeConverter) Type() string { return NodeTypeUser }

func (c *UserNodeConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	prop, err := DecodeProperty[easyflow.UserProperty](raw.Properties)
	if err != nil {
		return model.NodeSpec{}, fmt.Errorf("解析用户节点属性失败 [id=%s]: %w", raw.ID, err)
	}

	name := DefaultNameUser
	if strings.TrimSpace(prop.Name) != "" {
		name = prop.Name
	}

	// 1. 判断节点角色：抄送 (CC) vs 人工审批 (Approval)
	nodeType := model.NodeApproval
	if prop.IsCC {
		nodeType = model.NodeCarbonCopy
	}

	// 2. 审批规则仲裁：会签 (ALL_PASS) vs 或签 (ONE_VOTE)
	signRule := model.SignOneVote
	if prop.IsCosigned {
		signRule = model.SignAllPass
	}

	// 3. 规范化提取分配规则，并转换为标准 URN 标识符
	assignees, hasDynamicRules := c.resolveAssignees(prop)

	// 4. 组装节点规格
	spec := model.NodeSpec{
		ID:        raw.ID,
		Name:      name,
		Type:      nodeType,
		SignRule:  signRule,
		Assignees: assignees,
		Hooks:     make([]string, 0),
		Props:     make(map[string]any),
	}

	// 若存在动态算人规则（如 team / oncall / leader / department），自动挂载算人 Hook
	if hasDynamicRules {
		spec.Hooks = append(spec.Hooks, "dynamic_assignee_resolver")
	}

	return spec, nil
}

// resolveAssignees 将 LogicFlow 配置的多维度分配规则归一化为 URN 格式
func (c *UserNodeConverter) resolveAssignees(prop easyflow.UserProperty) ([]string, bool) {
	rules := prop.NormalizeAssignees()
	var (
		result          []string
		hasDynamicRules bool
	)

	for _, a := range rules {
		switch a.Rule {
		case easyflow.APPOINT:
			// 静态指定用户
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, trimmed)
				}
			}
		case easyflow.FOUNDER:
			// 发起人自身
			result = append(result, "leader:starter")
			hasDynamicRules = true
		case easyflow.LEADER, easyflow.MAIN_LEADER:
			// 直属领导或分管领导
			result = append(result, "leader:starter")
			hasDynamicRules = true
		case easyflow.TEAM:
			// 团队人员
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "team:"+trimmed)
					hasDynamicRules = true
				}
			}
		case easyflow.ON_CALL:
			// 值班人员
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "oncall:"+trimmed)
					hasDynamicRules = true
				}
			}
		case easyflow.DEPARTMENT:
			// 部门人员
			for _, val := range a.Values {
				if trimmed := strings.TrimSpace(val); trimmed != "" {
					result = append(result, "dept:"+trimmed)
					hasDynamicRules = true
				}
			}
		default:
			// 兜底直接使用 values
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

func (c *ParallelGatewayConverter) Type() string { return NodeTypeParallel }

func (c *ParallelGatewayConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   raw.ID,
		Name: DefaultNameParallel,
		Type: model.NodeGatewayParallel,
	}, nil
}

// InclusionGatewayConverter 包容网关转换器 (OR)
type InclusionGatewayConverter struct{}

func (c *InclusionGatewayConverter) Type() string { return NodeTypeInclusion }

func (c *InclusionGatewayConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   raw.ID,
		Name: DefaultNameInclusion,
		Type: model.NodeGatewayInclusive,
	}, nil
}

// SelectiveGatewayConverter 排他网关转换器 (XOR)
type SelectiveGatewayConverter struct{}

func (c *SelectiveGatewayConverter) Type() string { return NodeTypeSelective }

func (c *SelectiveGatewayConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   raw.ID,
		Name: DefaultNameSelective,
		Type: model.NodeGatewayExclusive,
	}, nil
}

// ConditionGatewayConverter 兼容旧版 condition 节点（映射为排他网关）
type ConditionGatewayConverter struct{}

func (c *ConditionGatewayConverter) Type() string { return NodeTypeCondition }

func (c *ConditionGatewayConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	return model.NodeSpec{
		ID:   raw.ID,
		Name: "条件网关",
		Type: model.NodeGatewayExclusive,
	}, nil
}

// AutomationNodeConverter 自动化作业任务转换器
type AutomationNodeConverter struct{}

func (c *AutomationNodeConverter) Type() string { return NodeTypeAuto }

func (c *AutomationNodeConverter) Convert(_ *ConvertContext, raw Node) (model.NodeSpec, error) {
	prop, _ := DecodeProperty[easyflow.AutomationProperty](raw.Properties)
	name := "自动化任务"
	if strings.TrimSpace(prop.Name) != "" {
		name = prop.Name
	}

	props := make(map[string]any)
	if prop.CodebookId > 0 {
		props["codebook_id"] = prop.CodebookId
	}
	if prop.RunnerID > 0 {
		props["runner_id"] = prop.RunnerID
	}

	return model.NodeSpec{
		ID:         raw.ID,
		Name:       name,
		Type:       model.NodeTask,
		WorkerType: "automation",
		Props:      props,
	}, nil
}
