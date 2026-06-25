package repository

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
)

// IWorkflowCoreRepository 工作流核心定义仓储子接口
type IWorkflowCoreRepository interface {
	// Create 创建流程定义，返回生成的自增 ID
	Create(ctx context.Context, req domain.Workflow) (int64, error)
	// List 分页查询流程定义列表，按时间逆序
	List(ctx context.Context, offset, limit int64) ([]domain.Workflow, error)
	// Total 统计当前租户空间下所有的流程定义总数
	Total(ctx context.Context) (int64, error)
	// Update 覆盖更新当前流程相关的字段及画布拓扑配置，返回受影响行数
	Update(ctx context.Context, req domain.Workflow) (int64, error)
	// UpdateProcessId 绑定此流程对应引擎的 ID
	UpdateProcessId(ctx context.Context, id int64, processId int) error
	// Delete 根据 ID 删除流程定义，返回受影响行数
	Delete(ctx context.Context, id int64) (int64, error)
	// Find 根据主键 ID 查询流程定义（返回最新版元数据及完整画布数据）
	Find(ctx context.Context, id int64) (domain.Workflow, error)
	// FindByIds 根据主键 ID 列表批量查询流程定义元数据
	FindByIds(ctx context.Context, ids []int64) ([]domain.Workflow, error)
	// FindByKeyword 模糊匹配流程名称及描述的分页检索
	FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Workflow, error)
	// CountByKeyword 计算含有对应关键字特征的流程定义总条数
	CountByKeyword(ctx context.Context, keyword string) (int64, error)
}

// ISnapshotRepository 流程版本发布物理画布快照仓储子接口
type ISnapshotRepository interface {
	// CreateSnapshot 为已部署发布的流程生成一份此刻的画布快照记录，用于版本控制及图形状态回溯
	CreateSnapshot(ctx context.Context, workflow domain.Workflow, processID, processVersion int) error
	// FindSnapshot 根据引擎分配的唯一流程模板 ID 和具体部署版本号检索对应的流程设计原始画布快照
	FindSnapshot(ctx context.Context, processID, processVersion int) (domain.Workflow, error)
}

// IWorkflowRepository 工作流仓储层大组合接口 (采用接口隔离原则拆分，再经由嵌入优雅组合，兼具内聚与拓展特性)
type IWorkflowRepository interface {
	IWorkflowCoreRepository
	ISnapshotRepository
}

type workflowRepository struct {
	dao dao.IWorkflowDAO
}

// NewWorkflowRepository 初始化工作流仓储层实例
func NewWorkflowRepository(dao dao.IWorkflowDAO) IWorkflowRepository {
	return &workflowRepository{
		dao: dao,
	}
}

// --- Workflow 核心流程定义仓储实现 ---

func (repo *workflowRepository) Create(ctx context.Context, req domain.Workflow) (int64, error) {
	return repo.dao.CreateWorkflow(ctx, repo.toEntity(req))
}

func (repo *workflowRepository) List(ctx context.Context, offset, limit int64) ([]domain.Workflow, error) {
	ws, err := repo.dao.List(ctx, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(ws, func(idx int, src dao.Workflow) domain.Workflow {
		return repo.toDomain(src)
	}), nil
}

func (repo *workflowRepository) Total(ctx context.Context) (int64, error) {
	return repo.dao.Count(ctx)
}

func (repo *workflowRepository) Update(ctx context.Context, req domain.Workflow) (int64, error) {
	return repo.dao.UpdateWorkflow(ctx, repo.toEntity(req))
}

func (repo *workflowRepository) UpdateProcessId(ctx context.Context, id int64, processId int) error {
	return repo.dao.UpdateProcessId(ctx, id, processId)
}

func (repo *workflowRepository) Delete(ctx context.Context, id int64) (int64, error) {
	return repo.dao.DeleteWorkflow(ctx, id)
}

func (repo *workflowRepository) Find(ctx context.Context, id int64) (domain.Workflow, error) {
	w, err := repo.dao.FindWorkflow(ctx, id)
	if err != nil {
		return domain.Workflow{}, err
	}
	return repo.toDomain(w), nil
}

func (repo *workflowRepository) FindByIds(ctx context.Context, ids []int64) ([]domain.Workflow, error) {
	ws, err := repo.dao.FindByIds(ctx, ids)
	if err != nil {
		return nil, err
	}
	return slice.Map(ws, func(idx int, src dao.Workflow) domain.Workflow {
		return repo.toDomain(src)
	}), nil
}

func (repo *workflowRepository) FindByKeyword(ctx context.Context, keyword string, offset, limit int64) ([]domain.Workflow, error) {
	ws, err := repo.dao.FindByKeyword(ctx, keyword, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(ws, func(idx int, src dao.Workflow) domain.Workflow {
		return repo.toDomain(src)
	}), nil
}

func (repo *workflowRepository) CountByKeyword(ctx context.Context, keyword string) (int64, error) {
	return repo.dao.CountByKeyword(ctx, keyword)
}

// --- Snapshot 流程版本快照仓储实现 ---

func (repo *workflowRepository) CreateSnapshot(ctx context.Context, workflow domain.Workflow, processID, processVersion int) error {
	return repo.dao.CreateSnapshot(ctx, dao.Snapshot{
		WorkflowId:     workflow.Id,
		ProcessId:      processID,
		ProcessVersion: processVersion,
		Name:           workflow.Name,
		FlowData: sqlx.JsonField[dao.LogicFlow]{
			Val: dao.LogicFlow{
				Edges: workflow.FlowData.Edges,
				Nodes: workflow.FlowData.Nodes,
			},
			Valid: true,
		},
	})
}

func (repo *workflowRepository) FindSnapshot(ctx context.Context, processID, processVersion int) (domain.Workflow, error) {
	s, err := repo.dao.FindSnapshotByProcess(ctx, processID, processVersion)
	if err != nil {
		return domain.Workflow{}, err
	}

	return domain.Workflow{
		Id:        s.WorkflowId,
		ProcessId: s.ProcessId,
		Name:      s.Name,
		FlowData: domain.LogicFlow{
			Edges: s.FlowData.Val.Edges,
			Nodes: s.FlowData.Val.Nodes,
		},
	}, nil
}

// --- 实体与领域模型双向防腐转换辅助函数 ---

func (repo *workflowRepository) toEntity(req domain.Workflow) dao.Workflow {
	return dao.Workflow{
		Id:           req.Id,
		TemplateId:   req.TemplateId,
		Name:         req.Name,
		Icon:         req.Icon,
		Owner:        req.Owner,
		Desc:         req.Desc,
		ProcessId:    req.ProcessId,
		NotifyMethod: req.NotifyMethod.ToUint8(),
		IsNotify:     req.IsNotify,
		FlowData: sqlx.JsonField[dao.LogicFlow]{
			Val: dao.LogicFlow{
				Edges: req.FlowData.Edges,
				Nodes: req.FlowData.Nodes,
			},
			Valid: true,
		},
	}
}

func (repo *workflowRepository) toDomain(w dao.Workflow) domain.Workflow {
	return domain.Workflow{
		Id:           w.Id,
		TemplateId:   w.TemplateId,
		Name:         w.Name,
		Icon:         w.Icon,
		Owner:        w.Owner,
		Desc:         w.Desc,
		ProcessId:    w.ProcessId,
		NotifyMethod: domain.NotifyMethod(w.NotifyMethod),
		IsNotify:     w.IsNotify,
		FlowData: domain.LogicFlow{
			Edges: w.FlowData.Val.Edges,
			Nodes: w.FlowData.Val.Nodes,
		},
	}
}
