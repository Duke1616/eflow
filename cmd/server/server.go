package server

import (
	"context"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Duke1616/eflow/ioc"
	"github.com/gotomicro/ego"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
)

// NewCommand 返回 server 子命令。
func NewCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "server",
		Short: "启动服务节点",
		Run: func(cmd *cobra.Command, args []string) {
			startServer()
		},
	}
}

func startServer() {
	app, err := ioc.InitApp()
	if err != nil {
		elog.Panic("init_app_failed", elog.FieldErr(err))
	}

	// 注册流程引擎驱动事件
	engine.RegisterEvents(app.Event)

	// 启动后台任务
	ctx := context.Background()
	app.StartBackgroundTasks(ctx)

	if err = ego.New().Serve(app.GetServers()...).Run(); err != nil {
		elog.Panic("app_run_error", elog.FieldErr(err))
	}
}
