package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskExecutionKindAllowsStart(t *testing.T) {
	testCases := []struct {
		name   string
		kind   TaskExecutionKind
		status Status
		want   bool
	}{
		{name: "流程中普通任务允许启动", kind: TaskExecutionProcess, status: PROCESS, want: true},
		{name: "撤回中禁止普通任务", kind: TaskExecutionProcess, status: WITHDRAWING},
		{name: "撤回中允许补偿任务", kind: TaskExecutionCompensation, status: WITHDRAWING, want: true},
		{name: "流程中不启动补偿任务", kind: TaskExecutionCompensation, status: PROCESS},
		{name: "已撤回不再启动任务", kind: TaskExecutionCompensation, status: WITHDRAW},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.want, testCase.kind.AllowsStart(testCase.status))
		})
	}
}

func TestCancelledTaskCannotRetry(t *testing.T) {
	require.False(t, TaskStatusCancelled.CanRetry())
}
