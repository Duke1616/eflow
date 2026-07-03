package repository

import (
	"context"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/ecodeclub/ekit/slice"
)

// IEngineRepository 流程引擎仓储接口
type IEngineRepository interface {
	// TodoList 获取指定用户的待办任务列表，转换为领域对象
	TodoList(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) ([]domain.Instance, error)
	// CountTodo 统计指定用户的待办任务总数
	CountTodo(ctx context.Context, userId, processName string) (int64, error)
	// CountStartUser 统计用户发起的流程实例总数
	CountStartUser(ctx context.Context, userId, processName string) (int64, error)
	// GetTasksByCurrentNodeId 根据当前进行节点获取其对应未完成的任务
	GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]model.Task, error)
	// ListStartUser 获取由指定用户发起的流程实例列表
	ListStartUser(ctx context.Context, userId, processName string, offset, limit int) ([]domain.Instance, error)
	// ListTaskRecord 获取指定实例的全局审批变更流转任务链
	ListTaskRecord(ctx context.Context, processInstId, offset, limit int) ([]model.Task, error)
	// CountTaskRecord 统计指定流程实例的流转任务总条数
	CountTaskRecord(ctx context.Context, processInstId int) (int64, error)
	// UpdateIsFinishedByPreNodeId 系统自动流转前置代理节点任务为已完结
	UpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// ForceUpdateIsFinishedByPreNodeId 强制归档清理指定前置节点下挂载的所有流转任务（包含已完成）
	ForceUpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// ForceUpdateIsFinishedByNodeId 强制归档清理指定节点 ID 下挂载的所有流转任务（包含已完成）
	ForceUpdateIsFinishedByNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// CountReject 获取针对该任务的驳回记录数
	CountReject(ctx context.Context, taskId int) (int64, error)
	// ListTasksByProcInstIds 批量获取流程实例列表中属于发起人的流转中任务
	ListTasksByProcInstIds(ctx context.Context, processInstIds []int, starter string) ([]domain.Instance, error)
	// GetAutomationTask 获取当前等待自动执行的节点任务详情
	GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (model.Task, error)
	// GetTasksByInstUsers 获取属于指定用户组且在该实例下待办的任务列表
	GetTasksByInstUsers(ctx context.Context, processInstId int, userIds []string) ([]model.Task, error)
	// GetTicketIdByVariable 获取与当前工作流关联的业务工单唯一标识 ID
	GetTicketIdByVariable(ctx context.Context, processInstId int) (string, error)
	// GetProxyNodeID 根据前置节点检索已产生的自动流转代理节点任务
	GetProxyNodeID(ctx context.Context, processInstId int, prevNodeID string) (model.Task, error)
	// GetProxyNodeByProcessInstId 检索属于该实例唯一的自动代理节点任务记录
	GetProxyNodeByProcessInstId(ctx context.Context, processInstId int) (model.Task, error)
	// DeleteProxyNodeByNodeId 删除代理流转临时节点任务
	DeleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error
	// UpdateTaskPrevNodeID 修改流转任务的前置节点 ID（用于条件分支动态回溯重算）
	UpdateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error
	// CreateSkippedTask 写入被跳过节点的已完结任务记录
	CreateSkippedTask(ctx context.Context, task model.Task) error
	// GetInstanceByID 根据实例 ID 获取流程实例的基础及多租户信息
	GetInstanceByID(ctx context.Context, processInstId int) (domain.Instance, error)
	// GetProcessDefineByVersion 根据流程 ID 和版本号获取当时锁定的完整流程图定义
	GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error)
	// GetLatestProcessVersion 获取该工作流最新生效并可用的版本号
	GetLatestProcessVersion(ctx context.Context, processID int) (int, error)
	// Transfer 将特定代办审批节点任务转交/转签至新指定的用户列表中
	Transfer(ctx context.Context, taskId int, userIds []string) ([]model.Task, error)
}

type engineRepository struct {
	engineDao dao.IEngineDAO
}

// NewProcessEngineRepository 初始化引擎仓储层
func NewProcessEngineRepository(engineDao dao.IEngineDAO) IEngineRepository {
	return &engineRepository{
		engineDao: engineDao,
	}
}

func (repo *engineRepository) UpdateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error {
	return repo.engineDao.UpdateTaskPrevNodeID(ctx, taskId, prevNodeId)
}

func (repo *engineRepository) Transfer(ctx context.Context, taskId int, userIds []string) ([]model.Task, error) {
	return repo.engineDao.Transfer(ctx, taskId, userIds)
}

func (repo *engineRepository) CreateSkippedTask(ctx context.Context, task model.Task) error {
	return repo.engineDao.CreateSkippedTask(ctx, task)
}

func (repo *engineRepository) GetProxyNodeID(ctx context.Context, processInstId int, prevNodeID string) (model.Task, error) {
	return repo.engineDao.GetProxyNodeID(ctx, processInstId, prevNodeID)
}

func (repo *engineRepository) GetProxyNodeByProcessInstId(ctx context.Context, processInstId int) (model.Task, error) {
	return repo.engineDao.GetProxyNodeByProcessInstId(ctx, processInstId)
}

func (repo *engineRepository) DeleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error {
	return repo.engineDao.DeleteProxyNodeByNodeId(ctx, processInstId, nodeId)
}

