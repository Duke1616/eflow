package repository

import (
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestTaskSnapshotMapping(t *testing.T) {
	testCases := []struct {
		name string
		task domain.Task
	}{
		{
			name: "保留自动化节点历史快照",
			task: domain.Task{
				ID: 1, TenantID: 2, TicketID: 3, ProcessInstanceID: 4,
				NodeID: "automation-node", NodeName: "部署生产环境", ProcessVersion: 7,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			actual := toTaskDomain(toTaskEntity(testCase.task))

			require.Equal(t, testCase.task.NodeID, actual.NodeID)
			require.Equal(t, testCase.task.NodeName, actual.NodeName)
			require.Equal(t, testCase.task.ProcessVersion, actual.ProcessVersion)
		})
	}
}
