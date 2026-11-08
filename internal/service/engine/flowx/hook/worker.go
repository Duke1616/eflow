package hook

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	taskSvc "github.com/Duke1616/eflow/internal/service/task"
	"github.com/Duke1616/eflow/pkg/flowx"
)

// RegisterAutomationWorker 注册 flowx 自动化任务执行器，将流程自动化节点无缝对接到任务编排服务。
func RegisterAutomationWorker(engine flowx.Engine, tasks taskSvc.Service) {
	engine.RegisterWorker("automation", func(ctx context.Context, tc *flowx.TaskContext) (*flowx.TaskResult, error) {
		// 遵循架构规范，解除与前端 HTTP 短生命周期 Context 的绑定，严防前端 5s 超时或网络断开导致底层流转中断
		taskCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()

		ticketID := domain.NewID(tc.Instance.BusinessKey).Int64()
		instanceID := domain.NewID(tc.Instance.ID)
		_, err := tasks.CreateTask(taskCtx, ticketID, instanceID, tc.Node.ID, tc.Node.Name, tc.Instance.Version)
		return nil, err
	})
}
