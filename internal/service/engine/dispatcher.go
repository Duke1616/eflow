package engine

import (
	"context"
	"fmt"
	"sort"
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

func (d *engineDispatcher) GetAutomationTask(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, nodeID string) (domain.TaskRecord, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return domain.TaskRecord{}, err
	}
	return eng.GetAutomationTask(ctx, engineType, instanceID, nodeID)
}

// GetTasksByCurrentNode 获取指定流程实例在特定节点下当前挂起/未完成的任务记录集合
func (d *engineDispatcher) GetTasksByCurrentNode(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, err
	}
	return eng.GetTasksByCurrentNode(ctx, engineType, instanceID, nodeID)
}

// maxPerEngineTodoLimit 单引擎待办任务拉取上限保护（防止异常批量数据撑爆内存）
const maxPerEngineTodoLimit = 500

// ListTodoTasks 并发拉取所有已注册底层引擎的待办任务，并在分发器层完成全局时间排序与精确内存分页
// 解释设计原因：跨异构引擎（如 FlowX 与 EasyFlow）下推各自的分页 offset 会导致分布式伪分页与时间交错乱序。
// 由于个人待办数据体量天然较小（单人处于 Pending 状态的任务通常在几十条以内），
// 采用“带防爆上限的各引擎活跃待办拉取 + 分发器全局时间归并排序 + 内存切片”是业界最优且最稳固的实践。
func (d *engineDispatcher) ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error) {
	d.mu.RLock()
	engines := lo.Values(d.engines)
	d.mu.RUnlock()

	var (
		eg       errgroup.Group
		mu       sync.Mutex
		allTasks []domain.TodoTask
	)

	// 1. 构造面向底层引擎的无偏移行全量拉取请求，附加防爆熔断上限
	engineReq := domain.TodoQueryReq{
		Username:    req.Username,
		ProcessName: req.ProcessName,
		SortByAsc:   req.SortByAsc,
		Offset:      0,
		Limit:       maxPerEngineTodoLimit,
		IsAdmin:     req.IsAdmin,
	}

	for _, eng := range engines {
		e := eng
		eg.Go(func() error {
			tasks, _, err := e.ListTodoTasks(ctx, engineReq)
			if err != nil {
				return err
			}
			mu.Lock()
			allTasks = append(allTasks, tasks...)
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	// 2. 全局严格按任务创建时间进行跨引擎归并排序（保证绝对时间线性）
	sort.SliceStable(allTasks, func(i, j int) bool {
		if req.SortByAsc {
			return allTasks[i].CreateTime.Before(allTasks[j].CreateTime)
		}
		return allTasks[i].CreateTime.After(allTasks[j].CreateTime)
	})

	total := int64(len(allTasks))

	// 3. 内存安全切片，保证前端标准分页不重不漏
	if req.Offset < 0 || req.Offset >= len(allTasks) {
		return []domain.TodoTask{}, total, nil
	}

	end := req.Offset + req.Limit
	if req.Limit <= 0 || end > len(allTasks) {
		end = len(allTasks)
	}

	return allTasks[req.Offset:end], total, nil
}

// ListPendingSteps 基于工单绑定的流程引擎类型精准定向检索当前挂起的待办步骤，绝不向未涉及的引擎盲广播
func (d *engineDispatcher) ListPendingSteps(ctx context.Context, processes []domain.Process) ([]domain.TodoTask, error) {
	if len(processes) == 0 {
		return nil, nil
	}

	// 1. 按工单绑定的引擎类型对流程实例进行精确分组
	procByEngine := lo.GroupBy(processes, func(p domain.Process) domain.EngineType {
		return p.ResolveEngineType()
	})

	var (
		eg       errgroup.Group
		mu       sync.Mutex
		allSteps []domain.TodoTask
	)

	// 2. 仅对实际包含实例的引擎定向并发调用，未涉及的引擎 0 开销
	for engType, procs := range procByEngine {
		targetEngine, err := d.getEngine(engType)
		if err != nil {
			return nil, err
		}
		targetProcs := procs
		eg.Go(func() error {
			steps, err := targetEngine.ListPendingSteps(ctx, targetProcs)
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
