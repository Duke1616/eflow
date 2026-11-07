package engine

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/gotomicro/ego/core/elog"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

type engineDispatcher struct {
	mu      sync.RWMutex
	engines map[domain.EngineType]IEngine
	logger  *elog.Component
}

// NewEngineDispatcher 基于开闭原则（OCP）装配多引擎驱动，对外统一以 IEngine 契约暴露
func NewEngineDispatcher(engines ...IEngine) IEngine {
	d := &engineDispatcher{
		engines: make(map[domain.EngineType]IEngine),
		logger:  elog.DefaultLogger,
	}
	for _, eng := range engines {
		if eng != nil {
			d.register(eng)
		}
	}
	return d
}

func (d *engineDispatcher) register(eng IEngine) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.engines[eng.EngineType()] = eng
}

func (d *engineDispatcher) EngineType() domain.EngineType {
	return ""
}

// getEngine 根据显式引擎类型获取注册的驱动实例
func (d *engineDispatcher) getEngine(engineType domain.EngineType) (IEngine, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.findEngineLocked(engineType)
}

// findEngineLocked 在已持有读锁的前提下查找引擎实例
func (d *engineDispatcher) findEngineLocked(engineType domain.EngineType) (IEngine, error) {
	eng, ok := d.engines[engineType]
	if !ok {
		return nil, fmt.Errorf("%w: 未注册对应的流程引擎驱动 [engine_type=%s]", ErrEngineNotFound, engineType)
	}
	return eng, nil
}

func (d *engineDispatcher) Deploy(ctx context.Context, engineType domain.EngineType, rawDef any) (domain.DeployInfo, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return domain.DeployInfo{}, err
	}
	return eng.Deploy(ctx, engineType, rawDef)
}

func (d *engineDispatcher) Start(ctx context.Context, req domain.ProcessLaunchReq) (domain.ProcessLaunchResult, error) {
	if err := req.Validate(); err != nil {
		return domain.ProcessLaunchResult{}, fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	eng, err := d.getEngine(req.EngineType)
	if err != nil {
		return domain.ProcessLaunchResult{}, err
	}

	start := time.Now()
	res, err := eng.Start(ctx, req)
	cost := time.Since(start)

	if err != nil {
		d.logger.Error("流程实例启动失败",
			elog.FieldErr(err),
			elog.Any("engine_type", req.EngineType),
			elog.Int64("ticket_id", req.TicketId),
			elog.Duration("cost", cost),
		)
	} else {
		d.logger.Info("流程实例启动成功",
			elog.Any("engine_type", res.EngineType),
			elog.Int64("ticket_id", req.TicketId),
			elog.String("instance_id", res.InstanceID.String()),
			elog.Duration("cost", cost),
		)
	}
	return res, err
}

func (d *engineDispatcher) GetTaskContext(ctx context.Context, engineType domain.EngineType, taskID domain.ID) (domain.TaskContext, error) {
	if taskID.IsZero() {
		return domain.TaskContext{}, fmt.Errorf("%w: 任务 ID 不能为空", ErrInvalidParameter)
	}

	eng, err := d.getEngine(engineType)
	if err != nil {
		return domain.TaskContext{}, err
	}
	return eng.GetTaskContext(ctx, engineType, taskID)
}

func (d *engineDispatcher) Pass(ctx context.Context, req domain.TaskPassReq) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	eng, err := d.getEngine(req.EngineType)
	if err != nil {
		return err
	}
	return eng.Pass(ctx, req)
}

func (d *engineDispatcher) Reject(ctx context.Context, req domain.TaskRejectReq) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	eng, err := d.getEngine(req.EngineType)
	if err != nil {
		return err
	}
	return eng.Reject(ctx, req)
}

func (d *engineDispatcher) Transfer(ctx context.Context, req domain.TaskTransferReq) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	eng, err := d.getEngine(req.EngineType)
	if err != nil {
		return err
	}
	return eng.Transfer(ctx, req)
}

