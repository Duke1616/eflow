package flowx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/idgen"
	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	"github.com/Duke1616/eflow/pkg/flowx/middleware"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
	"github.com/samber/lo"
)

// StartReq 流程实例发起请求
type StartReq struct {
	// 预分配流程实例唯一标识（可选，若提供则作为实例物理主键）
	InstanceID string
	// 流程图定义快照（可选，若提供则自动部署；主要用于单元测试与快速演练，Value Semantics 优先）
	Def model.ProcessDefinition
	// 流程唯一标识（当 Def 未提供时必填）
	Key string
	// 流程版本（可选，默认 1）
	Version int
	// 发起人账号（必填）
	Starter string
	// 初始业务变量（可选）
	Variables model.Variables
	// 业务关联唯一标识（可选，如工单 ID），透传至生命周期事件中，消除时序竞态
	BusinessKey string
}

// PassReq 审批通过请求
type PassReq struct {
	// 待处理任务物理 ID（必填）
	TaskID string
	// 当前操作人账号（必填，作为流转审计完成人）
	Operator string
	// 审批意见（可选）
	Comment string
	// 增量表单变量合并（可选）
	Variables model.Variables
}

// RejectReq 审批驳回请求
type RejectReq struct {
	// 待处理任务物理 ID（必填）
	TaskID string
	// 当前操作人账号（必填，作为流转审计完成人）
	Operator string
	// 驳回原因（必填）
	Comment string
	// 驳回目标节点 ID（可选；若留空则默认回退到直接上游人工审批节点）
	TargetNodeID string
	// 是否直接终止/作废流程实例（可选；若为 true 则直接终结工单，不进行回退流转）
	Terminate bool
}

// RevokeReq 流程实例撤回请求
type RevokeReq struct {
	// 流程实例物理 ID（必填）
	InstanceID string
	// 当前操作人账号（必填，作为流转审计操作人）
	Operator string
	// 撤回原因（可选）
	Comment string
	// 是否撤回并退回至提单人重新编辑（可选；若为 false 则直接终结工单，置为 Cancelled 终态）
	BackToStarter bool
}

// CompleteTaskReq 外部异步自动化作业执行完成回调请求
type CompleteTaskReq struct {
	// 待处理任务物理 ID（必填）
	TaskID string
	// 执行结果说明（可选，默认 "系统异步作业执行完成"）
	Comment string
	// 自动化任务产出的增量业务变量表（可选）
	Variables model.Variables
}

// IProcessOperator 流程运行时操作视角接口，供业务层（如工单审批 Controller/Service）使用，最小化暴露权限
type IProcessOperator interface {
	// Start 启动流程实例，激活初始节点并派发待办任务
	Start(ctx context.Context, req StartReq) (string, error)
	// Pass 审批人同意通过当前任务，触发状态机推导并流向下一步
	Pass(ctx context.Context, req PassReq) error
	// CompleteTask 外部异步作业/系统执行器完成任务后的回调入口，唤醒挂起并驱动状态机
	CompleteTask(ctx context.Context, req CompleteTaskReq) error
	// Reject 审批人驳回当前任务，原子清理兄弟待办并回退至指定或上游节点（批次号自增）
	Reject(ctx context.Context, req RejectReq) error
	// Revoke 提单人主动撤回在途工单，支持直接作废或退回重新编辑
	Revoke(ctx context.Context, req RevokeReq) error
	// GetTimeline 按执行批次提取流转时间线
	GetTimeline(ctx context.Context, instanceID string) ([]model.TimelineEvent, error)
	// GetTask 根据任务物理 ID 获取单条审批任务详情
	GetTask(ctx context.Context, taskID string) (*model.Task, error)
	// GetInstance 根据流程实例 ID 获取当前运行态实体
	GetInstance(ctx context.Context, instanceID string) (*model.Instance, error)
	// Transfer 将当前审批任务转签/委托指派给新目标用户
	Transfer(ctx context.Context, taskID string, targetUsers []string) error
	// ListPendingTasks 按条件分页查询未办结待办任务，按创建时间有序返回，并附带该条件下的真实总数
	ListPendingTasks(ctx context.Context, query model.PendingTaskQuery) ([]*model.Task, int64, error)
	// ListPendingTasksByInstances 批量拉取多个流程实例当前挂起的未办结审批任务（单次 SQL 批量检索）
	ListPendingTasksByInstances(ctx context.Context, instanceIDs []string) ([]*model.Task, error)
	// ListTasksByInstance 获取指定流程实例下的全部流转与历史任务列表
	ListTasksByInstance(ctx context.Context, instanceID string) ([]*model.Task, error)
	// GetExecutionView 获取流程实例的运行态综合视图（包含节点状态映射、高亮已流转边、待处理人等）
	GetExecutionView(ctx context.Context, instanceID string) (model.ExecutionView, error)
	// GetDefinition 根据流程唯一标识与版本号加载流程图定义快照
	GetDefinition(ctx context.Context, key string, version int) (model.ProcessDefinition, error)
	// GenInstanceID 预分配符合引擎规范的流程实例全局唯一标识
	GenInstanceID(procKey string) string
	// GetRejectTargets 查询指定任务允许驳回的目标节点集合（包含起点与历史已流转审批节点）
	GetRejectTargets(ctx context.Context, taskID string) ([]model.NodeSpec, error)
}

