package etask

import (
	"context"
	"errors"
	"testing"

	schedulerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/scheduler/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestDispatchClassifiesSubmissionError(t *testing.T) {
	testCases := []struct {
		name         string
		response     *schedulerv1.RunRunnerResponse
		err          error
		wantRejected bool
		wantID       int64
	}{
		{name: "提交成功", response: &schedulerv1.RunRunnerResponse{ExecutionId: 99}, wantID: 99},
		{name: "参数被拒绝", err: status.Error(codes.InvalidArgument, "runner disabled"), wantRejected: true},
		{name: "前置条件不满足", err: status.Error(codes.FailedPrecondition, "runner disabled"), wantRejected: true},
		{name: "服务内部故障", err: status.Error(codes.Internal, "database unavailable")},
		{name: "网络结果不确定", err: status.Error(codes.Unavailable, "connection lost")},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			client := &ETASKClient{SchedulerClient: &schedulerClientStub{
				response: testCase.response, err: testCase.err,
			}}
			dispatcher := NewTaskDispatcher(client)
			executionID, err := dispatcher.Dispatch(context.Background(), domain.TaskAttempt{
				RequestID: "eflow:1:1", RunnerID: 10, Input: domain.TaskArgs{"ticket_id": 1},
			})

			require.Equal(t, testCase.wantID, executionID)
			require.Equal(t, testCase.wantRejected, errors.Is(err, ErrRejected))
			if testCase.err == nil {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type schedulerClientStub struct {
	response *schedulerv1.RunRunnerResponse
	err      error
}

func (s *schedulerClientStub) RunRunner(context.Context, *schedulerv1.RunRunnerRequest,
	...grpc.CallOption) (*schedulerv1.RunRunnerResponse, error) {
	return s.response, s.err
}
