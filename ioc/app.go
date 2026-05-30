package ioc

import (
	"context"

	endpointv1 "github.com/Duke1616/ecmdb/api/proto/gen/ecmdb/endpoint/v1"
	"github.com/gotomicro/ego/server"
	"github.com/gotomicro/ego/server/egin"
)

// Task 后台长任务接口 —— 各种补偿任务、消费者等
type Task interface {
	Start(ctx context.Context)
}

// App 模块化容器
type App struct {
	Web         *egin.Component
	EndpointSvc endpointv1.EndpointServiceClient
	Tasks       []Task
}

// GetServers 获取所有需要启动的服务列表
func (a *App) GetServers() []server.Server {
	var res []server.Server
	if a.Web != nil {
		res = append(res, a.Web)
	}
	return res
}

// StartBackgroundTasks 启动所有后台任务
func (a *App) StartBackgroundTasks(ctx context.Context) {
	for _, t := range a.Tasks {
		go func(t Task) {
			t.Start(ctx)
		}(t)
	}
}
