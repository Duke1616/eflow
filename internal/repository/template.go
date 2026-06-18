package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
	"github.com/xen0n/go-workwx"
	"gorm.io/gorm"
)

var (
	// ErrTemplateNotFound 未找到对应工单模板的仓储层 Sentinel 错误
	ErrTemplateNotFound = gorm.ErrRecordNotFound
	// ErrTemplateGroupNotEmpty 删除分组前发现分组内仍存在模板
	ErrTemplateGroupNotEmpty = errors.New("请先删除分组下的模板后再删除分组")
)

// ITemplateCoreRepository 工单模板核心防腐仓储子接口
type ITemplateCoreRepository interface {
	// CreateTemplate 创建模板，返回生成的自增主键 ID
	CreateTemplate(ctx context.Context, req domain.Template) (int64, error)
	// FindByHash 通过唯一内容摘要哈希值检索对应的工单模板信息
	FindByHash(ctx context.Context, hash string) (domain.Template, error)
	// FindByExternalTemplateId 通过外部绑定系统模板 ID 获取工单模板信息
	FindByExternalTemplateId(ctx context.Context, externalTemplateId string) (domain.Template, error)
	// DetailTemplate 获取对应主键 ID 的单个模板的详细属性与配置
	DetailTemplate(ctx context.Context, id int64) (domain.Template, error)
	// DeleteTemplate 根据 ID 物理删除模板，返回受影响的行数
	DeleteTemplate(ctx context.Context, id int64) (int64, error)
	// DetailTemplateByExternalTemplateId 通过外部模板 ID（如企微 OA 模板）获取模板的详情
	DetailTemplateByExternalTemplateId(ctx context.Context, externalId string) (domain.Template, error)
	// UpdateTemplate 覆盖更新当前模板相关的字段配置，返回受影响行数
	UpdateTemplate(ctx context.Context, req domain.Template) (int64, error)
	// ListTemplate 分页拉取工单模板列表，groupId 大于 0 时按分组过滤，keyword 非空时按名称或描述模糊搜索，按照时间逆序
	ListTemplate(ctx context.Context, groupId int64, keyword string, offset, limit int64) ([]domain.Template, error)
	// Total 统计当前租户空间下模板总数，groupId 大于 0 时按分组过滤，keyword 非空时按名称或描述模糊搜索
	Total(ctx context.Context, groupId int64, keyword string) (int64, error)
	// FindByTemplateIds 根据一批指定的主键 ID 列表批量拉取模板详情集合
	FindByTemplateIds(ctx context.Context, ids []int64) ([]domain.Template, error)
	// GetByWorkflowId 获取某个特定工作流流程关联的所有工单模板
	GetByWorkflowId(ctx context.Context, workflowId int64) ([]domain.Template, error)
}

// ITemplateGroupRepository 模板分类分组防腐仓储子接口
type ITemplateGroupRepository interface {
	// CreateGroup 新建一个分类分组，返回生成的自增 ID
	CreateGroup(ctx context.Context, req domain.TemplateGroup) (int64, error)

	// UpdateGroup 更新分类分组基本信息，返回受影响行数
	UpdateGroup(ctx context.Context, req domain.TemplateGroup) (int64, error)

	// DeleteGroup 删除分类分组，分组下存在模板时拒绝删除
	DeleteGroup(ctx context.Context, id int64) (int64, error)

	// ListGroup 分页获取模板的分类分组列表
	ListGroup(ctx context.Context, offset, limit int64) ([]domain.TemplateGroup, error)

	// TotalGroup 统计系统当前有效的模板分组总条数
	TotalGroup(ctx context.Context) (int64, error)

	// ListGroupSummaries 获取模板分组摘要及每组模板数量
	ListGroupSummaries(ctx context.Context) ([]domain.TemplateGroupSummary, error)
}

// ITemplateFavoriteRepository 模板收藏防腐仓储子接口
type ITemplateFavoriteRepository interface {
	// ToggleFavorite 切换当前用户对目标模板的收藏状态（加锁事务保护），并返回该模板操作后的最新收藏状态
	ToggleFavorite(ctx context.Context, userId int64, templateId int64) (bool, error)

	// ListFavoriteTemplates 拉取并呈现指定关联用户所有的收藏夹模板列表
	ListFavoriteTemplates(ctx context.Context, userId int64) ([]domain.Template, error)
}

