package ticketpbac

import (
	"reflect"
	"strings"
	"testing"

	"github.com/Duke1616/eiam/pkg/pbac"
	pbacgorm "github.com/Duke1616/eiam/pkg/pbac/gormx"
)

func TestTodoProfileBindsValues(t *testing.T) {
	injection := "bob' OR 1=1 --"
	testCases := []struct {
		name      string
		predicate *pbac.Predicate
		wantSQL   string
		wantArgs  []any
	}{
		{
			name:      "处理人等值多值按值展开片段且注入载荷只作为参数绑定",
			predicate: &pbac.Predicate{Key: TaskAssignee, Operator: pbac.StringEquals, Values: []pbac.Operand{pbac.Literal("alice"), pbac.Literal(injection)}},
			wantSQL:   todoAssigneeSQL + " OR " + todoAssigneeSQL,
			wantArgs:  []any{"alice", "alice", injection, injection},
		},
		{
			name:      "处理人不等值整体取反表达所有值均未命中待办",
			predicate: &pbac.Predicate{Key: TaskAssignee, Operator: pbac.StringNotEquals, Values: []pbac.Operand{pbac.Literal("alice")}},
			wantSQL:   "NOT (" + todoAssigneeSQL + ")",
			wantArgs:  []any{"alice", "alice"},
		},
		{
			name:      "相关人集合算子单值绑定片段内全部四个占位符",
			predicate: &pbac.Predicate{Key: TicketRelatedUsers, Operator: pbac.ForAnyValueStringEquals, Values: []pbac.Operand{pbac.Literal("alice")}},
			wantSQL:   todoRelatedUsersSQL,
			wantArgs:  []any{"alice", "alice", "alice", "alice"},
		},
		{
			name:      "提单人标量等值编译为 IN 参数绑定",
			predicate: &pbac.Predicate{Key: TicketCreateBy, Operator: pbac.StringEquals, Values: []pbac.Operand{pbac.Literal("alice")}},
			wantSQL:   "t.create_by IN (?)",
			wantArgs:  []any{"alice"},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			decision := pbac.Decision{Allowed: true, FilterProfile: TodoProfile, AllowAccessScope: &pbac.AccessScope{Predicate: tc.predicate}}
			expression, err := pbacgorm.Compile(decision, Todo)
			if err != nil {
				t.Fatal(err)
			}
			// Compile 会在编译产物外统一包一层括号，断言时对齐该形态
			if expression.SQL != "("+tc.wantSQL+")" || !reflect.DeepEqual(expression.Args, tc.wantArgs) {
				t.Fatalf("unexpected expression: %#v", expression)
			}
		})
	}
}

func TestTodoProfile_DualEngine_Assignee(t *testing.T) {
	decision := pbac.Decision{
		Allowed:       true,
		FilterProfile: TodoProfile,
		AllowAccessScope: &pbac.AccessScope{Predicate: &pbac.Predicate{
			Key: TaskAssignee, Operator: pbac.StringEquals, Values: []pbac.Operand{pbac.Literal("alice")},
		}},
	}
	expression, err := pbacgorm.Compile(decision, Todo)
	if err != nil {
		t.Fatalf("编译 Todo 策略失败: %v", err)
	}

	// 双引擎待办判定必须各带未办结谓词，防止把已办任务误判为当前待办
	expectedSQLKeywords := []string{
		"t.engine_type = 'flowx'",
		"SELECT 1 FROM flow_task pbac_ft WHERE pbac_ft.instance_id = t.process_instance_key AND pbac_ft.assignee = ? AND pbac_ft.status = 0",
		"t.engine_type = 'easy-workflow'",
		"proc_task pbac_pt WHERE pbac_pt.proc_inst_id = t.process_instance_id AND pbac_pt.user_id = ? AND pbac_pt.is_finished = 0",
	}
	for _, kw := range expectedSQLKeywords {
		if !strings.Contains(expression.SQL, kw) {
			t.Fatalf("SQL 未包含预期关键字: %s, 实际 SQL: %s", kw, expression.SQL)
		}
	}
}

func TestTodoProfile_RejectsAmbiguousOperators(t *testing.T) {
	testCases := []struct {
		name      string
		predicate *pbac.Predicate
	}{
		{
			name:      "处理人拒绝子串算子避免模糊匹配扩大可见范围",
			predicate: &pbac.Predicate{Key: TaskAssignee, Operator: pbac.StringContains, Values: []pbac.Operand{pbac.Literal("alice")}},
		},
		{
			name:      "相关人拒绝标量等值算子避免多值语义歧义",
			predicate: &pbac.Predicate{Key: TicketRelatedUsers, Operator: pbac.StringEquals, Values: []pbac.Operand{pbac.Literal("alice")}},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			decision := pbac.Decision{Allowed: true, FilterProfile: TodoProfile, AllowAccessScope: &pbac.AccessScope{Predicate: tc.predicate}}
			if _, err := pbacgorm.Compile(decision, Todo); err == nil {
				t.Fatal("expected ambiguous operator to be rejected")
			}
		})
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
