package hook

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eflow/pkg/flowx"
)

// RegisterAutomationWorker 注册 flowx 自动化任务执行器，将流程自动化节点无缝对接到任务编排服务。
func RegisterAutomationWorker(engine flowx.Engine, tasks taskSvc.Service) {
	engine.RegisterWorker("automation", func(ctx context.Context, tc *flowx.TaskContext) (*flowx.TaskResult, error) {
		ticketID := domain.NewID(tc.Instance.BusinessKey).Int64()
		instanceID := domain.NewID(tc.Instance.ID)
		_, err := tasks.CreateTask(ctx, ticketID, instanceID, tc.Node.ID, tc.Node.Name, tc.Instance.Version)
		return nil, err
	})
}
