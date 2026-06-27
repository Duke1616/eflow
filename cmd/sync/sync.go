package sync

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/ioc"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/cobra"
)

// NewCommand 返回 sync 父命令。
func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "数据同步与自愈命令",
	}

	cmd.AddCommand(NewTemplateCommand())
	return cmd
}

// NewTemplateCommand 返回 sync template 子命令。
func NewTemplateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "template",
		Short: "同步全局工单消息通知模板",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSyncTemplate(cmd.Context())
		},
	}
}

// runSyncTemplate 执行具体同步逻辑，初始化容器并触发同步器
func runSyncTemplate(ctx context.Context) error {
	elog.Info("正在通过命令行手动执行工单消息通知模板同步任务")

	// 1. 使用轻量级 Wire 注入器初始化模板同步器（避开 Web、DB、Kafka 等重型资源）
	syncer, err := ioc.InitTemplateSyncer()
	if err != nil {
		return fmt.Errorf("初始化模板同步服务失败: %w", err)
	}

	// 2. 注入系统根租户 ID (母体租户 = 1)，以便多租户底层正常校验
	systemCtx := ctxutil.WithTenantID(ctx, ctxutil.SystemTenantID)

	// 3. 执行全量同步自愈
	if err = syncer.SyncAll(systemCtx); err != nil {
		elog.Error("同步工单消息通知模板失败", elog.FieldErr(err))
		return err
	}

	elog.Info("同步工单消息通知模板成功")
	return nil
}