// IProcessAdmin 流程定义部署与组件注册管理视角接口，供初始化、后台管理或运维脚手架使用
type IProcessAdmin interface {
	// Deploy 部署/发布流程定义版本。若 def.Version <= 0 则自动获取已发布最大版本并原子递增 (+1)，返回最终生效的流程定义快照
	Deploy(ctx context.Context, def model.ProcessDefinition) (model.ProcessDefinition, error)
	// RegisterHook 注册节点同步守卫与动态算人 Hook
	RegisterHook(name string, handler HookHandler) Engine
	// AddListener 注册全局生命周期事件监听器 (观察者模式)
	AddListener(listener IEventListener) Engine
	// Subscribe 订阅指定类型的生命周期事件 (函数式语法糖)
	Subscribe(types []EventType, fn func(context.Context, Event) error) Engine
	// RegisterWorker 注册自动化节点执行器
	RegisterWorker(workerType string, worker TaskWorker) Engine
	// Use 注册全局洋葱模型中间件 (如链路追踪、指标打点、安全审计)
	Use(mws ...middleware.Middleware) Engine
}

// Engine 工作流对外统一核心服务接口，组合运行态操作与运维管理能力，并支持物理本地事务闭包
type Engine interface {
	IProcessOperator
	IProcessAdmin
	// DoTx 在物理本地事务闭包内执行工作流与业务编排，异常自动回滚
	DoTx(ctx context.Context, fn func(ctx context.Context, txEngine Engine) error) error
}

type flowEngine struct {
	storage     storage.IStorage
	hooks       *hookRegistry
	eventBus    *EventBus
	publisher   IEventPublisher
	workers     *workerRegistry
	middlewares []middleware.Middleware
	planCache   *kernel.PlanCache
	idGen       idgen.IDGenerator
	executors   map[model.NodeType]nodeExecutor
}

// 编译期严格校验是否完整实现了各角色接口契约
var (
	_ Engine           = (*flowEngine)(nil)
	_ IProcessOperator = (*flowEngine)(nil)
	_ IProcessAdmin    = (*flowEngine)(nil)
)

// Option 引擎配置选项（Functional Options 模式）
type Option func(*flowEngine)

// WithIDGenerator 配置自定义的唯一 ID 生成器
func WithIDGenerator(gen idgen.IDGenerator) Option {
	return func(e *flowEngine) {
		if gen != nil {
			e.idGen = gen
		}
	}
}

func castEngine(engine Engine) (*flowEngine, error) {
	fe, ok := engine.(*flowEngine)
	if !ok {
		return nil, errors.New("流程引擎实例类型断言异常")
	}
	return fe, nil
}

// NewEngine 构造工作流引擎实例
func NewEngine(store storage.IStorage, opts ...Option) Engine {
	engine := &flowEngine{
		storage:     store,
		hooks:       newHookRegistry(),
		eventBus:    newEventBus(),
		workers:     newWorkerRegistry(),
		middlewares: make([]middleware.Middleware, 0),
		planCache:   kernel.NewPlanCache(),
		idGen:       idgen.NewDefaultIDGenerator(),
	}
	engine.publisher = engine.eventBus
	engine.executors = newDefaultExecutors(engine)
	for _, opt := range opts {
		opt(engine)
	}
	return engine
}

// RegisterHook 注册节点同步守卫与动态算人 Hook
func (e *flowEngine) RegisterHook(name string, handler HookHandler) Engine {
	e.hooks.Register(name, handler)
	return e
}

// AddListener 注册全局生命周期事件监听器
func (e *flowEngine) AddListener(listener IEventListener) Engine {
	e.eventBus.AddListener(listener)
	return e
}

// Subscribe 订阅指定类型的生命周期事件
func (e *flowEngine) Subscribe(types []EventType, fn func(context.Context, Event) error) Engine {
	e.eventBus.AddListener(NewEventListener(types, fn))
	return e
}

// RegisterWorker 注册自动化节点执行器
func (e *flowEngine) RegisterWorker(workerType string, worker TaskWorker) Engine {
	e.workers.Register(workerType, worker)
	return e
}