// ITemplateRepository 工单模板防腐仓储大组合接口 (采用接口隔离原则拆分，再经由嵌入组合，兼具内聚与拓展特性)
type ITemplateRepository interface {
	ITemplateCoreRepository
	ITemplateGroupRepository
	ITemplateFavoriteRepository
}

type templateRepository struct {
	dao dao.ITemplateDAO
}

// NewTemplateRepository 初始化工单模板仓储层
func NewTemplateRepository(dao dao.ITemplateDAO) ITemplateRepository {
	return &templateRepository{
		dao: dao,
	}
}

// --- Template 仓储实现 ---

func (repo *templateRepository) CreateTemplate(ctx context.Context, req domain.Template) (int64, error) {
	return repo.dao.CreateTemplate(ctx, repo.toEntity(req))
}

func (repo *templateRepository) FindByHash(ctx context.Context, hash string) (domain.Template, error) {
	t, err := repo.dao.FindByHash(ctx, hash)
	if err != nil {
		return domain.Template{}, err
	}
	return repo.toDomain(t), nil
}

func (repo *templateRepository) FindByExternalTemplateId(ctx context.Context, externalTemplateId string) (domain.Template, error) {
	t, err := repo.dao.FindByExternalTemplateId(ctx, externalTemplateId)
	if err != nil {
		return domain.Template{}, err
	}
	return repo.toDomain(t), nil
}

func (repo *templateRepository) DetailTemplate(ctx context.Context, id int64) (domain.Template, error) {
	t, err := repo.dao.DetailTemplate(ctx, id)
	if err != nil {
		return domain.Template{}, err
	}
	return repo.toDomain(t), nil
}

func (repo *templateRepository) DeleteTemplate(ctx context.Context, id int64) (int64, error) {
	return repo.dao.DeleteTemplate(ctx, id)
}

func (repo *templateRepository) DetailTemplateByExternalTemplateId(ctx context.Context, externalId string) (domain.Template, error) {
	t, err := repo.dao.DetailTemplateByExternalTemplateId(ctx, externalId)
	if err != nil {
		return domain.Template{}, err
	}
	return repo.toDomain(t), nil
}

func (repo *templateRepository) UpdateTemplate(ctx context.Context, req domain.Template) (int64, error) {
	return repo.dao.UpdateTemplate(ctx, repo.toEntity(req))
}

func (repo *templateRepository) ListTemplate(ctx context.Context, groupId int64, keyword string, offset, limit int64) ([]domain.Template, error) {
	ts, err := repo.dao.ListTemplate(ctx, groupId, keyword, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src dao.Template) domain.Template {
		return repo.toDomain(src)
	}), nil
}

func (repo *templateRepository) Total(ctx context.Context, groupId int64, keyword string) (int64, error) {
	return repo.dao.Count(ctx, groupId, keyword)
}

func (repo *templateRepository) FindByTemplateIds(ctx context.Context, ids []int64) ([]domain.Template, error) {
	ts, err := repo.dao.FindByTemplateIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src dao.Template) domain.Template {
		return repo.toDomain(src)
	}), nil
}

func (repo *templateRepository) GetByWorkflowId(ctx context.Context, workflowId int64) ([]domain.Template, error) {
	ts, err := repo.dao.GetByWorkflowId(ctx, workflowId)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src dao.Template) domain.Template {
		return repo.toDomain(src)
	}), nil
}

func (repo *templateRepository) ToggleFavorite(ctx context.Context, userId int64, templateId int64) (bool, error) {
	return repo.dao.ToggleFavorite(ctx, userId, templateId)
}

func (repo *templateRepository) ListFavoriteTemplates(ctx context.Context, userId int64) ([]domain.Template, error) {
	ids, err := repo.dao.ListTemplateIdsByUserId(ctx, userId)
	if err != nil {
		return nil, err
	}
	return repo.FindByTemplateIds(ctx, ids)
}

