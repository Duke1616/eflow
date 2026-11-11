package notification

import (
	"context"
	"strconv"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/notification/strategy"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eiam/pkg/ctxutil"
	"github.com/ecodeclub/ekit/queue"
	"github.com/ecodeclub/ekit/syncx"
	"github.com/gotomicro/ego/core/elog"
)

// FlowxNotificationListener 监听 flowx 流程引擎生命周期事件并触发多渠道消息通知（如飞书卡片）
// 核心设计：基于事件总线完全解耦消息副作用，与工作流主事务生命周期脱钩，确保高可用与非阻塞。
type FlowxNotificationListener struct {
	ticketRepo  repository.TicketRepository
	workflowSvc workflowSvc.Service
	dispatcher  *strategy.Dispatcher
	logger      *elog.Component
	syncMode    bool

	// instanceQueues 针对同一流程实例的串行任务队列映射（基于 ekit 泛型并发 Map，彻底消灭互斥锁竞争）
	instanceQueues syncx.Map[string, *queue.ConcurrentLinkedBlockingQueue[func()]]
}

// Option 配置 FlowxNotificationListener 可选参数
type Option func(*FlowxNotificationListener)

// WithSyncMode 设置是否同步处理（默认为 false 异步非阻塞执行；单测可设置为 true 保证可确定性断言）
func WithSyncMode(sync bool) Option {
	return func(l *FlowxNotificationListener) {
		l.syncMode = sync
	}
}

// NewFlowxNotificationListener 构造 flowx 专属消息通知监听器
func NewFlowxNotificationListener(
	ticketRepo repository.TicketRepository,
	workflowSvc workflowSvc.Service,
	dispatcher *strategy.Dispatcher,
	opts ...Option,
) flowx.IEventListener {
	l := &FlowxNotificationListener{
		ticketRepo:  ticketRepo,
		workflowSvc: workflowSvc,
		dispatcher:  dispatcher,
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("FlowxNotificationListener")),
	}
	for _, opt := range opts {
		opt(l)
	}
	return l
}

// InterestIn 声明关注的领域生命周期事件
func (l *FlowxNotificationListener) InterestIn() []flowx.EventType {
	return []flowx.EventType{
		flowx.EventProcessStarted, // 流程实例启动 -> 触发发起人撤单卡片
		flowx.EventTaskCreated,    // 审批任务创建 -> 触发审批人待办审批卡片
		flowx.EventNodeEntered,    // 节点进入 -> 触发抄送通知或群通知
	}
}

// OnEvent 响应流程引擎广播的生命周期事件（异步非阻塞脱钩主事务，单实例严格保序）
func (l *FlowxNotificationListener) OnEvent(ctx context.Context, evt flowx.Event) error {
	if evt.Instance == nil || evt.Instance.ID == "" {
		return nil
	}

	// 遵循并发安全与可靠性法则：异步协程生命周期脱钩，防止上游 Context 取消导致通知被截断
	safeCtx := context.WithoutCancel(ctx)

	process := func() {
		defer func() {
			if r := recover(); r != nil {
				l.logger.Error("flowx 通知处理发生 panic", elog.Any("recover", r))
			}
		}()
		l.handleEvent(safeCtx, evt)
	}

	if l.syncMode {
		process()
	} else {
		l.dispatchAsync(evt.Instance.ID, process)
	}

	return nil
}

// dispatchAsync 投递任务到对应实例的串行队列中（基于 ekit 并发阻塞队列与并发 Map），保障同流程实例下不同类型通知因果时序严格有序
func (l *FlowxNotificationListener) dispatchAsync(instanceKey string, fn func()) {
	newQ := queue.NewConcurrentLinkedBlockingQueue[func()](0)
	q, loaded := l.instanceQueues.LoadOrStore(instanceKey, newQ)
	if !loaded {
		// 仅由首个创建队列的协程启动该流程实例的单消费者 Worker
		go l.runInstanceWorker(instanceKey, newQ)
	}

	_ = q.Enqueue(context.Background(), fn)
}

