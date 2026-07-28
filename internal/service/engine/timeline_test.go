package engine

import (
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestAttachTimelineMembers_GroupsByNodeAndBatch(t *testing.T) {
	groups := []domain.TaskTimelineGroup{
		{NodeID: "approve", BatchCode: "batch-1"},
		{NodeID: "approve", BatchCode: "batch-2"},
	}
	members := []model.Task{
		{TaskID: 1, NodeID: "approve", BatchCode: "batch-1", UserID: "alice"},
		{TaskID: 2, NodeID: "approve", BatchCode: "batch-1", UserID: "bob"},
		{TaskID: 3, NodeID: "approve", BatchCode: "batch-2", UserID: "carol"},
	}

	actual := attachTimelineMembers(groups, members)

	require.Len(t, actual, 2)
	require.Len(t, actual[0].Members, 2)
	require.Equal(t, []string{"alice", "bob"}, []string{actual[0].Members[0].UserID, actual[0].Members[1].UserID})
	require.Len(t, actual[1].Members, 1)
	require.Equal(t, "carol", actual[1].Members[0].UserID)
}
