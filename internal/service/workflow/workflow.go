package workflow

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
	"golang.org/x/sync/errgroup"
)

// IWorkflowCoreService 工作流核心定义业务服务子接口
type IWorkflowCoreService interface {
	// Create 新建流程定义，返回生成的自增 ID
	Create(ctx context.Context, req domain.Workflow) (int64, error)
	// List 分页拉取工单流程模板列表，同时返回当前租户总条数以供分页计算
	List(ctx context.Context, offset, limit int64) ([]domain.Workflow, int64, error)
	// Find 精确获取指定主键 ID 的工作流详情，承载最新的画布设计与元数据属性
	Find(ctx context.Context, id int64) (domain.Workflow, error)
	// FindByIds 根据一批主键 ID 批量获取工作流元数据，用于前端展示流程名称等轻量信息
	FindByIds(ctx context.Context, ids []int64) ([]domain.Workflow, error)
	// Update 更新现有的工作流设计与图结构，返回受影响行数
	Update(ctx context.Context, req domain.Workflow) (int64, error)
	// Delete 根据 ID 删除工作流定义，返回受影响行数
	Delete(ctx context.Context, id int64) (int64, error)
	// Deploy 部署发布流程到 easyflow 底层引擎并加锁落库此时的画布版本快照
	Deploy(ctx context.Context, flow domain.Workflow) error
	// FindByKeyword 按照关键字(模糊匹配流程名或描述)进行分页列表搜索
	FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Workflow, int64, error)
	// GetAutomationProperty 获取已发布画布中特定自动任务节点的脚本或事件自动化扩展属性
	GetAutomationProperty(workflow easyflow.Workflow, nodeId string) (easyflow.AutomationProperty, error)
	// GetAutomationCodebookIds 获取工作流画布中自动化节点名称与脚本模板 ID 的映射
	GetAutomationCodebookIds(ctx context.Context, workflowId int64) (map[string]int64, error)
	// GetWorkflowSnapshot 依据引擎流程 ID 和发布版本号获取精确锁定的快照图结构详情
	GetWorkflowSnapshot(ctx context.Context, processID, version int) (domain.Workflow, error)
	// FindInstanceFlow 获取流程实例运行时所绑定版本的流程定义，提供特定历史快照回溯与降级解析
	FindInstanceFlow(ctx context.Context, workflowID int64, processID, version int) (domain.Workflow, error)
}

// IWorkflowBindingService 通知渠道绑定管理业务服务子接口
type IWorkflowBindingService interface {
	// Create 创建通知与特定模版的绑定记录，返回生成的主键 ID
	Create(ctx context.Context, n domain.NotifyBinding) (int64, error)
	// Update 覆盖更新绑定项，返回受影响行数
	Update(ctx context.Context, n domain.NotifyBinding) (int64, error)
	// Delete 根据主键 ID 删除绑定记录，返回受影响行数
	Delete(ctx context.Context, id int64) (int64, error)
	// List 查询特定工作流 ID 挂载的所有消息绑定详情列表
	List(ctx context.Context, workflowId int64) ([]domain.NotifyBinding, error)
	// GetEffective 根据流转时机类型及渠道获取匹配到的生效消息模版配置 (支持特定绑定 -> 全局 0 降级)
	GetEffective(ctx context.Context, workflowId int64, notifyType domain.NotifyType, channel string) (domain.NotifyBinding, error)
}

// Service 工作流大业务服务接口 (遵循接口隔离原则进行拆分并借助接口组合以兼容历史嵌套引用)
type Service interface {
	IWorkflowCoreService
	// AdminNotifyBinding 管理侧通道绑定子接口获取，提供对已有模块嵌套调用的完全向后兼容
	AdminNotifyBinding() IWorkflowBindingService
}

type workflowService struct {
	repo         repository.IWorkflowRepository
	engineSvc    engine.Service
	engineCovert easyflow.Converter
}

// NewWorkflowService 初始化工作流业务服务层实例
func NewWorkflowService(repo repository.IWorkflowRepository, engineSvc engine.Service, engineCovert easyflow.Converter) Service {
	return &workflowService{
		repo:         repo,
		engineSvc:    engineSvc,
		engineCovert: engineCovert,
	}
}

// --- Workflow 核心流程定义业务服务实现 ---

func (s *workflowService) Create(ctx context.Context, req domain.Workflow) (int64, error) {
	return s.repo.Create(ctx, req)
}