func (d *engineDispatcher) Revoke(ctx context.Context, req domain.RevokeReq) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	eng, err := d.getEngine(req.EngineType)
	if err != nil {
		return err
	}
	return eng.Revoke(ctx, req)
}

func (d *engineDispatcher) GetTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) ([]domain.TimelineNode, error) {
	if instanceID.IsZero() {
		return nil, fmt.Errorf("%w: 流程实例 ID 不能为空", ErrInvalidParameter)
	}

	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, err
	}
	return eng.GetTimeline(ctx, engineType, instanceID)
}

func (d *engineDispatcher) GetInstanceExecutionView(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, status uint8) (domain.InstanceExecutionView, error) {
	if instanceID.IsZero() {
		return domain.InstanceExecutionView{}, fmt.Errorf("%w: 流程实例 ID 不能为空", ErrInvalidParameter)
	}

	eng, err := d.getEngine(engineType)
	if err != nil {
		return domain.InstanceExecutionView{}, err
	}
	return eng.GetInstanceExecutionView(ctx, engineType, instanceID, status)
}

func (d *engineDispatcher) GetAutomationTask(ctx context.Context, instanceID domain.ID, nodeID string) (domain.TaskRecord, error) {
	eng, err := d.getEngine(domain.EngineTypeEasyFlow)
	if err != nil {
		return domain.TaskRecord{}, err
	}
	return eng.GetAutomationTask(ctx, instanceID, nodeID)
}

// GetTasksByCurrentNode 获取指定流程实例在特定节点下当前挂起/未完成的任务记录集合
func (d *engineDispatcher) GetTasksByCurrentNode(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, err
	}
	return eng.GetTasksByCurrentNode(ctx, engineType, instanceID, nodeID)
}

// ListTodoTasks 并发聚合所有已注册底层引擎的待办任务，向业务层屏蔽底层异构细节（多态聚合）
func (d *engineDispatcher) ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error) {
	d.mu.RLock()
	engines := lo.Values(d.engines)
	d.mu.RUnlock()

	var (
		eg       errgroup.Group
		mu       sync.Mutex
		allTasks []domain.TodoTask
		total    int64
	)

	for _, eng := range engines {
		e := eng
		eg.Go(func() error {
			tasks, count, err := e.ListTodoTasks(ctx, req)
			if err != nil {
				return err
			}
			mu.Lock()
			allTasks = append(allTasks, tasks...)
			total += count
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	return allTasks, total, nil
}

// ListPendingSteps 跨引擎批量检索指定流程实例列表中当前挂起的待办步骤
func (d *engineDispatcher) ListPendingSteps(ctx context.Context, instanceIDs []domain.ID) ([]domain.TodoTask, error) {
	if len(instanceIDs) == 0 {
		return nil, nil
	}

	d.mu.RLock()
	engines := lo.Values(d.engines)
	d.mu.RUnlock()

	var (
		eg       errgroup.Group
		mu       sync.Mutex
		allSteps []domain.TodoTask
	)

	for _, eng := range engines {
		e := eng
		eg.Go(func() error {
			steps, err := e.ListPendingSteps(ctx, instanceIDs)
			if err != nil {
				return err
			}
			if len(steps) > 0 {
				mu.Lock()
				allSteps = append(allSteps, steps...)
				mu.Unlock()
			}
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	return allSteps, nil
}

// GetTaskRecords 分页获取指定流程实例的流转执行记录
func (d *engineDispatcher) GetTaskRecords(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, 0, err
	}
	return eng.GetTaskRecords(ctx, engineType, instanceID, offset, limit)
}

// GetTaskTimeline 分页获取指定流程实例的聚合时间线事件
func (d *engineDispatcher) GetTaskTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, 0, err
	}
	return eng.GetTaskTimeline(ctx, engineType, instanceID, offset, limit)
}
