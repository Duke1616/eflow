package repository

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository/dao"
	"github.com/Duke1616/eflow/pkg/sqlx"
	"github.com/ecodeclub/ekit/slice"
	"gorm.io/gorm"
)

var ErrTaskNotFound = gorm.ErrRecordNotFound

// TaskRepository 自动化作业任务的数据仓储接口
type TaskRepository interface {
	// CreateTask 在仓储物理层持久化创建一个新的作业任务
	CreateTask(ctx context.Context, req domain.Task) (domain.Task, error)
	// FindByProcessInstID 根据工作流实例 ID 与对应的当前节点 ID 查找特定的任务记录
	FindByProcessInstID(ctx context.Context, processInstID int, nodeID string) (domain.Task, error)
	// FindOrCreate 尝试根据实例 ID 和节点 ID 查询作业任务，若不存在则调用 CreateTask 进行逻辑兜底创建
	FindOrCreate(ctx context.Context, req domain.Task) (domain.Task, error)
	// FindByID 精确查找指定作业任务自增主键的持久化数据
	FindByID(ctx context.Context, id int64) (domain.Task, error)
	// UpdateTask 更新已存在任务的全部非空物理字段及属性
	UpdateTask(ctx context.Context, req domain.Task) (int64, error)
	// UpdateTaskStatus 更新特定任务的执行状态及触发位置信息
	UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error)
	// UpdateVariables 设置并批量覆写任务绑定的参数环境变量快照
	UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error)
	// ListTask 分页抓取全量作业任务列表
	ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, error)
	// ListTaskByStatus 根据状态分页过滤拉取任务
	ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]domain.Task, error)
	// ListTaskByStatusAndKind 根据执行协议类型（如 GRPC/KAFKA）与当前状态联合筛选分页列表
	ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]domain.Task, error)
	// Total 根据状态统计底层满足条件的数据总数
	Total(ctx context.Context, status uint8) (int64, error)
	// TotalByStatusAndKind 根据类型与状态联合统计总数
	TotalByStatusAndKind(ctx context.Context, status uint8, kind string) (int64, error)
	// UpdateArgs 单独更新任务下发时透传的自定义参数快照
	UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error)
	// ListSuccessTasksByUtime 拉取处于特定更新时间之后且状态成功的所有任务，以便自愈轮询层安全执行
	ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]domain.Task, error)
	// TotalByUtime 统计在特定时间戳之后状态成功的任务总行数
	TotalByUtime(ctx context.Context, utime int64) (int64, error)
	// FindTaskByNodeID 查询指定流程实例与节点的持久化最终作业结果
	FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (domain.Task, error)
	// ListReadyTasks 获取已达计划执行时间且状态属于待触发的就绪任务列表
	ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error)
	// ListTaskByInstanceID 分页过滤提取指定流程实例下的全部子任务列表
	ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]domain.Task, error)
	// TotalByInstanceID 统计特定实例下属的子任务记录总量
	TotalByInstanceID(ctx context.Context, instanceID int) (int64, error)
	// MarkTaskAsAutoPassed 将已成功处理过的作业任务标记为已自动通过状态，防止流程重复流转
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	// UpdateExternalID 绑定持久层与三方分布式引擎（如任务调度平台）的外部作业实例映射
	UpdateExternalID(ctx context.Context, id int64, externalID string) error
}

type taskRepository struct {
	dao dao.TaskDAO
}

func NewTaskRepository(dao dao.TaskDAO) TaskRepository {
	return &taskRepository{dao: dao}
}

func (repo *taskRepository) CreateTask(ctx context.Context, req domain.Task) (domain.Task, error) {
	t, err := repo.dao.CreateTask(ctx, repo.toEntity(req))
	if err != nil {
		return domain.Task{}, err
	}
	return repo.toDomain(t), nil
}

func (repo *taskRepository) FindByProcessInstID(ctx context.Context, processInstID int, nodeID string) (domain.Task, error) {
	task, err := repo.dao.FindByProcessInstID(ctx, processInstID, nodeID)
	return repo.toDomain(task), err
}

func (repo *taskRepository) FindOrCreate(ctx context.Context, req domain.Task) (domain.Task, error) {
	task, err := repo.dao.FindByProcessInstID(ctx, req.ProcessInstId, req.CurrentNodeId)
	if err == nil {
		return repo.toDomain(task), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, err
	}
	return repo.CreateTask(ctx, req)
}

func (repo *taskRepository) FindByID(ctx context.Context, id int64) (domain.Task, error) {
	task, err := repo.dao.FindByID(ctx, id)
	return repo.toDomain(task), err
}

func (repo *taskRepository) UpdateTask(ctx context.Context, req domain.Task) (int64, error) {
	return repo.dao.UpdateTask(ctx, repo.toEntity(req))
}

func (repo *taskRepository) UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error) {
	return repo.dao.UpdateTaskStatus(ctx, repo.toUpdateEntity(req))
}

func (repo *taskRepository) UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error) {
	return repo.dao.UpdateVariables(ctx, id, variables)
}