func (s *workflowService) List(ctx context.Context, offset, limit int64) ([]domain.Workflow, int64, error) {
	var (
		eg    errgroup.Group
		ws    []domain.Workflow
		total int64
	)

	eg.Go(func() error {
		var err error
		ws, err = s.repo.List(ctx, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.Total(ctx)
		return err
	})

	if err := eg.Wait(); err != nil {
		return ws, total, err
	}
	return ws, total, nil
}

func (s *workflowService) Find(ctx context.Context, id int64) (domain.Workflow, error) {
	return s.repo.Find(ctx, id)
}

func (s *workflowService) FindByIds(ctx context.Context, ids []int64) ([]domain.Workflow, error) {
	return s.repo.FindByIds(ctx, ids)
}

func (s *workflowService) Update(ctx context.Context, req domain.Workflow) (int64, error) {
	return s.repo.Update(ctx, req)
}

func (s *workflowService) Delete(ctx context.Context, id int64) (int64, error) {
	return s.repo.Delete(ctx, id)
}

func (s *workflowService) Deploy(ctx context.Context, wf domain.Workflow) error {
	// 1. 将画布中的前端 DSL 数据转换底层 easyflow 路由引擎所需的图拓扑 Process 模型
	process, err := s.engineCovert.Convert(s.toEasyWorkflow(wf))
	if err != nil {
		return err
	}

	// 2. 部署保存至底层流程引擎，并返回全局唯一的自增 processId
	processId, err := s.engineSvc.ProcessSave(ctx, process)
	if err != nil {
		return err
	}

	// 3. 关联回写主记录的 process_id，用于日常状态映射和数据追溯
	if err = s.repo.UpdateProcessId(ctx, wf.Id, processId); err != nil {
		return err
	}

	// 4. Double Check 安全验证: 获取刚刚成功部署发布的最新版本号
	version, err := s.engineSvc.GetLatestProcessVersion(ctx, processId)
	if err != nil {
		return err
	}

	// 5. 将当前发布的最新图数据在物理快照表 c_workflow_snapshot 中锁死存档，为以后的历史归档提供数据兜底
	return s.repo.CreateSnapshot(ctx, wf, processId, version)
}

func (s *workflowService) FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Workflow, int64, error) {
	var (
		eg    errgroup.Group
		ws    []domain.Workflow
		total int64
	)

	eg.Go(func() error {
		var err error
		ws, err = s.repo.FindByKeyword(ctx, keyword, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountByKeyword(ctx, keyword)
		return err
	})

	if err := eg.Wait(); err != nil {
		return ws, total, err
	}
	return ws, total, nil
}

func (s *workflowService) GetAutomationProperty(workflow easyflow.Workflow, nodeId string) (easyflow.AutomationProperty, error) {
	nodes, err := easyflow.ParseNodes(workflow.FlowData.Nodes)
	if err != nil {
		return easyflow.AutomationProperty{}, err
	}

	for _, node := range nodes {
		if node.ID == nodeId {
			return easyflow.ToNodeProperty[easyflow.AutomationProperty](node)
		}
	}

	return easyflow.AutomationProperty{}, errors.New("node not found")
}

func (s *workflowService) GetAutomationCodebookIds(ctx context.Context, workflowId int64) (map[string]int64, error) {
	wf, err := s.Find(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	nodes, err := easyflow.ParseNodes(wf.FlowData.Nodes)
	if err != nil {
		return nil, err
	}

	codebookIds := make(map[string]int64, len(nodes))
	for _, node := range nodes {
		if node.Type != "automation" {
			continue
		}
		property, err := easyflow.ToNodeProperty[easyflow.AutomationProperty](node)
		if err != nil {
			return nil, err
		}
		if property.Name == "" || property.CodebookId == 0 {
			continue
		}
		codebookIds[property.Name] = property.CodebookId
	}

	return codebookIds, nil
}

func (s *workflowService) GetWorkflowSnapshot(ctx context.Context, processID, version int) (domain.Workflow, error) {
	return s.repo.FindSnapshot(ctx, processID, version)
}

func (s *workflowService) FindInstanceFlow(ctx context.Context, workflowID int64, processID, version int) (domain.Workflow, error) {
	// 1. 获取工作流最新版元数据模型
	latest, err := s.Find(ctx, workflowID)

	// 2. 降级容灾逻辑：如果主记录在极端场景下已被物理删除或查询失败，则完全从此刻版本快照中拉取做物理回填恢复
	if err != nil {
		snapshot, snapErr := s.repo.FindSnapshot(ctx, processID, version)
		if snapErr != nil {
			return domain.Workflow{}, err
		}
		return snapshot, nil
	}

	// 3. 读取快照覆盖主实体的 flow_data，以保障用户点击历史工单图渲染时看到的是当时审批时的画布拓扑，而不是最新的画布，符合设计完整性
	snapshot, err := s.repo.FindSnapshot(ctx, processID, version)
	if err == nil {
		latest.FlowData = snapshot.FlowData
	}

	return latest, nil
}

func (s *workflowService) toEasyWorkflow(wf domain.Workflow) easyflow.Workflow {
	edges := make([]map[string]interface{}, len(wf.FlowData.Edges))
	for i, e := range wf.FlowData.Edges {
		edges[i] = e
	}
	nodes := make([]map[string]interface{}, len(wf.FlowData.Nodes))
	for i, n := range wf.FlowData.Nodes {
		nodes[i] = n
	}

	return easyflow.Workflow{
		Id:    wf.Id,
		Name:  wf.Name,
		Owner: wf.Owner,
		FlowData: easyflow.LogicFlow{
			Edges: edges,
			Nodes: nodes,
		},
	}
}

// --- AdminNotifyBinding 兼容嵌套接口返回逻辑 ---

func (s *workflowService) AdminNotifyBinding() IWorkflowBindingService {
	return &adminNotifyBindingService{
		repo: s.repo,
	}
}

type adminNotifyBindingService struct {
	repo repository.IWorkflowRepository
}

func (s *adminNotifyBindingService) Create(ctx context.Context, n domain.NotifyBinding) (int64, error) {
	return s.repo.CreateBinding(ctx, n)
}

func (s *adminNotifyBindingService) Update(ctx context.Context, n domain.NotifyBinding) (int64, error) {
	return s.repo.UpdateBinding(ctx, n)
}

func (s *adminNotifyBindingService) Delete(ctx context.Context, id int64) (int64, error) {
	return s.repo.DeleteBinding(ctx, id)
}

func (s *adminNotifyBindingService) List(ctx context.Context, workflowId int64) ([]domain.NotifyBinding, error) {
	return s.repo.ListBindings(ctx, workflowId)
}

func (s *adminNotifyBindingService) GetEffective(ctx context.Context, workflowId int64, notifyType domain.NotifyType, channel string) (domain.NotifyBinding, error) {
	return s.repo.GetEffectiveBinding(ctx, workflowId, notifyType, channel)
}