// Use 注册全局洋葱模型中间件
func (e *flowEngine) Use(mws ...middleware.Middleware) Engine {
	e.middlewares = append(e.middlewares, mws...)
	return e
}

// GetTask Deploy 部署流程定义快照（发布期自动执行静态拓扑与语法校验，并预热进入只读执行缓存）
// 若 def.Version <= 0，则由引擎自动查询已发布的最大版本号并原子递增 (+1)，返回生效后的定义实体
func (e *flowEngine) GetTask(ctx context.Context, taskID string) (*model.Task, error) {
	return e.storage.GetTask(ctx, taskID)
}

func (e *flowEngine) GetInstance(ctx context.Context, instanceID string) (*model.Instance, error) {
	return e.storage.GetInstance(ctx, instanceID)
}

func (e *flowEngine) Transfer(ctx context.Context, taskID string, targetUsers []string) error {
	validUsers := lo.Filter(targetUsers, func(u string, _ int) bool {
		return strings.TrimSpace(u) != ""
	})
	if len(validUsers) == 0 {
		return errors.New("转签目标用户不能为空")
	}
	if len(validUsers) > 1 {
		return errors.New("转签仅支持指定单个目标处理人")
	}
	targetUser := strings.TrimSpace(validUsers[0])

	return e.DoTx(ctx, func(txCtx context.Context, txEngine Engine) error {
		txEng, err := castEngine(txEngine)
		if err != nil {
			return err
		}
		task, inst, err := txEng.loadAndValidateTask(txCtx, taskID)
		if err != nil {
			return err
		}
		// 若已指派给相同处理人，幂等返回
		if task.Assignee == targetUser {
			return nil
		}

		oldAssignee := task.Assignee
		task.Assignee = targetUser
		if err := txEng.storage.UpdateTask(txCtx, task); err != nil {
			return err
		}

		// 复用缓存加载流程执行拓扑，广播转签审计事件
		plan, err := txEng.loadPlan(txCtx, inst)
		if err != nil {
			return err
		}
		var nodeSpec *model.NodeSpec
		if n, ok := plan.NodesMap[task.NodeID]; ok {
			nodeSpec = &n
		}
		txEng.publisher.Publish(txCtx, NewTaskEvent(EventTaskTransferred, inst, nodeSpec, task, oldAssignee, fmt.Sprintf("由 [%s] 转签给 [%s]", oldAssignee, targetUser)))
		return nil
	})
}

func (e *flowEngine) Deploy(ctx context.Context, def model.ProcessDefinition) (model.ProcessDefinition, error) {
	if err := def.Validate(); err != nil {
		return model.ProcessDefinition{}, fmt.Errorf("%w: %s", ErrInvalidDefinition, err.Error())
	}
	if def.Version <= 0 {
		latestVersion, err := e.storage.GetLatestVersion(ctx, def.Key)
		if err != nil {
			return model.ProcessDefinition{}, fmt.Errorf("获取流程最新版本失败: %w", err)
		}
		def.Version = latestVersion + 1
	}
	if err := e.storage.SaveDefinition(ctx, def); err != nil {
		return model.ProcessDefinition{}, err
	}
	e.planCache.Put(def)
	return def, nil
}

func (e *flowEngine) cloneWithTx(store storage.IStorage, pub IEventPublisher) *flowEngine {
	sub := &flowEngine{
		storage:     store,
		hooks:       e.hooks,
		eventBus:    e.eventBus,
		publisher:   pub,
		workers:     e.workers,
		middlewares: e.middlewares,
		planCache:   e.planCache,
		idGen:       e.idGen,
	}
	sub.executors = newDefaultExecutors(sub)
	return sub
}

// DoTx 在物理本地事务闭包内执行操作，异常自动回滚，并在事务成功 Commit 后统一刷出生命周期事件（事务性 Outbox 模式）
func (e *flowEngine) DoTx(ctx context.Context, fn func(ctx context.Context, txEngine Engine) error) error {
	bufPub := newBufferedEventPublisher(e.publisher)

	txStore, ok := e.storage.(storage.ITxStorage)
	if !ok {
		txSubEngine := e.cloneWithTx(e.storage, bufPub)
		if err := fn(ctx, txSubEngine); err != nil {
			return err
		}
		bufPub.Flush()
		return nil
	}

	err := txStore.DoTx(ctx, func(txStorage storage.IStorage) error {
		txSubEngine := e.cloneWithTx(txStorage, bufPub)
		return fn(ctx, txSubEngine)
	})
	if err != nil {
		return err
	}
	bufPub.Flush()
	return nil
}

