package ticketpbac

import (
	"fmt"
	"strings"

	"github.com/Duke1616/eiam/pkg/pbac"
	pbacgorm "github.com/Duke1616/eiam/pkg/pbac/gormx"
)

// 工单查询支持的 PBAC profile 与业务属性键。
const (
	HistoryProfile     pbac.FilterProfile = "ticket_history.v1"
	TodoProfile        pbac.FilterProfile = "ticket_todo.v1"
	TicketCreateBy     pbac.AttributeKey  = "ticket:create_by"
	TicketRelatedUsers pbac.AttributeKey  = "ticket:related_users"
	TaskAssignee       pbac.AttributeKey  = "task:assignee"
)

var (
	// History 将历史工单 AccessScope 编译为 ticket 表及任务关联查询。
	History = pbacgorm.Profile{
		Name:             HistoryProfile,
		CompilePredicate: compileHistory,
	}
	// Todo 将全部待办 AccessScope 编译为当前任务及其工单关联查询。
	Todo = pbacgorm.Profile{
		Name:             TodoProfile,
		CompilePredicate: compileTodo,
	}
	// HistoryPresets 是历史列表、详情和流转记录共同支持的访问范围模板。
	HistoryPresets = []pbac.AccessScopePreset{creatorPreset(), relatedPreset()}
	// TodoPresets 是全部待办查询支持的访问范围模板。
	TodoPresets = []pbac.AccessScopePreset{creatorPreset(), relatedPreset(), assigneePreset()}
)

func creatorPreset() pbac.AccessScopePreset {
	return pbac.AccessScopePreset{
		Code:        "ticket_creator",
		Name:        "仅本人创建",
		Description: "只允许访问由当前登录用户创建的工单",
		Expression: &pbac.AccessScope{Predicate: &pbac.Predicate{
			Key: TicketCreateBy, Operator: pbac.StringEquals,
			Values: []pbac.Operand{pbac.Ref(pbac.PrincipalUsername)},
		}},
	}
}

func relatedPreset() pbac.AccessScopePreset {
	return pbac.AccessScopePreset{
		Code:        "ticket_related",
		Name:        "本人创建或参与",
		Description: "允许访问本人创建、当前处理或历史参与的工单",
		Expression: &pbac.AccessScope{Predicate: &pbac.Predicate{
			Key: TicketRelatedUsers, Operator: pbac.ForAnyValueStringEquals,
			Values: []pbac.Operand{pbac.Ref(pbac.PrincipalUsername)},
		}},
	}
}

func assigneePreset() pbac.AccessScopePreset {
	return pbac.AccessScopePreset{
		Code:        "todo_assignee",
		Name:        "仅本人待办",
		Description: "只允许访问当前处理人为登录用户的待办",
		Expression: &pbac.AccessScope{Predicate: &pbac.Predicate{
			Key: TaskAssignee, Operator: pbac.StringEquals,
			Values: []pbac.Operand{pbac.Ref(pbac.PrincipalUsername)},
		}},
	}
}

const (
	historyRelatedUsersSQL = "(ticket.create_by = ? OR " +
		"EXISTS (SELECT 1 FROM proc_task pbac_pt WHERE pbac_pt.proc_inst_id = ticket.process_instance_id AND pbac_pt.user_id = ?) OR " +
		"EXISTS (SELECT 1 FROM hist_proc_task pbac_ht WHERE pbac_ht.proc_inst_id = ticket.process_instance_id AND pbac_ht.user_id = ?))"
	todoRelatedUsersSQL = "(t.create_by = ? OR " +
		"EXISTS (SELECT 1 FROM proc_task pbac_pt WHERE pbac_pt.proc_inst_id = t.process_instance_id AND pbac_pt.user_id = ?) OR " +
		"EXISTS (SELECT 1 FROM hist_proc_task pbac_ht WHERE pbac_ht.proc_inst_id = t.process_instance_id AND pbac_ht.user_id = ?))"
)

func compileHistory(predicate *pbac.Predicate) (pbacgorm.Expression, error) {
	values, err := pbacgorm.LiteralStrings(predicate.Values)
	if err != nil {
		return pbacgorm.Expression{}, err
	}
	switch predicate.Key {
	case TicketCreateBy:
		return stringColumn("ticket.create_by", predicate.Operator, values)
	case TicketRelatedUsers:
		return membership(predicate.Operator, values, historyRelatedUsersSQL)
	default:
		return pbacgorm.Expression{}, fmt.Errorf("access scope key %q is not allowed on %s", predicate.Key, HistoryProfile)
	}
}

func compileTodo(predicate *pbac.Predicate) (pbacgorm.Expression, error) {
	values, err := pbacgorm.LiteralStrings(predicate.Values)
	if err != nil {
		return pbacgorm.Expression{}, err
	}
	switch predicate.Key {
	case TicketCreateBy:
		return stringColumn("t.create_by", predicate.Operator, values)
	case TaskAssignee:
		return stringColumn("pt.user_id", predicate.Operator, values)
	case TicketRelatedUsers:
		return membership(predicate.Operator, values, todoRelatedUsersSQL)
	default:
		return pbacgorm.Expression{}, fmt.Errorf("access scope key %q is not allowed on %s", predicate.Key, TodoProfile)
	}
}

func stringColumn(column string, operator pbac.Operator, values []any) (pbacgorm.Expression, error) {
	if operator != pbac.StringEquals && operator != pbac.StringNotEquals {
		return pbacgorm.Expression{}, fmt.Errorf("operator %q is not allowed for scalar field", operator)
	}
	keyword := "IN"
	if operator == pbac.StringNotEquals {
		keyword = "NOT IN"
	}
	return pbacgorm.Expression{
		SQL:  fmt.Sprintf("%s %s (%s)", column, keyword, pbacgorm.Placeholders(len(values))),
		Args: values,
	}, nil
}

func membership(operator pbac.Operator, values []any, fragment string) (pbacgorm.Expression, error) {
	if operator != pbac.ForAnyValueStringEquals {
		return pbacgorm.Expression{}, fmt.Errorf("operator %q is not allowed for membership", operator)
	}
	clauses := make([]string, 0, len(values))
	args := make([]any, 0, len(values)*3)
	for _, value := range values {
		clauses = append(clauses, fragment)
		args = append(args, value, value, value)
	}
	return pbacgorm.Expression{
		SQL:  strings.Join(clauses, " OR "),
		Args: args,
	}, nil
}