// runInstanceWorker 单流程实例专属消费协程，空闲超时自动清理退出防止内存泄露
func (l *FlowxNotificationListener) runInstanceWorker(instanceKey string, q *queue.ConcurrentLinkedBlockingQueue[func()]) {
	for {
		// 单次等待空闲超时为 2 分钟，超时未有新任务则自动清理退出，防止内存泄漏
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute*2)
		task, err := q.Dequeue(ctx)
		cancel()

		if err != nil {
			if q.Len() == 0 {
				l.instanceQueues.Delete(instanceKey)
				// 二次兜底：排空 Delete 期间极小概率并发追加的残留任务
				l.drainRemainingTasks(q)
				return
			}
			continue
		}

		task()
	}
}

// drainRemainingTasks 排空并消费队列中残留的任务，带严格超时保护与 defer cancel 防泄漏
func (l *FlowxNotificationListener) drainRemainingTasks(q *queue.ConcurrentLinkedBlockingQueue[func()]) {
	if q.Len() == 0 {
		return
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), time.Second*5)
	defer drainCancel()

	for q.Len() > 0 {
		task, err := q.Dequeue(drainCtx)
		if err != nil {
			break
		}
		task()
	}
}

func (l *FlowxNotificationListener) handleEvent(safeCtx context.Context, evt flowx.Event) {
	instanceKey := evt.Instance.ID

	// 1. 优先根据 BusinessKey（即工单 ID）秒级检索工单实体；若无则通过流程实例多态标识检索
	var (
		ticket domain.Ticket
		err    error
	)
	if evt.Instance.BusinessKey != "" {
		if tID, parseErr := strconv.ParseInt(evt.Instance.BusinessKey, 10, 64); parseErr == nil && tID > 0 {
			ticket, err = l.ticketRepo.Detail(safeCtx, tID)
		}
	}
	if ticket.Id == 0 || err != nil {
		ticket, err = l.ticketRepo.DetailByEngineInstance(safeCtx, domain.EngineTypeFlowx, domain.NewID(instanceKey))
	}
	if err != nil {
		l.logger.Warn("flowx 通知处理: 未检索到流程实例关联的工单",
			elog.String("instanceKey", instanceKey),
			elog.String("businessKey", evt.Instance.BusinessKey),
			elog.FieldErr(err))
		return
	}

	// 补全流程实例多态标识与引擎元信息，确保后续策略统一寻址
	if ticket.Process.InstanceID.IsZero() {
		ticket.Process.InstanceID = domain.NewID(instanceKey)
		ticket.Process.EngineType = domain.EngineTypeFlowx
	}

	// 融合 flowx 流程实例最新的动态变量表，保障即时通知卡片渲染不缺失刚填写的业务字段
	if evt.Instance != nil && len(evt.Instance.Variables) > 0 {
		if ticket.Data == nil {
			ticket.Data = make(domain.TicketData)
		}
		for k, v := range evt.Instance.Variables {
			ticket.Data[k] = v
		}
	}

	// 2. 注入工单真实租户上下文，保障多租户权限与通知配置安全隔离
	safeCtx = ctxutil.WithTenantID(safeCtx, ticket.TenantID)

	// 3. 检索工作流定义模型
	wf, err := l.workflowSvc.Find(safeCtx, ticket.WorkflowId)
	if err != nil {
		l.logger.Warn("flowx 通知处理: 未检索到工作流定义",
			elog.Int64("workflowID", ticket.WorkflowId),
			elog.FieldErr(err))
		return
	}

	// 4. 根据不同事件细分路由对应的通知策略
	switch evt.Type {
	case flowx.EventProcessStarted:
		l.handleProcessStarted(safeCtx, instanceKey, ticket, wf, evt)

	case flowx.EventTaskCreated:
		l.handleTaskCreated(safeCtx, instanceKey, ticket, wf, evt)

	case flowx.EventNodeEntered:
		l.handleNodeEntered(safeCtx, instanceKey, ticket, wf, evt)
	}
}