// Start 启动流程实例，激活初始节点并派发待办任务
func (e *flowEngine) Start(ctx context.Context, req StartReq) (string, error) {
	actCtx := &middleware.ActionContext{
		ActionType: middleware.ActionStart,
		Operator:   req.Starter,
		Variables:  req.Variables,
		Extra:      make(map[string]any),
	}
	core := func(c context.Context, ac *middleware.ActionContext) error {
		return e.DoTx(c, func(txCtx context.Context, txEngine Engine) error {
			txEng, err := castEngine(txEngine)
			if err != nil {
				return err
			}
			id, err := txEng.doStart(txCtx, req)
			if err != nil {
				return err
			}
			ac.InstanceID = id
			return nil
		})
	}
	if err := middleware.Compose(e.middlewares, core)(ctx, actCtx); err != nil {
		return "", err
	}
	return actCtx.InstanceID, nil
}

func (e *flowEngine) GenInstanceID(procKey string) string {
	return e.idGen.GenInstanceID(procKey)
}

func (e *flowEngine) doStart(ctx context.Context, req StartReq) (string, error) {
	def, err := e.loadOrDeployDefinition(ctx, req)
	if err != nil {
		return "", err
	}

	plan := kernel.NewExecutionPlan(def)
	startNode, err := plan.FindStartNode()
	if err != nil {
		return "", err
	}

	instID := req.InstanceID
	if instID == "" {
		instID = e.idGen.GenInstanceID(def.Key)
	}
	inst := &model.Instance{
		ID:          instID,
		ProcKey:     def.Key,
		Version:     def.Version,
		Starter:     req.Starter,
		BusinessKey: req.BusinessKey,
		Status:      model.InstanceRunning,
		CurrentNode: startNode.ID,
		Variables:   req.Variables,
		CreateTime:  time.Now(),
	}

	// 1. 下沉到内核求解初始推进决策
	decision, err := plan.ResolveNextStep(startNode.ID, req.Variables, nil, 1)
	if err != nil {
		return "", fmt.Errorf("推导开始节点后续流转失败: %w", err)
	}

	session := newFlowSession(e.storage, inst, plan, nil)
	_ = session.SaveInstance(ctx) // 标记新创建的流程实例为脏，保证收口时写入

	// 2. 优先广播流程启动事件（确保提单人通知/撤销卡片在流转前第一时间发出，杜绝审批待办抢跑乱序）
	e.publisher.Publish(ctx, NewNodeEvent(EventProcessStarted, session.Instance(), &startNode, req.Starter, ""))

	// 3. 统一调度下一阶段动作（自动Skip占位、办结或激活待办；若无有效出路则由调度器统一拦截断流）
	if err := e.applyStepDecision(ctx, session, decision, 1, req.Starter); err != nil {
		return "", err
	}

	// 4. 工作单元统一收口：在事务提交前一次性刷入流程实例最终状态
	if err := session.Flush(ctx); err != nil {
		return "", err
	}

	return instID, nil
}

// Pass 审批人同意通过当前任务，触发状态机推导并流向下一步
func (e *flowEngine) Pass(ctx context.Context, req PassReq) error {
	actCtx := &middleware.ActionContext{
		ActionType: middleware.ActionPass,
		TaskID:     req.TaskID,
		Operator:   req.Operator,
		Comment:    req.Comment,
		Variables:  req.Variables,
		Extra:      make(map[string]any),
	}
	core := func(c context.Context, ac *middleware.ActionContext) error {
		return e.DoTx(c, func(txCtx context.Context, txEngine Engine) error {
			txEng, err := castEngine(txEngine)
			if err != nil {
				return err
			}
			return txEng.doPass(txCtx, req)
		})
	}
	return middleware.Compose(e.middlewares, core)(ctx, actCtx)
}

func (e *flowEngine) doPass(ctx context.Context, req PassReq) error {
	// 1. 锁查询与客观物理状态防卫校验 (防并发重复处理、防终态非法流转)
	task, inst, err := e.loadAndValidateTask(ctx, req.TaskID)
	if err != nil {
		return err
	}

	// 2. 初始化流转工作单元会话 (预加载全量任务快照与执行拓扑)
	session, err := e.newSessionForInstance(ctx, inst)
	if err != nil {
		return err
	}

	// 3. 执行状态机推进管线
	if err := e.advanceTaskPipeline(ctx, session, task, req.Comment, req.Variables, req.Operator); err != nil {
		return err
	}

	// 4. 工作单元统一收口：在事务提交前一次性刷入流程实例最终状态（N -> 1 UPDATE）
	return session.Flush(ctx)
}

