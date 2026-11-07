package workflow

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/compiler"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
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
	// Deploy 部署发布流程到底层引擎并加锁落库此时的画布版本快照
	Deploy(ctx context.Context, flow domain.Workflow) error
	// FindByKeyword 按照关键字(模糊匹配流程名或描述)进行分页列表搜索
	FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Workflow, int64, error)
	// GetAutomationNodes 获取工作流画布中的自动化节点及默认执行单元。
	GetAutomationNodes(ctx context.Context, workflowId int64) ([]domain.AutomationNodeRef, error)
	// GetWorkflowSnapshot 依据引擎流程 ID 和发布版本号获取精确锁定的快照图结构详情
	GetWorkflowSnapshot(ctx context.Context, processID, version int) (domain.Workflow, error)
	// FindInstanceFlow 获取流程实例运行时所绑定版本的流程定义，提供特定历史快照回溯与降级解析
	FindInstanceFlow(ctx context.Context, workflowID int64, processID, version int) (domain.Workflow, error)
	// GetHighlightedGraph 根据审批轨迹流转情况计算并点亮画布图
	GetHighlightedGraph(ctx context.Context, workflowId int64, instanceID domain.ID, status domain.Status) (domain.Workflow, error)
	// GetMaxSnapshotVersion 获取指定工作流当前已发布的最大快照版本号（未发布过时返回 0）
	GetMaxSnapshotVersion(ctx context.Context, workflowID int64) (int, error)
}

// Service 工作流大业务服务接口
type Service interface {
	IWorkflowCoreService
}

type workflowService struct {
	repo     repository.IWorkflowRepository
	engine   engine.IEngine
	compiler compiler.ICompositeCompiler
}

