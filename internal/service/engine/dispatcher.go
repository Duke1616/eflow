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

// engineDispatcher 驱动注册表在构造期一次性装配完成后即不可变，且运行期不提供注册入口，
// 因此并发读取无需加锁
type engineDispatcher struct {
	drivers map[domain.EngineType]IDriver
	logger  *elog.Component
}

// NewEngineDispatcher 基于开闭原则（OCP）装配多引擎驱动，对外统一以 IEngine 门面暴露
func NewEngineDispatcher(drivers ...IDriver) IEngine {
	d := &engineDispatcher{
		drivers: make(map[domain.EngineType]IDriver, len(drivers)),
		logger:  elog.DefaultLogger,
	}
	for _, drv := range drivers {
		if drv != nil {
			d.drivers[drv.EngineType()] = drv
		}
	}
	return d
}

// getEngine 根据显式引擎类型获取注册的驱动实例
func (d *engineDispatcher) getEngine(engineType domain.EngineType) (IDriver, error) {
	drv, ok := d.drivers[engineType]
	if !ok {
		return nil, fmt.Errorf("%w: 未注册对应的流程引擎驱动 [engine_type=%s]", ErrEngineNotFound, engineType)
	}
	return drv, nil
}

func (d *engineDispatcher) Deploy(ctx context.Context, engineType domain.EngineType, rawDef any) (domain.DeployInfo, error) {
	drv, err := d.getEngine(engineType)
	if err != nil {
		return domain.DeployInfo{}, err
	}
	return drv.Deploy(ctx, rawDef)
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

	return eng.GetTaskContext(ctx, taskID)
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

	timeline, err := eng.GetTimeline(ctx, instanceID)
	if err != nil {
		// 跨引擎自适应容错：历史工单在跨引擎切换后，尝试使用备用引擎寻址
		fallbackType := lo.Ternary(engineType == domain.EngineTypeFlowx, domain.EngineTypeEasyFlow, domain.EngineTypeFlowx)
		if fallbackEng, fErr := d.getEngine(fallbackType); fErr == nil {
			if fallbackTimeline, fbErr := fallbackEng.GetTimeline(ctx, instanceID); fbErr == nil {
				return fallbackTimeline, nil
			}
		}
	}
	return timeline, err
}

func (d *engineDispatcher) GetInstanceExecutionView(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) (domain.InstanceExecutionView, error) {
	if instanceID.IsZero() {
		return domain.InstanceExecutionView{}, fmt.Errorf("%w: 流程实例 ID 不能为空", ErrInvalidParameter)
	}

	eng, err := d.getEngine(engineType)
	if err != nil {
		return domain.InstanceExecutionView{}, err
	}

	view, err := eng.GetInstanceExecutionView(ctx, instanceID)
	if err != nil {
		// 跨引擎自适应容错：当指定引擎未找到实例时（如工作流后来切换了引擎），自动尝试备选引擎
		fallbackType := lo.Ternary(engineType == domain.EngineTypeFlowx, domain.EngineTypeEasyFlow, domain.EngineTypeFlowx)
		if fallbackEng, fErr := d.getEngine(fallbackType); fErr == nil {
			if fallbackView, fbErr := fallbackEng.GetInstanceExecutionView(ctx, instanceID); fbErr == nil {
				return fallbackView, nil
			}
		}
	}
	return view, err
}

func (d *engineDispatcher) GetAutomationTask(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, nodeID string) (domain.TaskRecord, error) {
	eng, err := d.getEngine(engineType)
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
	return eng.GetTasksByCurrentNode(ctx, instanceID, nodeID)
}

// ListTodoTasks 跨引擎归并分页：各引擎按创建时间有序返回各自的前 K 条（K = offset + limit），
// 分发器归并后切片，total 取各引擎真实计数之和。
// 设计依据：全局前 K 条必然包含于「各引擎各自前 K 条」的并集——任一任务在其所属引擎内的排名
// 不会低于它的全局排名，因此无需全量拉取即可保证分页不重不漏，内存占用由请求页深决定而非数据总量。
// 严禁把 offset 直接下推给各引擎：那会得到分布式伪分页与跨引擎时间交错乱序。
func (d *engineDispatcher) ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error) {
	engines := lo.Values(d.drivers)

	// limit <= 0 语义为不限条数，此时必须全量拉取才能正确归并。
	// 下推 -1 而非 0：GORM 的 Limit(0) 会生成 LIMIT 0 从而返回零行
	fetchLimit := -1
	if req.Limit > 0 {
		fetchLimit = max(req.Offset, 0) + req.Limit
	}

	engineReq := domain.TodoQueryReq{
		Username:    req.Username,
		ProcessName: req.ProcessName,
		SortByAsc:   req.SortByAsc,
		Offset:      0,
		Limit:       fetchLimit,
		IsAdmin:     req.IsAdmin,
	}

	var (
		eg       errgroup.Group
		mu       sync.Mutex
		allTasks []domain.TodoTask
		total    int64
	)

	for _, eng := range engines {
		eg.Go(func() error {
			tasks, count, err := eng.ListTodoTasks(ctx, engineReq)
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

	// 全局按创建时间归并；时间相同时再以 TaskID 定序。
	// 缺少确定性 tiebreak 时，并发 append 的到达顺序会让同时间戳任务在相邻两次翻页请求间排序漂移，造成重复或遗漏
	sort.SliceStable(allTasks, func(i, j int) bool {
		if !allTasks[i].CreateTime.Equal(allTasks[j].CreateTime) {
			if req.SortByAsc {
				return allTasks[i].CreateTime.Before(allTasks[j].CreateTime)
			}
			return allTasks[i].CreateTime.After(allTasks[j].CreateTime)
		}
		return allTasks[i].TaskID < allTasks[j].TaskID
	})

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
		eg.Go(func() error {
			steps, err := targetEngine.ListPendingSteps(ctx, procs)
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
	return eng.GetTaskRecords(ctx, instanceID, offset, limit)
}

// GetTaskTimeline 分页获取指定流程实例的聚合时间线事件
func (d *engineDispatcher) GetTaskTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, 0, err
	}
	return eng.GetTaskTimeline(ctx, instanceID, offset, limit)
}

// GetRejectTargets 获取指定任务节点在当前流转状态下允许驳回的目标节点候选列表
func (d *engineDispatcher) GetRejectTargets(ctx context.Context, engineType domain.EngineType, taskID domain.ID) ([]domain.RejectTarget, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, err
	}
	return eng.GetRejectTargets(ctx, taskID)
}

// GetActiveReject 获取流程实例当前正处于待处理/待响应状态的最新驳回上下文（若无驳回或已重新推进闭环，返回 nil）
func (d *engineDispatcher) GetActiveReject(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) (*domain.RejectSnapshot, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, err
	}
	return eng.GetActiveReject(ctx, instanceID)
}

// GetPriorComments 精确获取流程实例最近 limit 条人工裁决意见与意见总数，按办结时间倒序
func (d *engineDispatcher) GetPriorComments(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, limit int) ([]domain.PriorComment, int64, error) {
	eng, err := d.getEngine(engineType)
	if err != nil {
		return nil, 0, err
	}
	return eng.GetPriorComments(ctx, instanceID, limit)
}
