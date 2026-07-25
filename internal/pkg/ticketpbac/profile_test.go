package ticketpbac

import (
	"reflect"
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
