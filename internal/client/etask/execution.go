package etask

import (
	"context"
	"fmt"

	executorv1 "github.com/Duke1616/eflow/api/proto/gen/etask/executor/v1"
)

// ExecutionLog 是 eflow 展示所需的最小 etask 日志模型。
type ExecutionLog struct {
	ID      int64
	Time    int64
	Content string
}

// Execution 是 eflow 对账所需的最小 etask 执行视图。
type Execution struct {
	ID     int64
	Status string
	Result string
}

// ExecutionReader 定义 eflow 对 etask 执行详情的只读访问能力。
type ExecutionReader interface {
	// Find 根据 execution ID 获取持久化执行状态。
	Find(ctx context.Context, executionID int64) (Execution, error)
	// Logs 分页读取指定 execution 的日志。
	Logs(ctx context.Context, executionID, minID int64, limit int) ([]ExecutionLog, int64, error)
}

func (r *executionReader) Find(ctx context.Context, executionID int64) (Execution, error) {
	response, err := r.client.GetTaskExecution(ctx, &executorv1.GetTaskExecutionRequest{ExecutionId: executionID})
	if err != nil {
		return Execution{}, err
	}
	execution := response.GetExecution()
	if execution == nil || execution.GetId() <= 0 {
		return Execution{}, fmt.Errorf("etask 返回了非法执行记录")
	}
	return Execution{
		ID: execution.GetId(), Status: execution.GetStatus().String(), Result: execution.GetTaskResult(),
	}, nil
}

type executionReader struct {
	client executorv1.TaskExecutionServiceClient
}

// NewExecutionReader 创建 etask 执行查询适配器。
func NewExecutionReader(client *ETASKClient) ExecutionReader {
	return &executionReader{client: client.ExecutorClient}
}

func (r *executionReader) Logs(ctx context.Context, executionID, minID int64,
	limit int) ([]ExecutionLog, int64, error) {
	response, err := r.client.GetExecutionLogs(ctx, &executorv1.GetExecutionLogsRequest{
		ExecutionId: executionID, MinId: minID, Limit: int32(limit),
	})
	if err != nil {
		return nil, 0, err
	}
	logs := make([]ExecutionLog, 0, len(response.GetLogs()))
	for _, log := range response.GetLogs() {
		logs = append(logs, ExecutionLog{ID: log.GetId(), Time: log.GetTime(), Content: log.GetContent()})
	}
	return logs, response.GetMaxId(), nil
}
