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

type TaskRepository interface {
	CreateTask(ctx context.Context, req domain.Task) (domain.Task, error)
	FindByProcessInstId(ctx context.Context, processInstId int, nodeId string) (domain.Task, error)
	FindOrCreate(ctx context.Context, req domain.Task) (domain.Task, error)
	FindById(ctx context.Context, id int64) (domain.Task, error)
	UpdateTask(ctx context.Context, req domain.Task) (int64, error)
	UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error)
	UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error)
	ListTask(ctx context.Context, offset, limit int64) ([]domain.Task, error)
	ListTaskByStatus(ctx context.Context, offset, limit int64, status uint8) ([]domain.Task, error)
	ListTaskByStatusAndKind(ctx context.Context, offset, limit int64, status uint8, kind string) ([]domain.Task, error)
	Total(ctx context.Context, status uint8) (int64, error)
	TotalByStatusAndKind(ctx context.Context, status uint8, kind string) (int64, error)
	UpdateArgs(ctx context.Context, id int64, args map[string]interface{}) (int64, error)
	ListSuccessTasksByUtime(ctx context.Context, offset, limit int64, utime int64) ([]domain.Task, error)
	TotalByUtime(ctx context.Context, utime int64) (int64, error)
	FindTaskResult(ctx context.Context, instanceId int, nodeId string) (domain.Task, error)
	ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error)
	ListTaskByInstanceId(ctx context.Context, offset, limit int64, instanceId int) ([]domain.Task, error)
	TotalByInstanceId(ctx context.Context, instanceId int) (int64, error)
	MarkTaskAsAutoPassed(ctx context.Context, id int64) error
	UpdateExternalId(ctx context.Context, id int64, externalId string) error
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

func (repo *taskRepository) FindByProcessInstId(ctx context.Context, processInstId int, nodeId string) (domain.Task, error) {
	task, err := repo.dao.FindByProcessInstId(ctx, processInstId, nodeId)
	return repo.toDomain(task), err
}

func (repo *taskRepository) FindOrCreate(ctx context.Context, req domain.Task) (domain.Task, error) {
	task, err := repo.dao.FindByProcessInstId(ctx, req.ProcessInstId, req.CurrentNodeId)
	if err == nil {
		return repo.toDomain(task), nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return domain.Task{}, err
	}
	return repo.CreateTask(ctx, req)
}

func (repo *taskRepository) FindById(ctx context.Context, id int64) (domain.Task, error) {
	task, err := repo.dao.FindById(ctx, id)
	return repo.toDomain(task), err
}

func (repo *taskRepository) UpdateTask(ctx context.Context, req domain.Task) (int64, error) {
	return repo.dao.UpdateTask(ctx, repo.toEntity(req))
}

func (repo *taskRepository) UpdateTaskStatus(ctx context.Context, req domain.TaskResult) (int64, error) {
	return repo.dao.UpdateTaskStatus(ctx, repo.toUpdateEntity(req))
}

func (repo *taskRepository) UpdateVariables(ctx context.Context, id int64, variables []domain.Variables) (int64, error) {
	return repo.dao.UpdateVariables(ctx, id, slice.Map(variables, func(idx int, src domain.Variables) dao.Variables {
		return dao.Variables{Key: src.Key, Value: src.Value, Secret: src.Secret}
	}))
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

func (repo *taskRepository) FindTaskResult(ctx context.Context, instanceId int, nodeId string) (domain.Task, error) {
	task, err := repo.dao.FindTaskResult(ctx, instanceId, nodeId)
	return repo.toDomain(task), err
}

func (repo *taskRepository) ListReadyTasks(ctx context.Context, limit int64) ([]domain.Task, error) {
	ts, err := repo.dao.ListReadyTasks(ctx, limit)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) ListTaskByInstanceId(ctx context.Context, offset, limit int64, instanceId int) ([]domain.Task, error) {
	ts, err := repo.dao.ListTaskByInstanceId(ctx, offset, limit, instanceId)
	return slice.Map(ts, func(idx int, src dao.Task) domain.Task { return repo.toDomain(src) }), err
}

func (repo *taskRepository) TotalByInstanceId(ctx context.Context, instanceId int) (int64, error) {
	return repo.dao.TotalByInstanceId(ctx, instanceId)
}

func (repo *taskRepository) MarkTaskAsAutoPassed(ctx context.Context, id int64) error {
	return repo.dao.MarkTaskAsAutoPassed(ctx, id)
}

func (repo *taskRepository) UpdateExternalId(ctx context.Context, id int64, externalId string) error {
	return repo.dao.UpdateExternalId(ctx, id, externalId)
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
		OrderId:         req.OrderId,
		ProcessInstId:   req.ProcessInstId,
		CurrentNodeId:   req.CurrentNodeId,
		TriggerPosition: req.TriggerPosition,
		WorkflowId:      req.WorkflowId,
		CodebookUid:     req.CodebookUid,
		CodebookName:    req.CodebookName,
		Code:            req.Code,
		Language:        req.Language,
		Args:            sqlx.JsonField[domain.TaskArgs]{Val: req.Args, Valid: true},
		Variables: sqlx.JsonField[[]dao.Variables]{
			Val: slice.Map(req.Variables, func(idx int, src domain.Variables) dao.Variables {
				return dao.Variables{Key: src.Key, Value: src.Value, Secret: src.Secret}
			}),
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
		OrderId:         req.OrderId,
		ProcessInstId:   req.ProcessInstId,
		CurrentNodeId:   req.CurrentNodeId,
		TriggerPosition: req.TriggerPosition,
		WorkflowId:      req.WorkflowId,
		CodebookUid:     req.CodebookUid,
		CodebookName:    req.CodebookName,
		Code:            req.Code,
		Language:        req.Language,
		Args:            req.Args.Val,
		Variables: slice.Map(req.Variables.Val, func(idx int, src dao.Variables) domain.Variables {
			return domain.Variables{Key: src.Key, Value: src.Value, Secret: src.Secret}
		}),
		Status:        domain.TaskStatus(req.Status),
		Result:        req.Result,
		WantResult:    req.WantResult,
		ExternalId:    req.ExternalId,
		StartTime:     req.StartTime,
		EndTime:       req.EndTime,
		RetryCount:    req.RetryCount,
		IsTiming:      req.IsTiming,
		ScheduledTime: req.ScheduledTime,
		Kind:          domain.Kind(req.Kind),
		Target:        req.Target,
		Handler:       req.Handler,
		Ctime:         req.Ctime,
		Utime:         req.Utime,
	}
	if t.Args == nil {
		t.Args = domain.TaskArgs{}
	}
	if t.Kind == "" {
		t.Kind = domain.KAFKA
	}
	return t
}