// CompleteTask 外部异步作业/系统执行器完成任务后的回调入口，唤醒挂起并驱动状态机
func (e *flowEngine) CompleteTask(ctx context.Context, req CompleteTaskReq) error {
	comment := req.Comment
	if comment == "" {
		comment = "系统异步作业执行完成"
	}
	actCtx := &middleware.ActionContext{
		ActionType: middleware.ActionComplete,
		TaskID:     req.TaskID,
		Operator:   "system",
		Comment:    comment,
		Variables:  req.Variables,
		Extra:      make(map[string]any),
	}
	core := func(c context.Context, ac *middleware.ActionContext) error {
		return e.DoTx(c, func(txCtx context.Context, txEngine Engine) error {
			txEng, err := castEngine(txEngine)
			if err != nil {
				return err
			}
			return txEng.doCompleteTask(txCtx, req, comment)
		})
	}
	return middleware.Compose(e.middlewares, core)(ctx, actCtx)
}

func (e *flowEngine) doCompleteTask(ctx context.Context, req CompleteTaskReq, comment string) error {
	task, inst, err := e.loadAndValidateTask(ctx, req.TaskID)
	if err != nil {
		return err
	}

	session, err := e.newSessionForInstance(ctx, inst)
	if err != nil {
		return err
	}

	if err := e.advanceTaskPipeline(ctx, session, task, comment, req.Variables, "system"); err != nil {
		return err
	}

	return session.Flush(ctx)
}

// advanceTaskPipeline 推进已办结任务的核心状态机管线（模板方法模式）
// 统一封装：任务通过 -> 节点仲裁 -> 下一步推导 -> 决策应用
func (e *flowEngine) advanceTaskPipeline(ctx context.Context, session *flowSession, task *model.Task, comment string, variables model.Variables, operator string) error {
	inst := session.Instance()

	// 1. 办结当前任务并增量合并表单变量
	task.Pass(comment)
	if err := session.UpdateTask(ctx, task); err != nil {
		return err
	}
	if len(variables) > 0 {
		inst.MergeVariables(variables)
	}

	// 广播任务通过事件
	node, ok := session.Plan().NodesMap[task.NodeID]
	if !ok {
		return fmt.Errorf("%w: 节点定义不存在 [node_id=%s]", ErrNodeDefinitionMissing, task.NodeID)
	}
	e.publisher.Publish(ctx, NewTaskEvent(EventTaskPassed, inst, &node, task, operator, comment))

	// 2. 当前节点纯内存流转仲裁（含或签联动跳过计算）
	arbRes := kernel.ArbitrateNode(node, task, session.AllTasks())
	if len(arbRes.SkippedTaskIDs) > 0 {
		if err := session.UpdateTasksStatus(ctx, arbRes.SkippedTaskIDs, model.TaskSkipped); err != nil {
			return fmt.Errorf("批量更新或签跳过任务失败: %w", err)
		}
		for _, skippedID := range arbRes.SkippedTaskIDs {
			if st, ok := session.GetTask(skippedID); ok {
				e.publisher.Publish(ctx, NewTaskEvent(EventTaskSkipped, inst, &node, st, "system", "或签竞争，系统自动跳过"))
			}
		}
	}
	if !arbRes.IsSatisfied {
		return session.SaveInstance(ctx) // 仲裁未就绪，标记状态并保持等待
	}

	// 3. 内核下沉推导下一步流向（纯内存计算，直接复用会话内的最新任务快照）
	decision, err := session.Plan().ResolveNextStep(task.NodeID, inst.Variables, session.AllTasks(), task.BatchCode)
	if err != nil {
		return fmt.Errorf("推导下一步流转决策失败 [node_id=%s]: %w", node.ID, err)
	}

	// 4. 统一应用推导决策（自动保存Skip任务、汇聚等待、完结流程或派发新待办）
	return e.applyStepDecision(ctx, session, decision, task.BatchCode, operator)
}

// Reject 审批人驳回当前任务，原子清理兄弟待办并回退至指定或上游节点（批次号自增）
func (e *flowEngine) Reject(ctx context.Context, req RejectReq) error {
	actCtx := &middleware.ActionContext{
		ActionType: middleware.ActionReject,
		TaskID:     req.TaskID,
		Operator:   req.Operator,
		Comment:    req.Comment,
		Extra:      make(map[string]any),
	}
	core := func(c context.Context, ac *middleware.ActionContext) error {
		return e.DoTx(c, func(txCtx context.Context, txEngine Engine) error {
			txEng, err := castEngine(txEngine)
			if err != nil {
				return err
			}
			return txEng.doReject(txCtx, req)
		})
	}
	return middleware.Compose(e.middlewares, core)(ctx, actCtx)
}

