package task

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/stretchr/testify/require"
)

func TestHandleExecuteResult(t *testing.T) {
	testCases := []struct {
		name       string
		event      event.ExecuteResultEvent
		wantErr    string
		wantCalls  int
		wantStatus domain.AttemptStatus
		wantReason string
	}{
		{name: "忽略其他执行来源", event: event.ExecuteResultEvent{Source: "TASK"}},
		{name: "工作流事件缺少请求标识", event: event.ExecuteResultEvent{
			Source: workflowExecutionSource, ExecID: 10}, wantErr: "缺少 request_id"},
		{name: "工作流事件缺少租户身份", event: event.ExecuteResultEvent{
			Source: workflowExecutionSource, RequestID: "eflow:1:1"}, wantErr: "缺少租户身份"},
		{name: "成功事件", event: event.ExecuteResultEvent{
			Source: workflowExecutionSource, RequestID: "eflow:1:1", ExecStatus: "SUCCESS",
			TaskResult: `{"result":"ok"}`}, wantCalls: 1, wantStatus: domain.AttemptStatusSuccess},
		{name: "失败事件", event: event.ExecuteResultEvent{
			Source: workflowExecutionSource, RequestID: "eflow:1:1", ExecStatus: "FAILED",
			TaskResult: "执行失败"}, wantCalls: 1, wantStatus: domain.AttemptStatusFailed,
			wantReason: "执行失败"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			svc := &taskServiceStub{}
			consumer := NewExecuteResultConsumer(nil, svc)
			ctx := context.Background()
			if testCase.wantErr != "缺少租户身份" {
				ctx = ctxutil.WithTenantID(ctx, 2)
			}
			err := consumer.handle(ctx, testCase.event)
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
			require.Equal(t, testCase.wantCalls, svc.calls)
			if testCase.wantCalls > 0 {
				require.Equal(t, testCase.wantStatus, svc.status)
				require.Equal(t, testCase.wantReason, svc.reason)
			}
		})
	}
}

type taskServiceStub struct {
	taskSvc.Service
	calls  int
	status domain.AttemptStatus
	reason string
}

func (s *taskServiceStub) CompleteAttempt(_ context.Context, _ string, status domain.AttemptStatus,
	_, reason string) (domain.TaskAttempt, error) {
	s.calls++
	s.status = status
	s.reason = reason
	return domain.TaskAttempt{}, nil
}
