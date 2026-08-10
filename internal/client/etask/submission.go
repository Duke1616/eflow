package etask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	schedulerv1 "github.com/Duke1616/eflow/api/proto/gen/etask/scheduler/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrRejected 表示 etask 已明确拒绝请求，使用相同参数重试不会成功。
var ErrRejected = errors.New("etask 拒绝执行请求")

// TaskDispatcher 定义 eflow 向 etask 提交执行尝试的能力。
type TaskDispatcher interface {
	// Dispatch 幂等提交执行尝试并返回 etask execution ID。
	Dispatch(ctx context.Context, attempt domain.TaskAttempt) (int64, error)
	// TerminateExecution 按 execution ID 或 request ID 持久化终止意图。
	TerminateExecution(ctx context.Context, executionID int64, requestID, reason string) error
}

func (d *taskDispatcher) TerminateExecution(ctx context.Context, executionID int64,
	requestID, reason string) error {
	response, err := d.client.TerminateExecution(ctx, &schedulerv1.TerminateExecutionRequest{
		ExecutionId: executionID, RequestId: requestID, Reason: reason,
	})
	if err != nil {
		if code := status.Code(err); code == codes.InvalidArgument || code == codes.FailedPrecondition {
			return fmt.Errorf("%w: %v", ErrRejected, err)
		}
		return fmt.Errorf("终止 etask 工作流执行失败: %w", err)
	}
	if !response.GetTerminated() {
		return fmt.Errorf("etask 未确认取消意图已持久化")
	}
	return nil
}

type taskDispatcher struct {
	client schedulerv1.SchedulerServiceClient
}

// NewTaskDispatcher 创建 etask 执行提交适配器。
func NewTaskDispatcher(client *ETASKClient) TaskDispatcher {
	return &taskDispatcher{client: client.SchedulerClient}
}

func (d *taskDispatcher) Dispatch(ctx context.Context, attempt domain.TaskAttempt) (int64, error) {
	args, err := json.Marshal(attempt.Input)
	if err != nil {
		return 0, fmt.Errorf("序列化自动化任务输入失败: %w", err)
	}
	response, err := d.client.RunRunner(ctx, &schedulerv1.RunRunnerRequest{
		RequestId: attempt.RequestID,
		RunnerId:  attempt.RunnerID,
		Params:    map[string]string{"args": string(args)},
	})
	if err != nil {
		if code := status.Code(err); code == codes.InvalidArgument || code == codes.FailedPrecondition {
			return 0, fmt.Errorf("%w: %v", ErrRejected, err)
		}
		return 0, fmt.Errorf("提交 etask 工作流执行失败: %w", err)
	}
	if response.GetExecutionId() <= 0 {
		return 0, fmt.Errorf("etask 返回了非法 execution ID")
	}
	return response.GetExecutionId(), nil
}
