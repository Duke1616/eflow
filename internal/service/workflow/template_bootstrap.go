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
func NewTemplateBootstrapTask(workflowSvc Service, templateClient templatev1.TemplateServiceClient) *TemplateBootstrapTask {
	syncer := NewTemplateSyncer(workflowSvc, templateClient)
	return &TemplateBootstrapTask{
		syncer: syncer,
	}
}

// Start 启动后台模板自愈检查（微延时异步执行，不阻塞应用主启动流程）
func (t *TemplateBootstrapTask) Start(ctx context.Context) {
	go func() {
		// 延迟 3 秒，等底层网络与存储连接完全准备就绪
		time.Sleep(3 * time.Second)

		// 注入系统根租户 ID (母体租户 = 1)，供底层的多租户 GORM 插件和 gRPC 拦截器使用
		systemCtx := ctxutil.WithTenantID(ctx, ctxutil.SystemTenantID)

		if err := t.syncer.SyncAll(systemCtx); err != nil {
			elog.Error("运行工单消息通知模板自愈任务失败", elog.FieldErr(err))
		}
	}()
}
