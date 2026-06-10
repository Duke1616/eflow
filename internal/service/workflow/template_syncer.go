package workflow

import (
	"context"
	"fmt"

	templatev1 "github.com/Duke1616/eflow/api/proto/gen/ealert/template/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
)

// ITemplateSyncer 定义了默认消息模板自愈同步器的行为契约
type ITemplateSyncer interface {
	// SyncAll 执行所有系统默认模板的自愈同步
	SyncAll(ctx context.Context) error
}

// templateSyncer 模板同步器的核心实现结构体
type templateSyncer struct {
	workflowSvc    Service
	templateClient templatev1.TemplateServiceClient
}

// NewTemplateSyncer 构建一个新的模板同步器实例
func NewTemplateSyncer(workflowSvc Service, templateClient templatev1.TemplateServiceClient) ITemplateSyncer {
	return &templateSyncer{
		workflowSvc:    workflowSvc,
		templateClient: templateClient,
	}
}

// SyncAll 执行批量自愈校验
func (s *templateSyncer) SyncAll(ctx context.Context) error {
	elog.Info("系统默认通知模板自愈任务启动")

	lo.ForEach(templates, func(cfg templateConfig, _ int) {
		if err := s.syncTemplate(ctx, cfg); err != nil {
			elog.Warn("同步默认通知模板失败", elog.String("name", cfg.Name), elog.FieldErr(err))
		}
	})

	elog.Info("系统默认通知模板自愈任务执行完毕")
	return nil
}

// syncTemplate 执行单个通知模板的防重幂等自愈与绑定关系确认
func (s *templateSyncer) syncTemplate(ctx context.Context, cfg templateConfig) error {
	// 1. 获取本地已存在的绑定规则 (0 表示默认全局绑定)
	binding, err := s.workflowSvc.AdminNotifyBinding().GetEffective(ctx, 0, cfg.NotifyType, cfg.Channel.String())
	if err != nil {
		return fmt.Errorf("获取通知模板绑定失败: %w", err)
	}

	// 2. 确保模板实体正常、内容最新并存在全局 Scope 属性
	templateId, err := s.ensureTemplate(ctx, binding, cfg)
	if err != nil {
		return err
	}

	// 3. 确保本地通知绑定映射关系处于正确状态
	return s.ensureBinding(ctx, binding, cfg, templateId)
}

// ensureTemplate 校验模板存在，并负责对老版本 Scope 属性以及最新内容的变更进行自愈升级
func (s *templateSyncer) ensureTemplate(ctx context.Context, binding domain.NotifyBinding, cfg templateConfig) (int64, error) {
	// 模板从未初始化，开始全新创建
	if binding.TemplateId == 0 {
		return s.createAndPublish(ctx, cfg)
	}

	// 模板已存在，读取远程服务中的详情
	tmpl, err := s.getTemplate(ctx, binding.TemplateId)
	if err != nil {
		return 0, err
	}

	// 兼容老版本：如果存量模板的 Scope 属性缺失或不是 GLOBAL，自动修复升级
	if err = s.upgradeScopeIfNeeded(ctx, tmpl); err != nil {
		return 0, err
	}

	// 升级版本：如果模板内容发生更新，自动追加并激活新版本
	if err = s.upgradeVersionIfNeeded(ctx, tmpl, cfg); err != nil {
		return 0, err
	}

	return tmpl.Id, nil
}

// ensureBinding 保证本地存储的通知绑定映射关系存在且正确指向目标模板
func (s *templateSyncer) ensureBinding(ctx context.Context, binding domain.NotifyBinding, cfg templateConfig, templateId int64) error {
	// 若之前无绑定记录，进行全新创建
	if binding.Id == 0 {
		return s.createBinding(ctx, cfg.NotifyType, cfg.Channel.String(), templateId)
	}

	// 如果绑定的模板 ID 不一致，执行映射更新（自愈的边界防御设计）
	if binding.TemplateId != templateId {
		binding.TemplateId = templateId
		if _, err := s.workflowSvc.AdminNotifyBinding().Update(ctx, binding); err != nil {
			return fmt.Errorf("更新本地通知绑定失败: %w", err)
		}
		elog.Info("本地通知绑定关系自愈更新成功", elog.String("name", cfg.Name), elog.Int64("template_id", templateId))
	}

	return nil
}

// getTemplate 获取远程模板详情
func (s *templateSyncer) getTemplate(ctx context.Context, templateId int64) (*templatev1.ChannelTemplate, error) {
	resp, err := s.templateClient.GetTemplateByID(ctx, &templatev1.GetTemplateByIDRequest{Id: templateId})
	if err != nil {
		return nil, fmt.Errorf("拉取远程模板详情失败: %w", err)
	}
	if resp.Template == nil {
		return nil, fmt.Errorf("远程模板服务返回空模板数据 (id: %d)", templateId)
	}
	return resp.Template, nil
}

