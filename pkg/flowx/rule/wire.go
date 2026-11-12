package rule

import (
	"strings"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// FromGroup 把持久化 DTO 还原成条件树。
// 历史编译产物给每个原子条件都套了一层 Logic=AND 的空组，这里折叠掉，运行期不再背这层包装。
func FromGroup(g *model.ConditionGroup) Condition {
	if g == nil {
		return nil
	}

	children := make([]Condition, 0, len(g.Conditions)+len(g.SubGroups))
	for _, c := range g.Conditions {
		children = append(children, Compare{Field: c.Field, Op: c.Operator, Value: c.Value})
	}
	for i := range g.SubGroups {
		if sub := FromGroup(&g.SubGroups[i]); sub != nil {
			children = append(children, sub)
		}
	}

	switch len(children) {
	case 0:
		return nil
	case 1:
		return children[0]
	}
	if strings.EqualFold(g.Logic, "OR") {
		return Or(children)
	}
	return And(children)
}

// ToGroup 把条件树映射回持久化 DTO，落库 JSON 形状与历史保持一致，存量模板零迁移。
// 返回 nil 表示该树无法用 DTO 表达（含 Expr 节点），此时由 EdgeSpec.Condition 的原文承载。
func ToGroup(c Condition) *model.ConditionGroup {
	switch n := c.(type) {
	case Compare:
		return &model.ConditionGroup{
			Logic:      "AND",
			Conditions: []model.Condition{{Field: n.Field, Operator: n.Op, Value: n.Value}},
		}
	case And:
		return branchToGroup("AND", n)
	case Or:
		return branchToGroup("OR", n)
	default:
		return nil
	}
}

func branchToGroup(logic string, children []Condition) *model.ConditionGroup {
	g := &model.ConditionGroup{Logic: logic}
	for _, child := range children {
		if leaf, ok := child.(Compare); ok {
			g.Conditions = append(g.Conditions, model.Condition{
				Field:    leaf.Field,
				Operator: leaf.Op,
				Value:    leaf.Value,
			})
			continue
		}

		sub := ToGroup(child)
		if sub == nil {
			// 子树含 DTO 表达不了的节点，整体退回原文承载，绝不静默丢掉条件
			return nil
		}
		g.SubGroups = append(g.SubGroups, *sub)
	}
	return g
}
