package flowx

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// nodeExecutor 定义各类型节点激活时的行为契约 (面向接口多态设计，消除上帝 switch-case)
type nodeExecutor interface {
	Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error
}

// newDefaultExecutors 初始化标准内置节点执行器注册表
func newDefaultExecutors(e *flowEngine) map[model.NodeType]nodeExecutor {
	return map[model.NodeType]nodeExecutor{
		model.NodeApproval:   &approvalNodeExecutor{engine: e},
		model.NodeCarbonCopy: &carbonCopyNodeExecutor{engine: e},
		model.NodeTask:       &taskNodeExecutor{engine: e},
		model.NodeEnd:        &endNodeExecutor{},
	}
}

// applyStepDecision 统一调度应用内核推导决策（保存Skip任务、汇聚等待、办结或派发新待办）
func (e *flowEngine) applyStepDecision(ctx context.Context, session *flowSession, dec *kernel.StepDecision, batchCode int, operator string) error {
	inst := session.Instance()

	// 1. 持久化未命中分支的 TaskSkipped 占位记录，供时间线渲染与汇聚感知
	if len(dec.SkippedNodes) > 0 {
		skippedTasks := lo.Map(dec.SkippedNodes, func(n model.NodeSpec, idx int) *model.Task {
			t := model.NewTask(e.idGen.GenTaskID(n.ID, "system_skip", batchCode, idx+1), inst.ID, n, batchCode, "system", model.TaskPending)
			t.Skip("分支条件未满足，系统自动跳过")
			return t
		})
		_ = session.SaveTasks(ctx, skippedTasks)

		for _, t := range skippedTasks {
			e.eventBus.Publish(ctx, NewTaskEvent(EventTaskSkipped, inst, nil, t, "system", t.Comment))
		}
	}

	// 2. 若汇聚网关尚未全部就绪，流程实例保存当前流转上下文，等待其他兄弟分支推进
	if dec.IsBlocked {
		return session.SaveInstance(ctx)
	}

	// 核心断流拦截：若未被汇聚网关阻断，但下游完全无可用就绪节点，判定为分支条件未命中导致的断流
	if len(dec.ReadyNodes) == 0 {
		return ErrNoAvailablePath
	}

	// 3. 统一激活就绪节点（按节点类型多态分发：办结、自动执行或派发待办任务）
	if err := e.activateNodes(ctx, session, dec.ReadyNodes, batchCode, operator); err != nil {
		return err
	}

	return session.SaveInstance(ctx)
}

// activateNodes 激活一组就绪节点（面向接口与能力契约分发，消除硬编码判等与上帝 switch）
func (e *flowEngine) activateNodes(ctx context.Context, session *flowSession, nodes []model.NodeSpec, batchCode int, operator string) error {
	inst := session.Instance()

	// 判定本轮决策是否包含未办结的活跃等待态节点 (通过能力契约判断，彻底消除硬编码)
	hasActiveNodes := lo.SomeBy(nodes, func(n model.NodeSpec) bool {
		return n.IsWaitState()
	})

	for _, node := range nodes {
		inst.CurrentNode = node.ID

		// 若当前节点为 End 且当前流转中已无任何等待态节点，全单圆满办结
		if node.Type == model.NodeEnd {
			if !hasActiveNodes {
				inst.Finish(node.ID)
				e.eventBus.Publish(ctx, NewNodeEvent(EventProcessFinished, inst, &node, operator, ""))
			}
			continue
		}

		executor, ok := e.executors[node.Type]
		if !ok {
			return fmt.Errorf("%w: 不支持的节点类型 %s", ErrUnsupportedNodeType, node.Type)
		}

		if err := executor.Execute(ctx, session, node, batchCode, operator); err != nil {
			return err
		}
	}

	return nil
}

// resolveAssignees 统一解析节点候选人列表（融合静态配置与同步Hook动态算人）
func (e *flowEngine) resolveAssignees(ctx context.Context, session *flowSession, node *model.NodeSpec, operator string) ([]string, error) {
	assignees := lo.Uniq(append([]string{}, node.Assignees...))
	if len(node.Hooks) == 0 {
		return assignees, nil
	}

	inst := session.Instance()
	hookCtx := NewHookContext(inst, node, operator, inst.Variables, assignees)
	if err := e.hooks.Execute(ctx, node.Hooks, hookCtx); err != nil {
		return nil, err
	}

	return hookCtx.Assignees(), nil
}

// approvalNodeExecutor 人工审批节点执行器 (职责明确：算人、生成待办任务、广播创建事件)
type approvalNodeExecutor struct {
	engine *flowEngine
}

