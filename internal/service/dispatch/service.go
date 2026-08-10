package dispatch

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"golang.org/x/sync/errgroup"
)

// Service 管理工单模板的执行单元条件路由。
type Service interface {
	// Create 新建执行单元路由规则。
	Create(ctx context.Context, req domain.Dispatch) (int64, error)
	// Update 修改指定的执行单元路由规则。
	Update(ctx context.Context, req domain.Dispatch) (int64, error)
	// Delete 删除指定的执行单元路由规则。
	Delete(ctx context.Context, id int64) (int64, error)
	// ListByTemplateId 分页获取模板的路由规则与总数。
	ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, int64, error)
	// ListByTemplateNode 获取运行时参与匹配的节点路由规则。
	ListByTemplateNode(ctx context.Context, templateID int64, nodeID string) ([]domain.Dispatch, error)
	// ReplaceFromTemplate 用来源模板的规则完整替换目标模板规则。
	ReplaceFromTemplate(ctx context.Context, targetTemplateID, sourceTemplateID int64) (int64, error)
}

type TemplateReader interface {
	DetailTemplate(ctx context.Context, id int64) (domain.Template, error)
}

type service struct {
	repo      repository.DispatchRepository
	templates TemplateReader
}

func (s *service) ReplaceFromTemplate(ctx context.Context, targetTemplateID,
	sourceTemplateID int64) (int64, error) {
	if targetTemplateID <= 0 || sourceTemplateID <= 0 {
		return 0, fmt.Errorf("来源模板和目标模板必须有效")
	}
	if targetTemplateID == sourceTemplateID {
		return 0, fmt.Errorf("不能从当前模板复制路由规则")
	}
	target, err := s.templates.DetailTemplate(ctx, targetTemplateID)
	if err != nil {
		return 0, fmt.Errorf("查询目标模板失败: %w", err)
	}
	source, err := s.templates.DetailTemplate(ctx, sourceTemplateID)
	if err != nil {
		return 0, fmt.Errorf("查询来源模板失败: %w", err)
	}
	if target.WorkflowId <= 0 || target.WorkflowId != source.WorkflowId {
		return 0, fmt.Errorf("只能复制同一工作流下的执行单元路由规则")
	}
	rules, err := s.repo.ListAllByTemplateID(ctx, sourceTemplateID)
	if err != nil {
		return 0, err
	}
	conditions := make(map[string]struct{}, len(rules))
	for i := range rules {
		if err = normalizeAndValidate(&rules[i], false); err != nil {
			return 0, fmt.Errorf("来源模板包含无效路由规则 %d: %w", rules[i].Id, err)
		}
		key := rules[i].AutomationNodeID + "\x00" + rules[i].Field + "\x00" + rules[i].Value
		if _, exists := conditions[key]; exists {
			return 0, fmt.Errorf("来源模板的自动化节点 %s 包含重复匹配条件", rules[i].AutomationNodeID)
		}
		conditions[key] = struct{}{}
	}
	return s.repo.ReplaceByTemplate(ctx, targetTemplateID, rules)
}

// Delete 删除指定路由规则。
func (s *service) Delete(ctx context.Context, id int64) (int64, error) {
	return s.repo.Delete(ctx, id)
}

// Create 创建路由规则。
func (s *service) Create(ctx context.Context, req domain.Dispatch) (int64, error) {
	if err := normalizeAndValidate(&req, true); err != nil {
		return 0, err
	}
	if err := s.ensureConditionAvailable(ctx, req); err != nil {
		return 0, err
	}
	return s.repo.Create(ctx, req)
}

// Update 修改路由规则。
func (s *service) Update(ctx context.Context, req domain.Dispatch) (int64, error) {
	if req.Id <= 0 {
		return 0, fmt.Errorf("路由规则 ID 非法")
	}
	if err := normalizeAndValidate(&req, true); err != nil {
		return 0, err
	}
	if err := s.ensureConditionAvailable(ctx, req); err != nil {
		return 0, err
	}
	return s.repo.Update(ctx, req)
}

func (s *service) ensureConditionAvailable(ctx context.Context, req domain.Dispatch) error {
	exists, err := s.repo.ExistsCondition(ctx, req.Id, req.TemplateId,
		req.AutomationNodeID, req.Field, req.Value)
	if err != nil {
		return fmt.Errorf("检查路由规则冲突失败: %w", err)
	}
	if exists {
		return fmt.Errorf("当前自动化节点已经存在相同匹配条件")
	}
	return nil
}

func normalizeAndValidate(req *domain.Dispatch, requireTemplate bool) error {
	req.AutomationNodeID = strings.TrimSpace(req.AutomationNodeID)
	req.Field = strings.TrimSpace(req.Field)
	req.Value = strings.TrimSpace(req.Value)
	if requireTemplate && req.TemplateId <= 0 {
		return fmt.Errorf("路由规则必须关联有效工单模板")
	}
	if err := validateRunnerID(req.RunnerId); err != nil {
		return err
	}
	if err := validateAutomationNodeID(req.AutomationNodeID); err != nil {
		return err
	}
	if req.Field == "" {
		return fmt.Errorf("路由规则必须选择匹配字段")
	}
	if req.Value == "" {
		return fmt.Errorf("路由规则必须填写匹配值")
	}
	if len([]rune(req.Value)) > 255 {
		return fmt.Errorf("路由规则匹配值不能超过 255 个字符")
	}
	if req.Priority <= 0 {
		req.Priority = domain.DefaultDispatchPriority
	}
	return nil
}

func validateAutomationNodeID(nodeID string) error {
	if nodeID == "" {
		return fmt.Errorf("路由规则必须关联自动化节点")
	}
	return nil
}

func validateRunnerID(runnerID int64) error {
	if runnerID <= 0 {
		return fmt.Errorf("路由规则必须选择有效的执行单元")
	}
	return nil
}

// ListByTemplateId 并发获取路由规则和总数。
func (s *service) ListByTemplateId(ctx context.Context, offset, limit int64, templateId int64) ([]domain.Dispatch, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Dispatch
		total int64
	)
	eg.Go(func() error {
		var err error
		ts, err = s.repo.ListByTemplateId(ctx, offset, limit, templateId)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountByTemplateId(ctx, templateId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func (s *service) ListByTemplateNode(ctx context.Context, templateID int64,
	nodeID string) ([]domain.Dispatch, error) {
	if templateID <= 0 {
		return nil, fmt.Errorf("工单模板 ID 非法")
	}
	if err := validateAutomationNodeID(nodeID); err != nil {
		return nil, err
	}
	return s.repo.ListByTemplateNode(ctx, templateID, nodeID)
}

// NewService 初始化执行单元路由服务。
func NewTemplateReader(repo repository.ITemplateRepository) TemplateReader { return repo }

func NewService(repo repository.DispatchRepository, templates TemplateReader) Service {
	return &service{
		repo: repo, templates: templates,
	}
}
