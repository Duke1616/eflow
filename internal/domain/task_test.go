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

func TestCancelledTaskCanRetry(t *testing.T) {
	require.True(t, TaskStatusCancelled.CanRetry())
	require.True(t, TaskStatusRunning.CanTerminate())
	require.True(t, TaskStatusFailed.CanTerminate())
	require.False(t, TaskStatusSuccess.CanTerminate())
	require.False(t, TaskStatusCancelled.CanTerminate())
	require.True(t, AttemptStatusCancelled.IsTerminal())
}

func TestTask_RichDomainBehaviors(t *testing.T) {
	t.Run("NewProcessTask 工厂构造初始化", func(t *testing.T) {
		task := NewProcessTask(1, 2, NewID(3), "node-a", "节点A", 1)
		require.Equal(t, int64(1), task.TenantID)
		require.Equal(t, int64(2), task.TicketID)
		require.Equal(t, TaskStatusWaiting, task.Status)
		require.Equal(t, TaskExecutionProcess, task.ExecutionKind)
		require.True(t, task.IsWaiting())
		require.False(t, task.IsPendingSubmission())
	})

	t.Run("Block 与 Schedule 修改状态", func(t *testing.T) {
		task := NewProcessTask(1, 2, NewID(3), "node-a", "节点A", 1)
		task.Schedule(1000, "node-comp")
		require.Equal(t, int64(1000), task.ScheduledAt)
		require.Equal(t, "node-comp", task.CompensationNodeID)

		task.Block("配置缺失")
		require.Equal(t, TaskStatusBlocked, task.Status)
		require.Equal(t, "配置缺失", task.LastError)
	})

	t.Run("ValidateTerminable 终态与补偿约束", func(t *testing.T) {
		testCases := []struct {
			name    string
			task    Task
			wantErr error
		}{
			{name: "补偿任务不能终止", task: Task{ExecutionKind: TaskExecutionCompensation}, wantErr: ErrCompensationNotTerminable},
			{name: "成功任务不能终止", task: Task{Status: TaskStatusSuccess}, wantErr: ErrTaskAlreadySucceeded},
			{name: "已取消任务允许幂等终止", task: Task{Status: TaskStatusCancelled}, wantErr: nil},
			{name: "运行中任务允许终止", task: Task{Status: TaskStatusRunning}, wantErr: nil},
			{name: "提交中任务允许终止", task: Task{Status: TaskStatusSubmitting}, wantErr: nil},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.task.ValidateTerminable()
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})

	t.Run("ValidateRetry 重试规则校验", func(t *testing.T) {
		testCases := []struct {
			name      string
			task      Task
			automatic bool
			wantErr   error
		}{
			{name: "成功任务不可重试", task: Task{Status: TaskStatusSuccess}, wantErr: ErrTaskCannotRetry},
			{name: "已取消任务支持人工重试", task: Task{Status: TaskStatusCancelled}, automatic: false, wantErr: nil},
			{name: "已取消任务禁止自动重试", task: Task{Status: TaskStatusCancelled}, automatic: true, wantErr: ErrCancelledTaskRequiresManualRetry},
			{name: "失败任务支持重试", task: Task{Status: TaskStatusFailed}, automatic: true, wantErr: nil},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				err := tc.task.ValidateRetry(tc.automatic)
				if tc.wantErr != nil {
					require.ErrorIs(t, err, tc.wantErr)
				} else {
					require.NoError(t, err)
				}
			})
		}
	})
}
