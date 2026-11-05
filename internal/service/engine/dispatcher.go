package engine

//go:generate /Users/luankz/go/bin/mockgen -source=./dispatcher.go -package=enginemocks -destination=./mocks/dispatcher.mock.go IEngineDispatcher

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/gotomicro/ego/core/elog"
)

// IProcessLauncher 面向工单创建消费者（ProcessEventConsumer）的最小角色接口
type IProcessLauncher interface {
	Launch(ctx context.Context, req ProcessLaunchReq) (ProcessLaunchResult, error)
}

// IEngineDispatcher 统一多引擎调度服务门面接口（智能路由与调度中心，对业务层彻底屏蔽底层引擎差异）
type IEngineDispatcher interface {
	IProcessLauncher
	Deploy(ctx context.Context, engineType domain.EngineType, rawDef any) (DeployInfo, error)
	Pass(ctx context.Context, req TaskPassReq) error
	Reject(ctx context.Context, req TaskRejectReq) error
	Revoke(ctx context.Context, instanceID ID, force bool, operator string) error
	GetTaskContext(ctx context.Context, taskID ID) (TaskContext, error)
	GetTimeline(ctx context.Context, instanceID ID) ([]TimelineNode, error)
	GetInstanceExecutionView(ctx context.Context, instanceID ID, status uint8) (InstanceExecutionView, error)
	GetAdapter(engineType domain.EngineType) (IEngineAdapter, error)
}

type engineDispatcher struct {
	mu       sync.RWMutex
	adapters map[domain.EngineType]IEngineAdapter
	logger   *elog.Component
}

// NewEngineDispatcher 基于开闭原则（OCP）动态装配多引擎驱动适配器
func NewEngineDispatcher(adapters ...IEngineAdapter) IEngineDispatcher {
	d := &engineDispatcher{
		adapters: make(map[domain.EngineType]IEngineAdapter),
		logger:   elog.DefaultLogger,
	}
	for _, adapter := range adapters {
		if adapter != nil {
			d.register(adapter)
		}
	}
	return d
}

func (d *engineDispatcher) register(adapter IEngineAdapter) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.adapters[adapter.EngineType()] = adapter
}

func (d *engineDispatcher) GetAdapter(engineType domain.EngineType) (IEngineAdapter, error) {
	d.mu.RLock()
	adapter, ok := d.adapters[engineType]
	d.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: 未注册对应的流程引擎适配器 [engine_type=%s]", ErrEngineNotFound, engineType)
	}
	return adapter, nil
}

// taskAdapter 根据任务 ID 确定性直接路由到底层引擎适配器（消灭中间类型转换与重复查表）
func (d *engineDispatcher) taskAdapter(taskID ID) (IEngineAdapter, error) {
	if taskID.IsZero() {
		return nil, fmt.Errorf("%w: 任务 ID 不能为空", ErrInvalidParameter)
	}
	idStr := taskID.String()
	d.mu.RLock()
	defer d.mu.RUnlock()

	if strings.HasPrefix(idStr, "task_") {
		if adapter, ok := d.adapters[domain.EngineTypeFlowx]; ok {
			return adapter, nil
		}
		return nil, fmt.Errorf("%w: flowx 引擎驱动未注册", ErrEngineNotFound)
	}

	if _, err := strconv.Atoi(idStr); err == nil {
		if adapter, ok := d.adapters[domain.EngineTypeEasyFlow]; ok {
			return adapter, nil
		}
		return nil, fmt.Errorf("%w: easyflow 引擎驱动未注册", ErrEngineNotFound)
	}

	return nil, fmt.Errorf("%w: 无法识别任务标识特征 [taskId=%s]", ErrTaskNotFound, idStr)
}