// --- TemplateGroup 仓储实现 ---

func (repo *templateRepository) CreateGroup(ctx context.Context, req domain.TemplateGroup) (int64, error) {
	return repo.dao.CreateGroup(ctx, repo.toGroupEntity(req))
}

func (repo *templateRepository) UpdateGroup(ctx context.Context, req domain.TemplateGroup) (int64, error) {
	return repo.dao.UpdateGroup(ctx, repo.toGroupEntity(req))
}

func (repo *templateRepository) DeleteGroup(ctx context.Context, id int64) (int64, error) {
	affected, err := repo.dao.DeleteGroup(ctx, id)
	if errors.Is(err, dao.ErrTemplateGroupNotEmpty) {
		return affected, ErrTemplateGroupNotEmpty
	}
	return affected, err
}

func (repo *templateRepository) ListGroup(ctx context.Context, offset, limit int64) ([]domain.TemplateGroup, error) {
	gs, err := repo.dao.ListGroup(ctx, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(gs, func(idx int, src dao.TemplateGroup) domain.TemplateGroup {
		return repo.toGroupDomain(src)
	}), nil
}

func (repo *templateRepository) TotalGroup(ctx context.Context) (int64, error) {
	return repo.dao.CountGroup(ctx) // 使用解耦拆分后的 Group 专用 Count
}

func (repo *templateRepository) ListGroupSummaries(ctx context.Context) ([]domain.TemplateGroupSummary, error) {
	summaries, err := repo.dao.ListGroupSummaries(ctx)
	if err != nil {
		return nil, err
	}
	return slice.Map(summaries, func(idx int, src dao.TemplateGroupSummary) domain.TemplateGroupSummary {
		return domain.TemplateGroupSummary{
			Id:    src.Id,
			Name:  src.Name,
			Icon:  src.Icon,
			Total: src.Total,
		}
	}), nil
}

// --- 实体与领域防腐映射辅助转换 ---

func (repo *templateRepository) toEntity(req domain.Template) dao.Template {
	rules := slice.Map(req.Rules, func(idx int, src domain.Rule) dao.Rule {
		return dao.Rule(src)
	})

	return dao.Template{
		Id:                 req.Id,
		Name:               req.Name,
		WorkflowId:         req.WorkflowId,
		GroupId:            req.GroupId,
		Icon:               req.Icon,
		CreateType:         req.CreateType.ToUint8(),
		UniqueHash:         req.UniqueHash,
		ExternalTemplateId: req.ExternalTemplateId,
		Desc:               req.Desc,
		Rules:              sqlx.JsonField[[]dao.Rule]{Val: rules, Valid: true},
		Options:            sqlx.JsonField[dao.TemplateOptions]{Val: dao.TemplateOptions(req.Options), Valid: true},
		WechatOAControls:   sqlx.JsonField[workwx.OATemplateControls]{Val: req.WechatOAControls, Valid: true},
	}
}

func (repo *templateRepository) toDomain(req dao.Template) domain.Template {
	rules := slice.Map(req.Rules.Val, func(idx int, src dao.Rule) domain.Rule {
		return domain.Rule(src)
	})

	return domain.Template{
		Id:                 req.Id,
		Name:               req.Name,
		WorkflowId:         req.WorkflowId,
		GroupId:            req.GroupId,
		Icon:               req.Icon,
		CreateType:         domain.CreateType(req.CreateType),
		UniqueHash:         req.UniqueHash,
		ExternalTemplateId: req.ExternalTemplateId,
		Desc:               req.Desc,
		Rules:              rules,
		Options:            domain.TemplateOptions(req.Options.Val),
		WechatOAControls:   req.WechatOAControls.Val,
	}
}

func (repo *templateRepository) toGroupEntity(req domain.TemplateGroup) dao.TemplateGroup {
	return dao.TemplateGroup{
		Id:   req.Id,
		Name: req.Name,
		Icon: req.Icon,
	}
}

func (repo *templateRepository) toGroupDomain(req dao.TemplateGroup) domain.TemplateGroup {
	return domain.TemplateGroup{
		Id:   req.Id,
		Name: req.Name,
		Icon: req.Icon,
	}
}
