package flowx

import (
	"context"
	"fmt"
	"strings"

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
		model.NodeChatGroup:  &chatGroupNodeExecutor{engine: e},
		model.NodeTask:       &taskNodeExecutor{engine: e},
		model.NodeEnd:        &endNodeExecutor{},
	}
}

// applyStepDecision 统一调度应用内核推导决策（保存Skip任务、汇聚等待、办结或派发新待办）
func (e *flowEngine) applyStepDecision(ctx context.Context, session *flowSession, dec kernel.StepDecision, batchCode int, operator string) error {
	inst := session.Instance()

	// 1. 持久化未命中分支的 TaskSkipped 占位记录，供时间线渲染与汇聚感知
	if len(dec.SkippedNodes) > 0 {
		skippedTasks := lo.FilterMap(dec.SkippedNodes, func(n model.NodeSpec, idx int) (*model.Task, bool) {
			if n.IsGateway() {
				return nil, false // 网关非物理审批节点，绝不生成 Task 记录
			}
			t := model.NewTask(e.idGen.GenTaskID(n.ID, "system_skip", batchCode, idx+1), inst.ID, n, batchCode, "system", model.TaskPending)
			t.Skip("分支条件未满足，系统自动跳过")
			return t, true
		})
		if len(skippedTasks) > 0 {
			if err := session.SaveTasks(ctx, skippedTasks); err != nil {
				return fmt.Errorf("持久化跳过占位任务失败: %w", err)
			}

			for _, t := range skippedTasks {
				e.publisher.Publish(ctx, NewTaskEvent(EventTaskSkipped, inst, nil, t, "system", t.Comment))
			}
		}
	}

	// 2. 联动更新排他汇聚竞争胜出后被跳过的兄弟分支未决待办
	if len(dec.SkippedTaskIDs) > 0 {
		if err := session.UpdateTasksStatus(ctx, dec.SkippedTaskIDs, model.TaskSkipped); err != nil {
			return fmt.Errorf("批量更新排他汇聚竞争跳过任务失败: %w", err)
		}
		for _, taskID := range dec.SkippedTaskIDs {
			if t, ok := session.GetTask(taskID); ok {
				node := session.Plan().NodesMap[t.NodeID]
				e.publisher.Publish(ctx, NewTaskEvent(EventTaskSkipped, inst, &node, t, "system", "排他分支竞争胜出，系统自动跳过"))
			}
		}
	}

	// 3. 若汇聚网关尚未全部就绪，流程实例保存当前流转上下文，等待其他兄弟分支推进
	if dec.IsBlocked {
		return session.SaveInstance(ctx)
	}

	// 核心断流拦截：若未被汇聚网关阻断，但下游完全无可用就绪节点，判定为分支条件未命中导致的断流
	if len(dec.ReadyNodes) == 0 {
		return ErrNoAvailablePath
	}

	// 4. 统一激活就绪节点（若实例在前置穿透递归中已达终态，直接保存退出，杜绝重复激活）
	if !inst.IsRunning() {
		return session.SaveInstance(ctx)
	}

	if err := e.activateNodes(ctx, session, dec.ReadyNodes, batchCode, operator); err != nil {
		return err
	}

	return session.SaveInstance(ctx)
}

// activateNodes 激活一组就绪节点（面向接口与能力契约分发，消除硬编码判等与上帝 switch）
func (e *flowEngine) activateNodes(ctx context.Context, session *flowSession, nodes []model.NodeSpec, batchCode int, operator string) error {
	inst := session.Instance()

	// 1. 先过滤并执行非结束节点（审批、任务、抄送、群通知等），确保待办与异步挂起任务正确落库
	nonEndNodes := lo.Filter(nodes, func(n model.NodeSpec, _ int) bool {
		return n.Type != model.NodeEnd
	})
	endNodes := lo.Filter(nodes, func(n model.NodeSpec, _ int) bool {
		return n.Type == model.NodeEnd
	})

	for _, node := range nonEndNodes {
		// 若前置节点瞬时穿透已致全单办结，中断后续非终态节点的无效激活
		if !inst.IsRunning() {
			break
		}

		inst.CurrentNode = node.ID

		// 广播节点激活进入事件 (便于抄送知会、群通知及流程点亮追踪)
		e.publisher.Publish(ctx, NewNodeEvent(EventNodeEntered, inst, &node, operator, ""))

		executor, ok := e.executors[node.Type]
		if !ok {
			return fmt.Errorf("%w: 不支持的节点类型 %s", ErrUnsupportedNodeType, node.Type)
		}

		if err := executor.Execute(ctx, session, node, batchCode, operator); err != nil {
			return err
		}
	}

	// 2. 调度到达结束节点：仅当全局会话中不存在任何未完结的在途待办/挂起任务时，流程实例才全单圆满办结
	if len(endNodes) > 0 {
		// 终态幂等防卫：若在前面的穿透节点深层递归中已办结流程，绝不重复触发流程办结与节点进入事件
		if !inst.IsRunning() {
			return nil
		}

		for _, endNode := range endNodes {
			inst.CurrentNode = endNode.ID
			e.publisher.Publish(ctx, NewNodeEvent(EventNodeEntered, inst, &endNode, operator, ""))
		}

		hasInFlightTasks := lo.SomeBy(session.AllTasks(), func(t *model.Task) bool {
			return t.IsPending()
		})
		if !hasInFlightTasks {
			lastEnd := endNodes[len(endNodes)-1]
			inst.Finish(lastEnd.ID)
			e.publisher.Publish(ctx, NewNodeEvent(EventProcessFinished, inst, &lastEnd, operator, ""))
		}
	}

	return nil
}

