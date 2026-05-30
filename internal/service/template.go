package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/pkg/hash"
	"github.com/xen0n/go-workwx"
	"golang.org/x/sync/errgroup"
)

// ITemplateCoreService 工单页面模板核心业务子接口
type ITemplateCoreService interface {
	// FindOrCreateByWechat 接收企微同步通知并拉取其 OA 审批模板详情，不存在则自动将其转换为本地模板存入，存在则完成增量校验更新
	FindOrCreateByWechat(ctx context.Context, req domain.WechatInfo) (domain.Template, error)
	// CreateTemplate 在当前租户空间下新建一个工单自定义模板，并返回生成的主键 ID
	CreateTemplate(ctx context.Context, req domain.Template) (int64, error)
	// DetailTemplate 获取指定主键 ID 的单个模板的详细渲染属性及表单配置数据
	DetailTemplate(ctx context.Context, id int64) (domain.Template, error)
	// FindByTemplateIds 根据一批给定的主键 ID 批量获取对应的模板领域模型列表
	FindByTemplateIds(ctx context.Context, ids []int64) ([]domain.Template, error)
	// DetailTemplateByExternalTemplateId 通过外部关联系统 ID (如飞书、企业微信的模板 UUID) 获取对应模板配置
	DetailTemplateByExternalTemplateId(ctx context.Context, externalId string) (domain.Template, error)
	// ListTemplate 分页获取所有可用工单模板列表，并返回包含所有模板的总记录数目
	ListTemplate(ctx context.Context, offset, limit int64) ([]domain.Template, int64, error)
	// DeleteTemplate 删除指定的工单模板实体，返回被成功删除的记录条数
	DeleteTemplate(ctx context.Context, id int64) (int64, error)
	// UpdateTemplate 覆盖更新替换已有的工单模板属性（如规则、选项等）
	UpdateTemplate(ctx context.Context, t domain.Template) (int64, error)
	// Pipeline 负责以组别结构的形式聚合返回不同组合分类的模板列表（已在仓储层内存优化），供工作流选单展示
	Pipeline(ctx context.Context) ([]domain.TemplateCombination, error)
	// GetByWorkflowId 获取关联了某个具体工作流流程定义 ID 的模板清单列表
	GetByWorkflowId(ctx context.Context, workflowId int64) ([]domain.Template, error)
	// FindByKeyword 输入过滤词对模板的标题或备注进行模糊查询，并分页反馈数据和符合条件的条数汇总
	FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Template, int64, error)
}

// ITemplateGroupService 模板分类分组业务子接口
type ITemplateGroupService interface {
	// CreateGroup 新建一个分类分组，返回生成的自增 ID
	CreateGroup(ctx context.Context, req domain.TemplateGroup) (int64, error)

	// ListGroup 分页获取模板的分类分组列表
	ListGroup(ctx context.Context, offset, limit int64) ([]domain.TemplateGroup, int64, error)

	// ListGroupsByIds 根据主键 ID 列表批量获取分类分组列表
	ListGroupsByIds(ctx context.Context, ids []int64) ([]domain.TemplateGroup, error)
}

// ITemplateFavoriteService 模板收藏业务子接口
type ITemplateFavoriteService interface {
	// ToggleFavorite 切换当前登录用户对指定模板的收藏状态（行级锁防重保护），并即时返回操作后最新的收藏布尔状态
	ToggleFavorite(ctx context.Context, userId int64, templateId int64) (bool, error)

	// ListFavoriteTemplates 呈现指定关联用户曾经收藏过并置于个人收藏夹当中的系列模板整体列表
	ListFavoriteTemplates(ctx context.Context, userId int64) ([]domain.Template, error)
}

// ITemplate 工单模板大组合业务服务接口 (通过接口隔离拆分，再经接口嵌入组合，兼顾高内聚与向后兼容)
type ITemplate interface {
	ITemplateCoreService
	ITemplateGroupService
	ITemplateFavoriteService
}

type templateService struct {
	repo    repository.ITemplateRepository
	workApp *workwx.WorkwxApp
}

// NewTemplateService 初始化模板业务服务
func NewTemplateService(repo repository.ITemplateRepository, workApp *workwx.WorkwxApp) ITemplate {
	return &templateService{
		repo:    repo,
		workApp: workApp,
	}
}

// --- Template 业务逻辑实现 ---