func (repo *engineRepository) GetInstanceByID(ctx context.Context, processInstId int) (domain.Instance, error) {
	inst, err := repo.engineDao.GetInstanceByID(ctx, processInstId)
	if err != nil {
		return domain.Instance{}, err
	}
	return repo.toDomainByInstance(inst), nil
}

func (repo *engineRepository) GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error) {
	return repo.engineDao.GetProcessDefineByVersion(ctx, processID, version)
}

func (repo *engineRepository) GetLatestProcessVersion(ctx context.Context, processID int) (int, error) {
	return repo.engineDao.GetLatestProcessVersion(ctx, processID)
}

func (repo *engineRepository) GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]model.Task, error) {
	return repo.engineDao.GetTasksByCurrentNodeId(ctx, processInstId, currentNodeId)
}

func (repo *engineRepository) GetTicketIdByVariable(ctx context.Context, processInstId int) (string, error) {
	return repo.engineDao.GetTicketIdByVariable(ctx, processInstId)
}

func (repo *engineRepository) GetTasksByInstUsers(ctx context.Context, processInstId int, userIds []string) ([]model.Task, error) {
	return repo.engineDao.GetTasksByInstUsers(ctx, processInstId, userIds)
}

func (repo *engineRepository) GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (model.Task, error) {
	return repo.engineDao.GetAutomationTask(ctx, currentNodeId, processInstId)
}

func (repo *engineRepository) ListTasksByProcInstIds(ctx context.Context, processInstIds []int, starter string) (
	[]domain.Instance, error) {
	ts, err := repo.engineDao.ListTasksByProcInstId(ctx, processInstIds, starter)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src model.Task) domain.Instance {
		return repo.toDomainByTask(src)
	}), nil
}

func (repo *engineRepository) CountReject(ctx context.Context, taskId int) (int64, error) {
	return repo.engineDao.CountReject(ctx, taskId)
}

func (repo *engineRepository) UpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return repo.engineDao.UpdateIsFinishedByPreNodeId(ctx, processInstId, nodeId, status, comment)
}

func (repo *engineRepository) ForceUpdateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return repo.engineDao.ForceUpdateIsFinishedByPreNodeId(ctx, processInstId, nodeId, status, comment)
}

func (repo *engineRepository) ForceUpdateIsFinishedByNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return repo.engineDao.ForceUpdateIsFinishedByNodeId(ctx, processInstId, nodeId, status, comment)
}

func (repo *engineRepository) TodoList(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) ([]domain.Instance, error) {
	ts, err := repo.engineDao.ListTodo(ctx, userId, processName, sortByAse, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src dao.Instance) domain.Instance {
		return repo.toDomainByInstance(src)
	}), nil
}

func (repo *engineRepository) ListStartUser(ctx context.Context, userId, processName string, offset,
	limit int) ([]domain.Instance, error) {
	ts, err := repo.engineDao.ListStartUser(ctx, userId, processName, offset, limit)
	if err != nil {
		return nil, err
	}
	return slice.Map(ts, func(idx int, src dao.Instance) domain.Instance {
		return repo.toDomainByInstance(src)
	}), nil
}

func (repo *engineRepository) CountTodo(ctx context.Context, userId, processName string) (int64, error) {
	return repo.engineDao.CountTodo(ctx, userId, processName)
}

func (repo *engineRepository) CountStartUser(ctx context.Context, userId, processName string) (int64, error) {
	return repo.engineDao.CountStartUser(ctx, userId, processName)
}

func (repo *engineRepository) ListTaskRecord(ctx context.Context, processInstId, offset, limit int) ([]model.Task, error) {
	return repo.engineDao.ListTaskRecord(ctx, processInstId, offset, limit)
}

func (repo *engineRepository) CountTaskRecord(ctx context.Context, processInstId int) (int64, error) {
	return repo.engineDao.CountTaskRecord(ctx, processInstId)
}

func (repo *engineRepository) toDomainByInstance(req dao.Instance) domain.Instance {
	return domain.Instance{
		TaskID:          req.TaskID,
		ProcInstID:      req.ProcInstID,
		ProcVersion:     req.ProcVersion,
		ProcID:          req.ProcID,
		ProcName:        req.ProcName,
		Status:          req.Status,
		CreateTime:      req.CreateTime,
		CurrentNodeID:   req.CurrentNodeID,
		CurrentNodeName: req.CurrentNodeName,
		BusinessID:      req.BusinessID,
		ApprovedBy:      req.ApprovedBy,
		Starter:         req.Starter,
	}
}

func (repo *engineRepository) toDomainByTask(req model.Task) domain.Instance {
	var createTime *time.Time
	if req.CreateTime != nil {
		t := time.Time(*req.CreateTime)
		createTime = &t
	}

	return domain.Instance{
		TaskID:          req.TaskID,
		ProcInstID:      req.ProcInstID,
		ProcID:          req.ProcID,
		ProcName:        req.ProcName,
		BusinessID:      req.BusinessID,
		Starter:         req.Starter,
		CurrentNodeID:   req.NodeID,
		CurrentNodeName: req.NodeName,
		CreateTime:      createTime,
		ApprovedBy:      req.UserID,
		Status:          req.Status,
	}
}
