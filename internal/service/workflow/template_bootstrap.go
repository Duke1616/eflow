package workflow

import (
	"context"
	"time"

	templatev1 "github.com/Duke1616/eflow/api/proto/gen/ealert/template/v1"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/gotomicro/ego/core/elog"
)

// TemplateBootstrapTask 全局工单消息通知模板同步自愈后台调度任务
type TemplateBootstrapTask struct {
	syncer ITemplateSyncer
}

// NewTemplateBootstrapTask 构建并统一组装工作流模板自愈任务实例
func NewTemplateBootstrapTask(templateClient templatev1.TemplateServiceClient) *TemplateBootstrapTask {
	syncer := NewTemplateSyncer(templateClient)
	return &TemplateBootstrapTask{
		syncer: syncer,
	}
}

// Start 启动后台模板自愈检查
func (t *TemplateBootstrapTask) Start(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				elog.DefaultLogger.Error("后台模板自愈巡检任务发生 panic", elog.Any("recover", r))
			}
		}()
		// 初始延迟 3 秒，等底层网络与存储连接完全准备就绪
		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}

		systemCtx := ctxutil.WithTenantID(ctx, ctxutil.SystemTenantID)

		// 指数退避重试策略：若 ealert 未启动，以 5s 起始退避重试；首次成功后每 30 分钟定期巡检
		backoff := 5 * time.Second
		const maxBackoff = 5 * time.Minute
		const regularInterval = 30 * time.Minute

		synced := false
		for {
			if ctx.Err() != nil {
				return
			}
			if err := t.syncer.SyncAll(systemCtx); err != nil {
				elog.Warn("工单消息通知模板自愈同步未完成，稍后重试",
					elog.FieldErr(err), elog.Duration("next_retry", backoff))
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
					backoff = min(backoff*2, maxBackoff)
				}
			} else {
				if !synced {
					elog.Info("工单消息通知模板首次全量自愈同步成功")
					synced = true
				}
				backoff = 5 * time.Second // 重置退避
				select {
				case <-ctx.Done():
					return
				case <-time.After(regularInterval):
				}
			}
		}
	}()
}