// NewWorkflowService 初始化工作流业务服务层实例
func NewWorkflowService(
	repo repository.IWorkflowRepository,
	engine engine.IEngine,
	compiler compiler.ICompositeCompiler,
) Service {
	return &workflowService{
		repo:     repo,
		engine:   engine,
		compiler: compiler,
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
	if err := wf.ValidateEngineType(); err != nil {
		return err
	}

	// 1. 编译器将前端画布编译为目标引擎原生定义
	compileResult, err := s.compiler.Compile(ctx, wf)
	if err != nil {
		return fmt.Errorf("工作流画布编译失败: %w", err)
	}

	// 2. 底层流程引擎调度门面统一执行部署发布
	deployInfo, err := s.engine.Deploy(ctx, wf.ResolveEngineType(), compileResult.RawDef)
	if err != nil {
		return fmt.Errorf("流程引擎部署失败: %w", err)
	}

	// 3. 计算平台全局统一自增版本号（继承历史最大快照版本，防止跨引擎切换导致版本重置为 1）
	maxVersion, err := s.repo.GetMaxSnapshotVersion(ctx, wf.Id)
	if err != nil {
		return fmt.Errorf("计算工作流版本失败: %w", err)
	}
	newVersion := maxVersion + 1
	if deployInfo.Version > newVersion {
		newVersion = deployInfo.Version
	}

	// 4. 关联回写 process_id（若底层引擎生成了自增ID则更新，flowx 引擎下直接以 wf.Id 建立映射）
	processId := deployInfo.ProcessID
	if processId == 0 {
		processId = int(wf.Id)
	}
	if processId != wf.ProcessId {
		if err = s.repo.UpdateProcessId(ctx, wf.Id, processId); err != nil {
			return fmt.Errorf("关联回写 process_id 失败: %w", err)
		}
	}

	// 5. 统一将当前发布的最新图数据在物理快照表中锁死存档
	return s.repo.CreateSnapshot(ctx, wf, processId, newVersion)
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

func (s *workflowService) GetAutomationNodes(ctx context.Context, workflowId int64) ([]domain.AutomationNodeRef, error) {
	wf, err := s.Find(ctx, workflowId)
	if err != nil {
		return nil, err
	}

	graph, err := flowdsl.Parse(wf.FlowData)
	if err != nil {
		return nil, fmt.Errorf("解析流程拓扑失败: %w", err)
	}

	refs := make([]domain.AutomationNodeRef, 0)
	for _, node := range graph.GetOrderedNodes() {
		autoCfg, isAuto := node.AsAutoTask()
		if !isAuto || node.Name == "" || autoCfg.CodebookID <= 0 {
			continue
		}
		refs = append(refs, domain.AutomationNodeRef{
			ID:         node.ID,
			Name:       node.Name,
			CodebookID: autoCfg.CodebookID,
			RunnerID:   autoCfg.RunnerID,
		})
	}

	return refs, nil
}

func (s *workflowService) GetWorkflowSnapshot(ctx context.Context, processID, version int) (domain.Workflow, error) {
	return s.repo.FindSnapshot(ctx, processID, version)
}

func (s *workflowService) FindInstanceFlow(ctx context.Context, workflowID int64, processID, version int) (domain.Workflow, error) {
	// 1. 获取工作流最新版元数据模型，确立工作流主数据与所属引擎类型
	latest, err := s.Find(ctx, workflowID)
	if err != nil {
		return domain.Workflow{}, fmt.Errorf("查询工作流定义失败 [workflowID=%d]: %w", workflowID, err)
	}

	// 2. 优先根据 workflowID + version 精准定位快照（现代 FlowX 与已升级流程统一按此检索）
	snapshot, snapErr := s.repo.FindSnapshotByWorkflowVersion(ctx, workflowID, latest.EngineType, version)
	if snapErr != nil && latest.ResolveEngineType() == domain.EngineTypeEasyFlow && processID > 0 {
		// 仅对历史未绑定 workflow_id 的极老 EasyFlow 存量数据进行安全兼容
		snapshot, snapErr = s.repo.FindSnapshot(ctx, processID, version)
	}

	// 3. 读取快照覆盖主实体的 flow_data，以保障用户点击历史工单图渲染时看到的是当时审批时的画布拓扑
	if snapErr == nil {
		latest.FlowData = snapshot.FlowData
	}

	return latest, nil
}

func (s *workflowService) GetHighlightedGraph(ctx context.Context, workflowId int64, instanceID domain.ID, status domain.Status) (domain.Workflow, error) {
	wf, err := s.Find(ctx, workflowId)
	if err != nil {
		return domain.Workflow{}, err
	}

	targetEngine := wf.ResolveEngineType()

	// 2. 通过统一流程引擎接口获取该实例的执行态点亮视图
	execView, err := s.engine.GetInstanceExecutionView(ctx, targetEngine, instanceID, status.ToUint8())
	if err != nil {
		return domain.Workflow{}, fmt.Errorf("获取流程实例执行态轨迹失败: %w", err)
	}

	// 2. 根据实例运行时的版本信息，从物理快照层回溯读取锁定的画布拓扑 FlowData
	flow, err := s.FindInstanceFlow(ctx, workflowId, execView.ProcessID, execView.ProcessVersion)
	if err != nil {
		return domain.Workflow{}, err
	}

	// 3. 利用 flowdsl 进行多画布兼容的连线高亮装饰与图层 Z-Index 优化
	highlightedCanvas, err := flowdsl.HighlightCanvas(flow.FlowData, flowdsl.ExecutionTrace{
		TraversedEdges: execView.TraversedEdges,
		NodeStatusMap:  execView.NodeStatusMap,
	})
	if err != nil {
		return domain.Workflow{}, fmt.Errorf("装饰高亮流程拓扑失败: %w", err)
	}

	flow.FlowData = highlightedCanvas
	return flow, nil
}

func (s *workflowService) GetMaxSnapshotVersion(ctx context.Context, workflowID int64) (int, error) {
	return s.repo.GetMaxSnapshotVersion(ctx, workflowID)
}