// instanceAdapter 根据流程实例 ID 确定性直接路由到底层引擎适配器
func (d *engineDispatcher) instanceAdapter(instanceID ID) (IEngineAdapter, error) {
	if instanceID.IsZero() {
		return nil, fmt.Errorf("%w: 流程实例 ID 不能为空", ErrInvalidParameter)
	}
	idStr := instanceID.String()
	d.mu.RLock()
	defer d.mu.RUnlock()

	if strings.HasPrefix(idStr, "inst_") {
		if adapter, ok := d.adapters[domain.EngineTypeFlowx]; ok {
			return adapter, nil
		}
		return nil, fmt.Errorf("%w: flowx 引擎驱动未注册", ErrEngineNotFound)
	}

	if _, err := strconv.Atoi(idStr); err == nil {
		if adapter, ok := d.adapters[domain.EngineTypeEasyFlow]; ok {
			return adapter, nil
		}
		return nil, fmt.Errorf("%w: easyflow 引擎驱动未注册", ErrEngineNotFound)
	}

	return nil, fmt.Errorf("%w: 无法识别流程实例标识特征 [instanceId=%s]", ErrInstanceNotFound, idStr)
}

func (d *engineDispatcher) Deploy(ctx context.Context, engineType domain.EngineType, rawDef any) (DeployInfo, error) {
	adapter, err := d.GetAdapter(engineType)
	if err != nil {
		return DeployInfo{}, err
	}
	return adapter.Deploy(ctx, rawDef)
}

func (d *engineDispatcher) Launch(ctx context.Context, req ProcessLaunchReq) (res ProcessLaunchResult, err error) {
	if err = req.Validate(); err != nil {
		return ProcessLaunchResult{}, fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	engineType := req.EngineType
	if engineType == "" {
		engineType = domain.EngineTypeEasyFlow
	}

	adapter, err := d.GetAdapter(engineType)
	if err != nil {
		return ProcessLaunchResult{}, err
	}

	start := time.Now()
	res, err = adapter.Start(ctx, req)
	cost := time.Since(start)

	if err != nil {
		d.logger.Error("流程实例启动失败",
			elog.FieldErr(err),
			elog.Any("engine_type", engineType),
			elog.Int64("ticket_id", req.TicketId),
			elog.Duration("cost", cost),
		)
	} else {
		d.logger.Info("流程实例启动成功",
			elog.Any("engine_type", engineType),
			elog.Int64("ticket_id", req.TicketId),
			elog.Int("instance_id", res.InstanceId),
			elog.String("instance_key", res.InstanceKey),
			elog.Duration("cost", cost),
		)
	}
	return res, err
}

func (d *engineDispatcher) GetTaskContext(ctx context.Context, taskID ID) (TaskContext, error) {
	adapter, err := d.taskAdapter(taskID)
	if err != nil {
		return TaskContext{}, err
	}
	return adapter.GetTaskContext(ctx, taskID)
}

func (d *engineDispatcher) Pass(ctx context.Context, req TaskPassReq) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	adapter, err := d.taskAdapter(req.TaskId)
	if err != nil {
		return err
	}
	return adapter.Pass(ctx, req)
}

func (d *engineDispatcher) Reject(ctx context.Context, req TaskRejectReq) error {
	if err := req.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidParameter, err)
	}

	adapter, err := d.taskAdapter(req.TaskId)
	if err != nil {
		return err
	}
	return adapter.Reject(ctx, req)
}

func (d *engineDispatcher) Revoke(ctx context.Context, instanceID ID, force bool, operator string) error {
	adapter, err := d.instanceAdapter(instanceID)
	if err != nil {
		return err
	}
	return adapter.Revoke(ctx, instanceID, force, operator)
}

func (d *engineDispatcher) GetTimeline(ctx context.Context, instanceID ID) ([]TimelineNode, error) {
	adapter, err := d.instanceAdapter(instanceID)
	if err != nil {
		return nil, err
	}
	return adapter.GetTimeline(ctx, instanceID)
}

func (d *engineDispatcher) GetInstanceExecutionView(ctx context.Context, instanceID ID, status uint8) (InstanceExecutionView, error) {
	adapter, err := d.instanceAdapter(instanceID)
	if err != nil {
		return InstanceExecutionView{}, err
	}
	return adapter.GetInstanceExecutionView(ctx, instanceID, status)
}