func (e *flowEngine) doReject(ctx context.Context, req RejectReq) error {
	task, inst, err := e.loadAndValidateTask(ctx, req.TaskID)
	if err != nil {
		return err
	}

	session, err := e.newSessionForInstance(ctx, inst)
	if err != nil {
		return err
	}

	// 1. 办结当前任务为驳回态
	node, ok := session.Plan().NodesMap[task.NodeID]
	if !ok {
		return fmt.Errorf("%w: 节点定义不存在 [node_id=%s]", ErrNodeDefinitionMissing, task.NodeID)
	}
	task.Reject(req.Comment)
	if err := session.UpdateTask(ctx, task); err != nil {
		return err
	}
	e.publisher.Publish(ctx, NewTaskEvent(EventTaskRejected, session.Instance(), &node, task, req.Operator, req.Comment))

	// 2. 原子清理当前批次中其余未处理的兄弟任务
	if err := e.cancelPendingSiblingTasks(ctx, session, task); err != nil {
		return err
	}

	// 3. 若明确指定直接终止，流程置为终态 Rejected 并广播流程终止事件
	if req.Terminate {
		session.Instance().Reject()
		if err := session.SaveInstance(ctx); err != nil {
			return err
		}
		e.publisher.Publish(ctx, NewProcessEvent(EventProcessRejected, session.Instance(), req.Operator, req.Comment))
		return session.Flush(ctx)
	}

	// 4. 确定驳回的目标节点（自选目标节点 vs 默认回溯上游）
	var targetNodes []model.NodeSpec
	if req.TargetNodeID != "" {
		tn, ok := session.Plan().NodesMap[req.TargetNodeID]
		if !ok {
			return fmt.Errorf("%w: 指定的驳回目标节点 [%s] 不存在", ErrInvalidTargetNode, req.TargetNodeID)
		}
		if !tn.CanRejectTo() {
			return fmt.Errorf("%w: 驳回目标必须是人工审批或提单节点", ErrInvalidTargetNode)
		}

		// 防御校验：指定的驳回目标必须属于当前流转实际可达的合法候选（防止伪造请求向未走过的分支或跳过节点驳回）
		validTargets, err := session.Plan().FindAllUpstreamRejectTargets(task.NodeID, session.AllTasks())
		if err != nil {
			return fmt.Errorf("推导合法驳回目标失败: %w", err)
		}
		if !lo.SomeBy(validTargets, func(item model.NodeSpec) bool { return item.ID == req.TargetNodeID }) {
			return fmt.Errorf("%w: 节点 [%s] 未在当前实际执行路径中审批通过，不可作为驳回目标", ErrInvalidTargetNode, req.TargetNodeID)
		}

		targetNodes = []model.NodeSpec{tn}
	} else {
		nodes, err := session.Plan().FindUpstreamRejectNodes(task.NodeID, session.AllTasks())
		if err != nil {
			// 回溯失败属于拓扑异常，必须中断并回滚；严禁降级为终态驳回，否则工单侧收不到事件将永久滞留流转中
			return fmt.Errorf("回溯上游驳回目标节点失败 [node_id=%s]: %w", task.NodeID, err)
		}
		if len(nodes) == 0 {
			session.Instance().Reject() // 无可用上游审批节点，流程置为终态 Rejected
			if err := session.SaveInstance(ctx); err != nil {
				return err
			}
			e.publisher.Publish(ctx, NewProcessEvent(EventProcessRejected, session.Instance(), req.Operator, req.Comment))
			return session.Flush(ctx)
		}
		targetNodes = nodes
	}

	// 5. 流转批次号自增 (batch_code + 1)，激活回退的目标节点集合
	newBatchCode := task.BatchCode + 1
	if len(targetNodes) == 1 && targetNodes[0].Type == model.NodeStart {
		// 回退至发起人重新编辑发起
		targetNode := targetNodes[0]
		session.Instance().CurrentNode = targetNode.ID
		comment := fmt.Sprintf("由审批人 [%s] 驳回至提单人重新编辑: %s", req.Operator, req.Comment)
		startTask := e.buildStarterTask(targetNode, session.Instance().ID, session.Instance().Starter, newBatchCode, comment)
		if err := session.SaveTasks(ctx, []*model.Task{startTask}); err != nil {
			return err
		}
		e.publisher.Publish(ctx, NewTaskEvent(EventTaskCreated, session.Instance(), &targetNode, startTask, req.Operator, comment))
	} else {
		if err := e.activateNodes(ctx, session, targetNodes, newBatchCode, req.Operator); err != nil {
			return err
		}
	}

	if err := session.SaveInstance(ctx); err != nil {
		return err
	}
	return session.Flush(ctx)
}

// Revoke 提单人主动撤回正在流转中的工单
func (e *flowEngine) Revoke(ctx context.Context, req RevokeReq) error {
	actCtx := &middleware.ActionContext{
		ActionType: middleware.ActionRevoke,
		InstanceID: req.InstanceID,
		Operator:   req.Operator,
		Comment:    req.Comment,
		Extra:      make(map[string]any),
	}
	core := func(c context.Context, ac *middleware.ActionContext) error {
		return e.DoTx(c, func(txCtx context.Context, txEngine Engine) error {
			txEng, err := castEngine(txEngine)
			if err != nil {
				return err
			}
			return txEng.doRevoke(txCtx, req)
		})
	}
	return middleware.Compose(e.middlewares, core)(ctx, actCtx)
}

