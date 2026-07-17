package migrations

import (
	"strings"
	"sync"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm/schema"
)

func TestLegacyTaskSchemaSupportsJSONArgs(t *testing.T) {
	parsed, err := schema.Parse(&legacyTask{}, &sync.Map{}, schema.NamingStrategy{})

	require.NoError(t, err)
	require.Equal(t, "task", parsed.Table)
	require.Equal(t, "json", string(parsed.FieldsByName["Args"].DataType))
}

func TestConvertLegacyTaskCreatesSafeHistoricalAttempt(t *testing.T) {
	source := legacyTask{
		ID: 10, TicketID: 20, ProcessInstID: 30, CurrentNodeID: "automation-1",
		Status: 3, Result: "旧执行被中断",
		Args:      sqlx.JsonField[domain.TaskArgs]{Val: domain.TaskArgs{"env": "prod"}, Valid: true},
		StartTime: 100, UTime: 200,
	}

	task, attempt := convertLegacyTask(source, 2)

	require.Equal(t, uint8(domain.TaskStatusBlocked), task.Status)
	require.Equal(t, "自动化任务", task.NodeName)
	require.Zero(t, task.ProcessVersion)
	require.Equal(t, "legacy:2:10", attempt.RequestID)
	require.Zero(t, attempt.RunnerID)
	require.False(t, attempt.ExecutionID.Valid)
	require.Equal(t, "旧执行被中断", attempt.Output)
}

func TestMapLegacyTaskState(t *testing.T) {
	testCases := []struct {
		name         string
		source       legacyTask
		wantStatus   domain.TaskStatus
		wantPhase    domain.TaskPhase
		wantAdvanced bool
		wantReason   string
	}{
		{name: "成功任务保持成功并禁止重复推进", source: legacyTask{
			Status: legacyTaskStatusSuccess, EndTime: 100,
		}, wantStatus: domain.TaskStatusSuccess, wantPhase: domain.TaskPhaseSucceeded, wantAdvanced: true},
		{name: "失败任务安全阻塞", source: legacyTask{
			Status: 2, Result: "执行失败",
		}, wantStatus: domain.TaskStatusBlocked, wantPhase: domain.TaskPhaseBlocked, wantReason: "执行失败"},
		{name: "运行中任务不自动续跑", source: legacyTask{
			Status: 3, TriggerPosition: "执行中断",
		}, wantStatus: domain.TaskStatusBlocked, wantPhase: domain.TaskPhaseBlocked, wantReason: "执行中断"},
		{name: "未知状态安全阻塞", source: legacyTask{
			Status: 99,
		}, wantStatus: domain.TaskStatusBlocked, wantPhase: domain.TaskPhaseBlocked, wantReason: "旧任务没有保存错误详情"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			status, phase, advancedAt, reason := mapLegacyTaskState(testCase.source)
			require.Equal(t, testCase.wantStatus, status)
			require.Equal(t, testCase.wantPhase, phase)
			require.Equal(t, testCase.wantAdvanced, advancedAt > 0)
			if testCase.wantReason == "" {
				require.Empty(t, reason)
			} else {
				require.Contains(t, reason, testCase.wantReason)
			}
		})
	}
}

func TestValidateLegacyTaskIdentity(t *testing.T) {
	testCases := []struct {
		name string
		task legacyTask
		want string
	}{
		{name: "身份完整", task: legacyTask{TicketID: 1, ProcessInstID: 2, CurrentNodeID: "node-1"}},
		{name: "缺少工单", task: legacyTask{ProcessInstID: 2, CurrentNodeID: "node-1"}, want: "缺少工单"},
		{name: "缺少多项", task: legacyTask{}, want: "缺少工单、缺少流程实例、缺少节点 ID"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			require.Equal(t, testCase.want, validateLegacyTaskIdentity(testCase.task))
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	value := strings.Repeat("中", 10)
	require.Equal(t, strings.Repeat("中", 4), truncateRunes(value, 4))
}