func (x *approvalNodeExecutor) Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error {
	assignees, err := x.engine.resolveAssignees(ctx, session, &node, operator)
	if err != nil {
		return err
	}

	// 严格校验：审批节点必须存在合法处理人，防止流程静默悬挂无主
	if len(assignees) == 0 {
		return fmt.Errorf("%w: 审批节点 [%s](%s) 未指定或未解析到有效审批人", ErrEmptyAssignees, node.ID, node.Name)
	}

	inst := session.Instance()

	// 生成待办审批任务
	nodeTasks := lo.Map(assignees, func(assignee string, idx int) *model.Task {
		return model.NewTask(x.engine.idGen.GenTaskID(node.ID, assignee, batchCode, idx+1), inst.ID, node, batchCode, assignee, model.TaskPending)
	})

	if err := session.SaveTasks(ctx, nodeTasks); err != nil {
		return err
	}

	// 广播新任务生成事件
	for _, t := range nodeTasks {
		x.engine.eventBus.Publish(ctx, NewTaskEvent(EventTaskCreated, inst, &node, t, operator, ""))
	}

	return nil
}

// taskNodeExecutor 自动化任务节点执行器 (职责明确：弹性重试执行作业、自动推进状态机)
type taskNodeExecutor struct {
	engine *flowEngine
}

func (x *taskNodeExecutor) Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error {
	worker, ok := x.engine.workers.Get(node.WorkerType)
	if !ok {
		return fmt.Errorf("%w: 未注册的执行器 [%s]", ErrWorkerNotFound, node.WorkerType)
	}

	inst := session.Instance()
	var (
		res       *TaskResult
		workerErr error
	)

	// 带退避重试执行
	attempts := 1
	if node.RetryPolicy != nil && node.RetryPolicy.MaxAttempts > 1 {
		attempts = node.RetryPolicy.MaxAttempts
	}

	interval := 100 * time.Millisecond
	if node.RetryPolicy != nil && node.RetryPolicy.InitialInterval > 0 {
		interval = node.RetryPolicy.InitialInterval
	}

	for i := 1; i <= attempts; i++ {
		res, workerErr = worker(ctx, &TaskContext{
			Instance:  inst,
			Node:      &node,
			Variables: inst.Variables,
			Attempt:   i,
		})
		if workerErr == nil {
			break
		}
		if i < attempts {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(interval):
			}
		}
	}

	if workerErr != nil {
		return fmt.Errorf("自动化任务 [%s] 执行失败: %w", node.ID, workerErr)
	}

	taskID := x.engine.idGen.GenTaskID(node.ID, "system", batchCode, 1)

	// 异步挂起模式：Worker 返回 nil 则派发 TaskPending 任务保持挂起，等待外部回调
	if res == nil {
		sysTask := model.NewTask(taskID, inst.ID, node, batchCode, "system", model.TaskPending)
		return session.SaveTasks(ctx, []*model.Task{sysTask})
	}

	// 同步完成：保存已通过任务记录
	sysTask := model.NewTask(taskID, inst.ID, node, batchCode, "system", model.TaskPending)
	sysTask.Pass("系统自动执行成功")
	if err := session.SaveTasks(ctx, []*model.Task{sysTask}); err != nil {
		return err
	}

	x.engine.eventBus.Publish(ctx, NewTaskEvent(EventTaskPassed, inst, &node, sysTask, "system", "系统自动执行成功"))

	// 合并自动化任务产出的增量表单变量
	if len(res.Variables) > 0 {
		inst.MergeVariables(res.Variables)
	}

	// 自动驱动下一步流转决策
	nextDec, err := session.Plan().ResolveNextStep(node.ID, inst.Variables, session.AllTasks(), batchCode)
	if err != nil {
		return err
	}

	return x.engine.applyStepDecision(ctx, session, nextDec, batchCode, "system")
}

// carbonCopyNodeExecutor 抄送节点执行器 (职责明确：生成已办结抄送记录、瞬时穿透推进下一节点)
type carbonCopyNodeExecutor struct {
	engine *flowEngine
}

func (x *carbonCopyNodeExecutor) Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error {
	assignees, err := x.engine.resolveAssignees(ctx, session, &node, operator)
	if err != nil {
		return err
	}

	inst := session.Instance()

	if len(assignees) > 0 {
		ccTasks := lo.Map(assignees, func(assignee string, idx int) *model.Task {
			t := model.NewTask(x.engine.idGen.GenTaskID(node.ID, assignee, batchCode, idx+1), inst.ID, node, batchCode, assignee, model.TaskPending)
			t.Pass("系统抄送知会")
			return t
		})
		if err := session.SaveTasks(ctx, ccTasks); err != nil {
			return err
		}

		for _, t := range ccTasks {
			x.engine.eventBus.Publish(ctx, NewTaskEvent(EventTaskPassed, inst, &node, t, operator, "系统抄送知会"))
		}
	}

	// 瞬时自动穿透推进：驱动抄送节点下一步流转决策
	nextDec, err := session.Plan().ResolveNextStep(node.ID, inst.Variables, session.AllTasks(), batchCode)
	if err != nil {
		return err
	}

	return x.engine.applyStepDecision(ctx, session, nextDec, batchCode, operator)
}

// endNodeExecutor 办结节点执行器
type endNodeExecutor struct{}

func (x *endNodeExecutor) Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error {
	return nil
}
