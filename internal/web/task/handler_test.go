package task

import (
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
)

func TestMapTasksExposesCompensationSemantics(t *testing.T) {
	tasks := mapTasks([]domain.Task{
		{ID: 1, ExecutionKind: domain.TaskExecutionProcess},
		{ID: 2, ExecutionKind: domain.TaskExecutionCompensation},
	})

	require.False(t, tasks[0].IsCompensation)
	require.True(t, tasks[1].IsCompensation)
}