func (l *FlowxNotificationListener) handleProcessStarted(ctx context.Context, instanceKey string, ticket domain.Ticket, wf domain.Workflow, evt flowx.Event) {
	nodeID := "Start"
	if evt.Node != nil && evt.Node.ID != "" {
		nodeID = evt.Node.ID
	}

	info := l.buildNotificationInfo(strategy.Start, instanceKey, ticket, wf, nodeID, "Start", []string{ticket.CreateBy})
	l.sendSafe(ctx, info, "流程启动提单通知")
}

func (l *FlowxNotificationListener) handleTaskCreated(ctx context.Context, instanceKey string, ticket domain.Ticket, wf domain.Workflow, evt flowx.Event) {
	if evt.Task == nil {
		return
	}

	info := l.buildNotificationInfo(strategy.User, instanceKey, ticket, wf, evt.Task.NodeID, evt.Task.NodeName, []string{evt.Task.Assignee})
	l.sendSafe(ctx, info, "人工审批待办通知")
}

func (l *FlowxNotificationListener) handleNodeEntered(ctx context.Context, instanceKey string, ticket domain.Ticket, wf domain.Workflow, evt flowx.Event) {
	if evt.Node == nil {
		return
	}

	strategyName, ok := map[flowxModel.NodeType]strategy.NodeName{
		flowxModel.NodeCarbonCopy: strategy.CarbonCopy,
		flowxModel.NodeChatGroup:  strategy.ChatGroup,
	}[evt.Node.Type]
	if !ok {
		return // 忽略无需消息推送的中间网关或状态节点
	}

	info := l.buildNotificationInfo(strategyName, instanceKey, ticket, wf, evt.Node.ID, evt.Node.Name, evt.Node.Assignees)
	l.sendSafe(ctx, info, string(strategyName)+"节点通知")
}

func (l *FlowxNotificationListener) buildNotificationInfo(
	strategyName strategy.NodeName,
	instanceKey string,
	ticket domain.Ticket,
	wf domain.Workflow,
	nodeID, nodeName string,
	userIDs []string,
) strategy.Info {
	instID := domain.NewID(instanceKey).Int()
	if ticket.Process.InstanceID.IsZero() && instanceKey != "" {
		ticket.Process.InstanceID = domain.NewID(instanceKey)
		ticket.Process.EngineType = domain.EngineTypeFlowx
	}
	return strategy.Info{
		NodeName: strategyName,
		FlowContext: strategy.FlowContext{
			InstID:   instID,
			Ticket:   ticket,
			Workflow: wf,
			Instance: domain.Instance{
				ProcInstID: instID,
				Starter:    ticket.CreateBy,
			},
			CurrentNode: &strategy.CurrentNode{
				NodeID:   nodeID,
				NodeName: nodeName,
				UserIDs:  userIDs,
			},
		},
	}
}

func (l *FlowxNotificationListener) sendSafe(ctx context.Context, info strategy.Info, desc string) {
	if l.dispatcher == nil {
		return
	}

	resp, err := l.dispatcher.Send(ctx, info)
	if err != nil {
		l.logger.Error("flowx 消息分发失败",
			elog.String("action", desc),
			elog.Int64("ticketID", info.Ticket.Id),
			elog.String("nodeID", info.CurrentNode.NodeID),
			elog.String("strategy", string(info.NodeName)),
			elog.FieldErr(err))
		return
	}

	l.logger.Info("flowx 消息分发成功",
		elog.String("action", desc),
		elog.Int64("ticketID", info.Ticket.Id),
		elog.String("nodeID", info.CurrentNode.NodeID),
		elog.String("status", resp.Status))
}