func (e *flowEngine) doRevoke(ctx context.Context, req RevokeReq) error {
	inst, err := e.storage.GetInstanceForUpdate(ctx, req.InstanceID)
	if err != nil {
		return err
	}
	if !inst.IsRunning() {
		return ErrInstanceNotRunning
	}

	session, err := e.newSessionForInstance(ctx, inst)
	if err != nil {
		return err
	}

	// 1. 获取所有任务并原子取消当前所有在途 Pending 任务
	pendingIDs := lo.FilterMap(session.AllTasks(), func(t *model.Task, _ int) (string, bool) {
		return t.ID, t.IsPending()
	})
	if len(pendingIDs) > 0 {
		if err := session.UpdateTasksStatus(ctx, pendingIDs, model.TaskCancelled); err != nil {
			return err
		}
		for _, pid := range pendingIDs {
			if pt, ok := session.GetTask(pid); ok {
				e.publisher.Publish(ctx, NewTaskEvent(EventTaskRevoked, session.Instance(), nil, pt, req.Operator, req.Comment))
			}
		}
	}

	// 2. 根据策略决定是退回提单人重新编辑，还是直接作废终结
	if req.BackToStarter {
		startNode, err := session.Plan().FindStartNode()
		if err != nil {
			return err
		}

		newBatchCode := session.MaxBatchCode() + 1
		session.Instance().CurrentNode = startNode.ID
		comment := fmt.Sprintf("提单人主动撤回修改: %s", req.Comment)
		startTask := e.buildStarterTask(startNode, session.Instance().ID, session.Instance().Starter, newBatchCode, comment)
		if err := session.SaveTasks(ctx, []*model.Task{startTask}); err != nil {
			return err
		}
		e.publisher.Publish(ctx, NewTaskEvent(EventTaskCreated, session.Instance(), &startNode, startTask, req.Operator, comment))
	} else {
		session.Instance().Cancel()
		e.publisher.Publish(ctx, NewProcessEvent(EventProcessCancelled, session.Instance(), req.Operator, req.Comment))
	}

	if err := session.SaveInstance(ctx); err != nil {
		return err
	}
	return session.Flush(ctx)
}

// cancelPendingSiblingTasks 原子清理同批次中其余未处理的兄弟任务（驳回时联动取消）
func (e *flowEngine) cancelPendingSiblingTasks(ctx context.Context, session *flowSession, task *model.Task) error {
	cancelIDs := lo.FilterMap(session.AllTasks(), func(t *model.Task, _ int) (string, bool) {
		return t.ID, t.ID != task.ID && t.BatchCode == task.BatchCode && t.IsPending()
	})
	if len(cancelIDs) > 0 {
		if err := session.UpdateTasksStatus(ctx, cancelIDs, model.TaskCancelled); err != nil {
			return fmt.Errorf("联动取消兄弟任务失败: %w", err)
		}
	}
	return nil
}

func (e *flowEngine) buildStarterTask(node model.NodeSpec, instID, starter string, batchCode int, comment string) *model.Task {
	task := model.NewTask(e.idGen.GenTaskID(node.ID, starter, batchCode, 1), instID, node, batchCode, starter, model.TaskPending)
	task.SignRule = model.SignOneVote
	task.Comment = comment
	return task
}

// GetTimeline 按执行批次提取流转时间线
func (e *flowEngine) GetTimeline(ctx context.Context, instanceID string) ([]model.TimelineEvent, error) {
	allTasks, err := e.storage.ListTasksByInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}
	return buildTimelineFromTasks(allTasks), nil
}

// ListPendingTasks 按条件分页查询未办结待办任务列表
func (e *flowEngine) ListPendingTasks(ctx context.Context, query model.PendingTaskQuery) ([]*model.Task, int64, error) {
	return e.storage.ListPendingTasks(ctx, query)
}

// ListPendingTasksByInstances 批量拉取多个流程实例当前挂起的未办结审批任务（单次 SQL 批量检索）
func (e *flowEngine) ListPendingTasksByInstances(ctx context.Context, instanceIDs []string) ([]*model.Task, error) {
	if len(instanceIDs) == 0 {
		return []*model.Task{}, nil
	}
	return e.storage.ListPendingTasksByInstances(ctx, instanceIDs)
}

// ListTasksByInstance 获取指定流程实例下的全部流转与历史任务列表
func (e *flowEngine) ListTasksByInstance(ctx context.Context, instanceID string) ([]*model.Task, error) {
	if instanceID == "" {
		return nil, errors.New("流程实例 ID 不能为空")
	}
	return e.storage.ListTasksByInstance(ctx, instanceID)
}