// upgradeScopeIfNeeded 在老版本模板没有 Scope 属性时进行兼容性修复升级
func (s *templateSyncer) upgradeScopeIfNeeded(ctx context.Context, tmpl *templatev1.ChannelTemplate) error {
	if tmpl.Scope == templatev1.Scope_GLOBAL {
		return nil
	}

	tmpl.Scope = templatev1.Scope_GLOBAL
	_, err := s.templateClient.UpdateTemplate(ctx, &templatev1.UpdateTemplateRequest{
		Template: tmpl,
	})
	if err != nil {
		return fmt.Errorf("修改远程模板作用域为 GLOBAL 失败: %w", err)
	}

	elog.Info("升级存量模板作用域至全局GLOBAL成功", elog.String("name", tmpl.Name), elog.Int64("template_id", tmpl.Id))
	return nil
}

// upgradeVersionIfNeeded 比对当前活跃模板内容，若有更改则升级新版本并发布
func (s *templateSyncer) upgradeVersionIfNeeded(ctx context.Context, tmpl *templatev1.ChannelTemplate, cfg templateConfig) error {
	activeVersion, ok := lo.Find(tmpl.Versions, func(v *templatev1.ChannelTemplateVersion) bool {
		return v.Id == tmpl.ActiveVersionId
	})

	// 若未找到活跃版本，或者活跃版本内容与最新配置不一致，说明需要发版进行自愈
	if !ok || activeVersion.Content != cfg.Content {
		// 内容变更，追加新版本
		versionId, err := s.createVersion(ctx, tmpl.Id, cfg)
		if err != nil {
			return err
		}

		// 激活新版本
		if err = s.publish(ctx, tmpl.Id, versionId); err != nil {
			return err
		}

		elog.Info("升级默认模板版本成功", elog.String("name", cfg.Name), elog.Int64("template_id", tmpl.Id), elog.Int64("version_id", versionId))
		return nil
	}

	elog.Debug("模板内容无变更，跳过升级", elog.String("name", cfg.Name), elog.Int64("template_id", tmpl.Id))
	return nil
}

// createAndPublish 从零创建默认模板并激活发布
func (s *templateSyncer) createAndPublish(ctx context.Context, cfg templateConfig) (int64, error) {
	createResp, err := s.templateClient.CreateTemplate(ctx, &templatev1.CreateTemplateRequest{
		Template: &templatev1.ChannelTemplate{
			Name:        cfg.Name,
			Description: cfg.Desc,
			Channel:     cfg.Channel,
			Scope:       templatev1.Scope_GLOBAL,
			Versions: []*templatev1.ChannelTemplateVersion{
				{
					Name:    cfg.VersionName,
					Content: cfg.Content,
					Desc:    "系统自愈初始化版本",
				},
			},
		},
	})
	if err != nil {
		return 0, fmt.Errorf("创建远程模板请求失败: %w", err)
	}

	tmpl := createResp.Template
	if tmpl == nil || len(tmpl.Versions) == 0 {
		return 0, fmt.Errorf("创建远程模板返回无效元数据")
	}

	templateId := tmpl.Id
	versionId := tmpl.Versions[0].Id

	if err = s.publish(ctx, templateId, versionId); err != nil {
		return 0, err
	}

	return templateId, nil
}

// createVersion 在已有模板上新建版本
func (s *templateSyncer) createVersion(ctx context.Context, templateId int64, cfg templateConfig) (int64, error) {
	resp, err := s.templateClient.CreateTemplateVersion(ctx, &templatev1.CreateTemplateVersionRequest{
		TemplateId: templateId,
		Name:       cfg.VersionName,
		Content:    cfg.Content,
		Desc:       "自愈升级更新版本",
	})
	if err != nil {
		return 0, fmt.Errorf("创建模板版本快照失败: %w", err)
	}
	if resp.Version == nil {
		return 0, fmt.Errorf("创建模板版本快照返回无效元数据")
	}
	return resp.Version.Id, nil
}

// publish 发布指定的模板版本为活跃版本
func (s *templateSyncer) publish(ctx context.Context, templateId, versionId int64) error {
	_, err := s.templateClient.PublishTemplate(ctx, &templatev1.PublishTemplateRequest{
		TemplateId: templateId,
		VersionId:  versionId,
	})
	if err != nil {
		return fmt.Errorf("发布活跃版本 %d 失败: %w", versionId, err)
	}
	return nil
}

// createBinding 本地写入新绑定规则记录
func (s *templateSyncer) createBinding(ctx context.Context, notifyType domain.NotifyType, channel string, templateId int64) error {
	_, err := s.workflowSvc.AdminNotifyBinding().Create(ctx, domain.NotifyBinding{
		WorkflowId: 0,
		NotifyType: notifyType,
		Channel:    channel,
		TemplateId: templateId,
	})
	if err != nil {
		return fmt.Errorf("本地数据库写入绑定映射关系失败: %w", err)
	}
	return nil
}