// resolveAssignees 统一解析节点候选人列表（融合上下文内置占位符展开与同步Hook业务守卫）
func (e *flowEngine) resolveAssignees(ctx context.Context, session *flowSession, node *model.NodeSpec, operator string) ([]string, error) {
	inst := session.Instance()

	// 1. 内置上下文占位符就地展开 (支持 starter / initiator 等自动映射为工单发起人)
	rawAssignees := lo.Map(node.Assignees, func(assignee string, _ int) string {
		trimmed := strings.TrimSpace(assignee)
		switch trimmed {
		case model.AssigneeStarter, model.AssigneeInitiator, "starter", "founder":
			if inst != nil && inst.Starter != "" {
				return inst.Starter
			}
		}
		return trimmed
	})

	assignees := lo.Uniq(lo.Filter(rawAssignees, func(a string, _ int) bool {
		return a != ""
	}))

	// 2. 触发节点声明的同步业务守卫 Hook (如有)
	if len(node.Hooks) == 0 {
		return assignees, nil
	}

	hookCtx := NewHookContext(inst, node, operator, assignees)
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
		x.engine.publisher.Publish(ctx, NewTaskEvent(EventTaskCreated, inst, &node, t, operator, ""))
	}

	return nil
}

// taskNodeExecutor 自动化任务节点执行器 (职责明确：分发执行作业、自动推进状态机)
type taskNodeExecutor struct {
	engine *flowEngine
}

func (x *taskNodeExecutor) Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error {
	worker, ok := x.engine.workers.Get(node.WorkerType)
	if !ok {
		return fmt.Errorf("%w: 未注册的执行器 [%s]", ErrWorkerNotFound, node.WorkerType)
	}

	inst := session.Instance()
	res, workerErr := worker(ctx, &TaskContext{
		Instance:  inst,
		Node:      &node,
		Variables: inst.Variables,
		Attempt:   1,
	})

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

	x.engine.publisher.Publish(ctx, NewTaskEvent(EventTaskPassed, inst, &node, sysTask, "system", "系统自动执行成功"))

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
			x.engine.publisher.Publish(ctx, NewTaskEvent(EventTaskPassed, inst, &node, t, operator, "系统抄送知会"))
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

// chatGroupNodeExecutor 群聊通知节点执行器 (职责明确：记录任务快照、广播群聊通知事件、瞬时穿透推进下一节点)
type chatGroupNodeExecutor struct {
	engine *flowEngine
}

func (x *chatGroupNodeExecutor) Execute(ctx context.Context, session *flowSession, node model.NodeSpec, batchCode int, operator string) error {
	inst := session.Instance()

	// 1. 生成并落库已通过的任务快照，供拓扑高亮推导 (traverser) 与上层通知链路消费
	taskID := x.engine.idGen.GenTaskID(node.ID, "chat_group", batchCode, 1)
	chatTask := model.NewTask(taskID, inst.ID, node, batchCode, "chat_group", model.TaskPending)
	chatTask.Pass("群聊通知自动完成")
	if err := session.SaveTasks(ctx, []*model.Task{chatTask}); err != nil {
		return err
	}

	// 2. 广播群聊通知节点激活与任务通过事件
	x.engine.publisher.Publish(ctx, NewNodeEvent(EventNodeEntered, inst, &node, operator, "群聊通知自动穿透"))
	x.engine.publisher.Publish(ctx, NewTaskEvent(EventTaskPassed, inst, &node, chatTask, operator, "群聊通知自动完成"))

	// 3. 瞬时自动穿透推进：驱动群聊通知节点下一步流转决策
	nextDec, err := session.Plan().ResolveNextStep(node.ID, inst.Variables, session.AllTasks(), batchCode)
	if err != nil {
		return err
	}

	return x.engine.applyStepDecision(ctx, session, nextDec, batchCode, operator)
}