// GetDefinition 根据流程唯一标识与版本号加载流程图定义快照
func (e *flowEngine) GetDefinition(ctx context.Context, key string, version int) (model.ProcessDefinition, error) {
	if e.storage == nil {
		return model.ProcessDefinition{}, errors.New("存储层未初始化")
	}
	return e.storage.GetDefinition(ctx, key, version)
}

// GetExecutionView 获取流程实例的运行态综合视图（包含节点状态映射、高亮已流转边、待处理人等）
func (e *flowEngine) GetExecutionView(ctx context.Context, instanceID string) (model.ExecutionView, error) {
	if instanceID == "" {
		return model.ExecutionView{}, errors.New("流程实例 ID 不能为空")
	}

	inst, err := e.GetInstance(ctx, instanceID)
	if err != nil {
		return model.ExecutionView{}, fmt.Errorf("查询流程实例详情失败: %w", err)
	}

	tasks, err := e.ListTasksByInstance(ctx, instanceID)
	if err != nil {
		return model.ExecutionView{}, fmt.Errorf("查询任务执行记录失败: %w", err)
	}

	plan, err := e.loadPlan(ctx, inst)
	if err != nil {
		return model.ExecutionView{}, fmt.Errorf("加载流程执行计划失败: %w", err)
	}

	// 按节点分组聚合任务状态，避免或签场景下兄弟任务的 TaskSkipped(5) 错误覆盖已通过的 TaskPassed(1)
	nodeTasks := lo.GroupBy(tasks, func(t *model.Task) string {
		return t.NodeID
	})
	nodeStatusMap := lo.MapValues(nodeTasks, func(group []*model.Task, _ string) int {
		return resolveNodeAggregateStatus(group)
	})

	pendingUsers := lo.Uniq(lo.FilterMap(tasks, func(t *model.Task, _ int) (string, bool) {
		if t.IsPending() && t.Assignee != "" {
			return t.Assignee, true
		}
		return "", false
	}))

	traversedEdges := plan.CalculateTraversedEdges(tasks, inst)

	return model.ExecutionView{
		ProcessKey:     inst.ProcKey,
		ProcessVersion: inst.Version,
		NodeStatusMap:  nodeStatusMap,
		TraversedEdges: traversedEdges,
		PendingUsers:   pendingUsers,
	}, nil
}

// resolveNodeAggregateStatus 根据节点下的所有任务裁决节点的综合状态
// 状态裁决原则：基于该节点产生的最新批次（Max BatchCode）进行裁决，避免历史批次干扰当前流转表现
// 同批次内状态裁决优先级：Passed (通过) > Rejected (驳回) > Pending (待办中) > Skipped (全跳过)
func resolveNodeAggregateStatus(tasks []*model.Task) int {
	currentTasks := model.FilterLatestBatchTasks(tasks)
	if len(currentTasks) == 0 {
		return int(model.TaskPending)
	}

	// 1. 最新批次内只要有任意任务已通过（人工通过或系统放行），节点综合状态裁决为通过
	if lo.SomeBy(currentTasks, func(t *model.Task) bool { return t.IsPassed() }) {
		return int(model.TaskPassed)
	}

	// 2. 最新批次内存在主动驳回任务，节点综合状态裁决为驳回
	if lo.SomeBy(currentTasks, func(t *model.Task) bool { return t.Status == model.TaskRejected }) {
		return int(model.TaskRejected)
	}

	// 3. 最新批次内仍有待办任务（会签等待中或尚未处理），节点综合状态裁决为待办中
	if lo.SomeBy(currentTasks, func(t *model.Task) bool { return t.IsPending() }) {
		return int(model.TaskPending)
	}

	// 4. 当且仅当最新批次中所有任务全为跳过时，节点综合状态才为跳过
	if lo.EveryBy(currentTasks, func(t *model.Task) bool { return t.Status == model.TaskSkipped }) {
		return int(model.TaskSkipped)
	}

	// 5. 兜底返回最新批次的最后一个任务状态
	return int(currentTasks[len(currentTasks)-1].Status)
}

// GetRejectTargets 查询指定任务允许驳回的目标节点集合（包含起点与历史已流转审批节点）
func (e *flowEngine) GetRejectTargets(ctx context.Context, taskID string) ([]model.NodeSpec, error) {
	task, inst, err := e.loadAndValidateTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("加载任务失败: %w", err)
	}

	session, err := e.newSessionForInstance(ctx, inst)
	if err != nil {
		return nil, fmt.Errorf("加载流程会话失败: %w", err)
	}

	return session.Plan().FindAllUpstreamRejectTargets(task.NodeID, session.AllTasks())
}
