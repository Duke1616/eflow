package flowx

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// loadOrDeployDefinition 加载已发布流程定义，或自动部署请求中附带的临时流程快照
func (e *flowEngine) loadOrDeployDefinition(ctx context.Context, req StartReq) (model.ProcessDefinition, error) {
	if req.Def.Key != "" {
		def, err := e.Deploy(ctx, req.Def)
		if err != nil {
			return model.ProcessDefinition{}, fmt.Errorf("部署流程定义失败: %w", err)
		}
		return def, nil
	}

	version := req.Version
	if version <= 0 {
		version = 1
	}
	plan, err := e.planCache.GetOrLoad(ctx, req.Key, version, func(c context.Context) (model.ProcessDefinition, error) {
		return e.storage.GetDefinition(c, req.Key, version)
	})
	if err != nil {
		return model.ProcessDefinition{}, fmt.Errorf("获取流程定义失败: %w", err)
	}
	return plan.Def, nil
}

// loadAndValidateTask 加载待办任务并执行客观物理运行态校验 (防并发重复处理、防终态非法流转)
// 架构遵循机制与策略分离原则。身份权限(代办/委托/管理/PBAC)由业务层或洋葱中间件决策，内核仅验证物理状态
func (e *flowEngine) loadAndValidateTask(ctx context.Context, taskID string) (*model.Task, *model.Instance, error) {
	task, err := e.storage.GetTaskForUpdate(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if !task.IsPending() {
		return nil, nil, ErrTaskNotPending
	}

	inst, err := e.storage.GetInstanceForUpdate(ctx, task.InstanceID)
	if err != nil {
		return nil, nil, err
	}
	if !inst.IsRunning() {
		return nil, nil, ErrInstanceNotRunning
	}
	return task, inst, nil
}

// loadPlan 从缓存或底层存储加载流程执行拓扑
func (e *flowEngine) loadPlan(ctx context.Context, inst *model.Instance) (*kernel.ExecutionPlan, error) {
	return e.planCache.GetOrLoad(ctx, inst.ProcKey, inst.Version, func(c context.Context) (model.ProcessDefinition, error) {
		return e.storage.GetDefinition(c, inst.ProcKey, inst.Version)
	})
}

// newSessionForInstance 组装单次流转事务的工作单元会话 (预加载实例任务快照)
func (e *flowEngine) newSessionForInstance(ctx context.Context, inst *model.Instance) (*flowSession, error) {
	plan, err := e.loadPlan(ctx, inst)
	if err != nil {
		return nil, err
	}
	allTasks, err := e.storage.ListTasksByInstance(ctx, inst.ID)
	if err != nil {
		return nil, err
	}
	return newFlowSession(e.storage, inst, plan, allTasks), nil
}