func (repo *taskRepository) ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, error) {
	ts, err := repo.dao.ListTask(ctx, offset, limit)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]domain.Task, error) {
	ts, err := repo.dao.ListTaskByStatus(ctx, offset, limit, status)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]domain.Task, error) {
	ts, err := repo.dao.ListTaskByStatusAndKind(ctx, offset, limit, status, kind)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) Total(ctx context.Context, status uint8) (int64, error) {
	return repo.dao.Count(ctx, status)
}

func (repo *taskRepository) TotalByStatusAndKind(ctx context.Context, status uint8, kind string) (int64, error) {
	return repo.dao.CountByStatusAndKind(ctx, status, kind)
}

func (repo *taskRepository) UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error) {
	return repo.dao.UpdateArgs(ctx, id, domain.TaskArgs(args))
}

func (repo *taskRepository) ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]domain.Task, error) {
	ts, err := repo.dao.ListSuccessTasksByUtime(ctx, offset, limit, utime)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) TotalByUtime(ctx context.Context, utime int64) (int64, error) {
	return repo.dao.TotalByUtime(ctx, utime)
}

func (repo *taskRepository) FindTaskByNodeID(ctx context.Context, instanceID int, nodeID string) (domain.Task, error) {
	task, err := repo.dao.FindTaskByNodeID(ctx, instanceID, nodeID)
	return repo.toDomain(task), err
}

func (repo *taskRepository) ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error) {
	ts, err := repo.dao.ListReadyTasks(ctx, limit)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) ListTaskByInstanceID(ctx context.Context, offset, limit int64, instanceID int) ([]domain.Task, error) {
	ts, err := repo.dao.ListTaskByInstanceID(ctx, offset, limit, instanceID)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) TotalByInstanceID(ctx context.Context, instanceID int) (int64, error) {
	return repo.dao.TotalByInstanceID(ctx, instanceID)
}

func (repo *taskRepository) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return repo.dao.MarkTaskAsAutoPassed(ctx, id)
}

func (repo *taskRepository) UpdateExternalID(ctx context.Context, id int64, externalID string) error {
	return repo.dao.UpdateExternalID(ctx, id, externalID)
}

func (repo *taskRepository) toUpdateEntity(req domain.TaskResult) dao.Task {
	return dao.Task{
		Id:              req.Id,
		Result:          req.Result,
		WantResult:      req.WantResult,
		Status:          req.Status.ToUint8(),
		TriggerPosition: req.TriggerPosition,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		RetryCount:      req.RetryCount,
	}
}

func (repo *taskRepository) toEntity(req domain.Task) dao.Task {
	return dao.Task{
		Id:              req.Id,
		TicketID:        req.TicketID,
		ProcessInstId:   req.ProcessInstId,
		CurrentNodeId:   req.CurrentNodeId,
		TriggerPosition: req.TriggerPosition,
		WorkflowId:      req.WorkflowId,
		CodebookUid:     req.CodebookUid,
		Code:            req.Code,
		Language:        req.Language,
		Args:            sqlx.JsonField[domain.TaskArgs]{Val: req.Args, Valid: true},
		Variables: sqlx.JsonField[[]domain.Variables]{
			Val:   req.Variables,
			Valid: true,
		},
		Status:        req.Status.ToUint8(),
		Result:        req.Result,
		WantResult:    req.WantResult,
		ExternalId:    req.ExternalId,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		RetryCount:    req.RetryCount,
		IsTiming:      req.IsTiming,
		ScheduledTime: req.ScheduledTime,
		Kind:          req.Kind.ToString(),
		Target:        req.Target,
		Handler:       req.Handler,
	}
}

func (repo *taskRepository) toDomain(req dao.Task) domain.Task {
	t := domain.Task{
		Id:              req.Id,
		TicketID:        req.TicketID,
		ProcessInstId:   req.ProcessInstId,
		CurrentNodeId:   req.CurrentNodeId,
		TriggerPosition: req.TriggerPosition,
		WorkflowId:      req.WorkflowId,
		CodebookUid:     req.CodebookUid,
		Code:            req.Code,
		Language:        req.Language,
		Args:            req.Args.Val,
		Variables:       req.Variables.Val,
		Status:          domain.TaskStatus(req.Status),
		Result:          req.Result,
		WantResult:      req.WantResult,
		ExternalId:      req.ExternalId,
		StartTime:       req.StartTime,
		EndTime:         req.EndTime,
		RetryCount:      req.RetryCount,
		IsTiming:        req.IsTiming,
		ScheduledTime:   req.ScheduledTime,
		Kind:            domain.Kind(req.Kind),
		Target:          req.Target,
		Handler:         req.Handler,
		Ctime:           req.Ctime,
		Utime:           req.Utime,
	}
	if t.Args == nil {
		t.Args = domain.TaskArgs{}
	}
	if t.Kind == "" {
		t.Kind = domain.KAFKA
	}
	return t
}