func (s *templateService) ToggleFavorite(ctx context.Context, userId int64, templateId int64) (bool, error) {
	return s.repo.ToggleFavorite(ctx, userId, templateId)
}

func (s *templateService) ListFavoriteTemplates(ctx context.Context, userId int64) ([]domain.Template, error) {
	return s.repo.ListFavoriteTemplates(ctx, userId)
}

func (s *templateService) GetByWorkflowId(ctx context.Context, workflowId int64) ([]domain.Template, error) {
	return s.repo.GetByWorkflowId(ctx, workflowId)
}

func (s *templateService) DetailTemplateByExternalTemplateId(ctx context.Context, externalId string) (domain.Template, error) {
	return s.repo.DetailTemplateByExternalTemplateId(ctx, externalId)
}

func (s *templateService) FindByTemplateIds(ctx context.Context, ids []int64) ([]domain.Template, error) {
	return s.repo.FindByTemplateIds(ctx, ids)
}

func (s *templateService) FindOrCreateByWechat(ctx context.Context, req domain.WechatInfo) (domain.Template, error) {
	OAInfo, err := s.workApp.GetOATemplateDetail(req.TemplateId)
	if err != nil {
		return domain.Template{}, fmt.Errorf("拉取企微 OA 模板详情失败: %w", err)
	}

	t, err := s.repo.FindByExternalTemplateId(ctx, req.TemplateId)
	if !errors.Is(err, repository.ErrTemplateNotFound) {
		if hash.Hash(OAInfo.TemplateContent) != hash.Hash(t.WechatOAControls) {
			// NOTE: 控制属性内容发生变更时，自动进行更新
			t.WechatOAControls = OAInfo.TemplateContent
			t.UniqueHash = hash.Hash(OAInfo.TemplateContent)
			_, updateErr := s.repo.UpdateTemplate(ctx, t)
			if updateErr != nil {
				return domain.Template{}, fmt.Errorf("更新企微 OA 模板失败: %w", updateErr)
			}
		}
		return t, err
	}

	t = domain.Template{
		CreateType:         domain.WechatCreate,
		Name:               req.TemplateName,
		ExternalTemplateId: req.TemplateId,
		WechatOAControls:   OAInfo.TemplateContent,
		UniqueHash:         hash.Hash(OAInfo.TemplateContent),
	}

	t.Id, err = s.repo.CreateTemplate(ctx, t)
	if err != nil {
		return domain.Template{}, err
	}

	return t, nil
}

func (s *templateService) CreateTemplate(ctx context.Context, req domain.Template) (int64, error) {
	return s.repo.CreateTemplate(ctx, req)
}

func (s *templateService) UpdateTemplate(ctx context.Context, t domain.Template) (int64, error) {
	return s.repo.UpdateTemplate(ctx, t)
}

func (s *templateService) DetailTemplate(ctx context.Context, id int64) (domain.Template, error) {
	return s.repo.DetailTemplate(ctx, id)
}

func (s *templateService) ListTemplate(ctx context.Context, offset, limit int64) ([]domain.Template, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Template
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListTemplate(ctx, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.Total(ctx)
		return err
	})

	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *templateService) DeleteTemplate(ctx context.Context, id int64) (int64, error) {
	return s.repo.DeleteTemplate(ctx, id)
}

func (s *templateService) Pipeline(ctx context.Context) ([]domain.TemplateCombination, error) {
	return s.repo.Pipeline(ctx)
}

func (s *templateService) FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Template, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Template
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.FindByKeyword(ctx, keyword, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountByKeyword(ctx, keyword)
		return err
	})

	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

// --- TemplateGroup 业务逻辑实现 ---

func (s *templateService) CreateGroup(ctx context.Context, req domain.TemplateGroup) (int64, error) {
	return s.repo.CreateGroup(ctx, req)
}

func (s *templateService) ListGroup(ctx context.Context, offset, limit int64) ([]domain.TemplateGroup, int64, error) {
	var (
		eg    errgroup.Group
		gs    []domain.TemplateGroup
		total int64
	)
	eg.Go(func() error {
		var err error
		gs, err = s.repo.ListGroup(ctx, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.TotalGroup(ctx)
		return err
	})

	if err := eg.Wait(); err != nil {
		return gs, total, err
	}
	return gs, total, nil
}

func (s *templateService) ListGroupsByIds(ctx context.Context, ids []int64) ([]domain.TemplateGroup, error) {
	return s.repo.ListGroupsByIds(ctx, ids)
}
