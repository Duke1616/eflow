package ticketpbac

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/pbac"
	pbacgorm "github.com/Duke1616/eiam/pkg/pbac/gormx"
)

func TestTodoProfileBindsValues(t *testing.T) {
	decision := pbac.Decision{Allowed: true, FilterProfile: TodoProfile, AllowAccessScope: &pbac.AccessScope{Predicate: &pbac.Predicate{Key: TaskAssignee, Operator: pbac.StringEquals, Values: []pbac.Operand{pbac.Literal("alice"), pbac.Literal("bob' OR 1=1 --")}}}}
	expression, err := pbacgorm.Compile(decision, Todo)
	if err != nil {
		t.Fatal(err)
	}
	if expression.SQL != "(pt.user_id IN (?,?))" || !reflect.DeepEqual(expression.Args, []any{"alice", "bob' OR 1=1 --"}) {
		t.Fatalf("unexpected expression: %#v", expression)
	}
}

func TestHistoryRejectsSubstringOperatorForMembership(t *testing.T) {
	decision := pbac.Decision{Allowed: true, FilterProfile: HistoryProfile, AllowAccessScope: &pbac.AccessScope{Predicate: &pbac.Predicate{Key: TicketRelatedUsers, Operator: pbac.StringContains, Values: []pbac.Operand{pbac.Literal("alice")}}}}
	if _, err := pbacgorm.Compile(decision, History); err == nil {
		t.Fatal("expected ambiguous operator to be rejected")
	}
}

func TestAccessScopePresetsMatchProfiles(t *testing.T) {
	for _, preset := range HistoryPresets {
		if err := pbac.ValidateAccessScope(preset.Expression); err != nil {
			t.Fatalf("invalid history preset %q: %v", preset.Code, err)
		}
	}
	if len(TodoPresets) != 3 || TodoPresets[2].Code != "todo_assignee" {
		t.Fatalf("unexpected todo presets: %#v", TodoPresets)
	}
}

func TestHistoryProfile_DualEngine_RelatedUsers(t *testing.T) {
	decision := pbac.Decision{
		Allowed:       true,
		FilterProfile: HistoryProfile,
		AllowAccessScope: &pbac.AccessScope{
			Predicate: &pbac.Predicate{
				Key:      TicketRelatedUsers,
				Operator: pbac.ForAnyValueStringEquals,
				Values:   []pbac.Operand{pbac.Literal("alice")},
			},
		},
	}
	expression, err := pbacgorm.Compile(decision, History)
	if err != nil {
		t.Fatalf("编译 History 策略失败: %v", err)
	}

	expectedArgs := []any{"alice", "alice", "alice", "alice"}
	if !reflect.DeepEqual(expression.Args, expectedArgs) {
		t.Fatalf("SQL 参数绑定不匹配, 期望: %#v, 实际: %#v", expectedArgs, expression.Args)
	}

	// 验证包含 FlowX 与 EasyFlow 的双引擎支持
	expectedSQLKeywords := []string{
		"ticket.create_by = ?",
		"ticket.engine_type = 'flowx'",
		"SELECT 1 FROM flow_task pbac_ft WHERE pbac_ft.instance_id = ticket.process_instance_key AND pbac_ft.assignee = ?",
		"ticket.engine_type = 'easy-workflow'",
		"proc_task pbac_pt WHERE pbac_pt.proc_inst_id = ticket.process_instance_id AND pbac_pt.user_id = ?",
		"hist_proc_task pbac_ht WHERE pbac_ht.proc_inst_id = ticket.process_instance_id AND pbac_ht.user_id = ?",
	}
	for _, kw := range expectedSQLKeywords {
		if !strings.Contains(expression.SQL, kw) {
			t.Fatalf("SQL 未包含预期关键字: %s, 实际 SQL: %s", kw, expression.SQL)
		}
	}
}

