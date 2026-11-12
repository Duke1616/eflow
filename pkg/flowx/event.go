package flowx

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// ==========================================
// 1. 同步守卫与动态算人 (Synchronous Hooks)
// ==========================================

// HookHandler 节点同步钩子函数签名，运行在主事务内，可动态算人或主动阻断
type HookHandler func(ctx context.Context, hc *HookContext) error

// HookContext 节点激活时的同步执行上下文 (支持动态算人、变量就地补齐与前置阻断)
// 核心设计：持有 Instance 领域实体指针，保证 Hook 内部的就地修改能直接生效
type HookContext struct {
	Instance  *model.Instance // 流程实例领域实体指针 (支持就地读取与状态检查)
	Node      *model.NodeSpec // 当前节点规格
	Operator  string          // 当前操作人
	Variables model.Variables // 全局运行时变量表引用

	assignees []string // 候选审批人切片 (内聚封装)
	aborted   bool
	abortErr  error
}

// NewHookContext 构建同步节点钩子上下文
func NewHookContext(inst *model.Instance, node *model.NodeSpec, operator string, assignees []string) *HookContext {
	return &HookContext{
		Instance:  inst,
		Node:      node,
		Operator:  operator,
		Variables: hookVariables(inst),
		assignees: assignees,
	}
}

func hookVariables(inst *model.Instance) model.Variables {
	if inst != nil {
		inst.Variables.Ensure()
		return inst.Variables
	}
	var variables model.Variables
	variables.Ensure()
	return variables
}

// Assignees 获取当前解析出的候选审批人列表
func (c *HookContext) Assignees() []string {
	return c.assignees
}

// SetAssignees 动态设置/重写审批人列表 (支持角色算人、组织架构级联插件)
func (c *HookContext) SetAssignees(users ...string) {
	c.assignees = lo.Uniq(users)
}

// MergeVariables 批量原地合并增量业务变量
func (c *HookContext) MergeVariables(patch model.Variables) {
	c.Variables.Merge(patch)
}

// Abort 主动阻断后续流转，回滚当前流转事务
func (c *HookContext) Abort(err error) {
	c.aborted = true
	c.abortErr = err
}

// IsAborted 判断当前是否已被阻断
func (c *HookContext) IsAborted() bool {
	return c.aborted
}

// AbortError 获取阻断原因
func (c *HookContext) AbortError() error {
	return c.abortErr
}

// hookRegistry 节点钩子注册表（并发安全）
type hookRegistry struct {
	mu    sync.RWMutex
	hooks map[string]HookHandler
}

func newHookRegistry() *hookRegistry {
	return &hookRegistry{
		hooks: make(map[string]HookHandler),
	}
}

func (r *hookRegistry) Register(name string, handler HookHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hooks[name] = handler
}

// Execute 顺序执行节点绑定的具名 Hook
func (r *hookRegistry) Execute(ctx context.Context, hookNames []string, hc *HookContext) error {
	if len(hookNames) == 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, name := range hookNames {
		handler, exists := r.hooks[name]
		if !exists {
			return fmt.Errorf("未注册的节点 Hook: %s", name)
		}

		if err := handler(ctx, hc); err != nil {
			return fmt.Errorf("节点 Hook [%s] 执行失败: %w", name, err)
		}

		if hc.IsAborted() {
			if hc.AbortError() != nil {
				return hc.AbortError()
			}
			return ErrHookAborted
		}
	}

	return nil
}

// ==========================================
// 2. 异步生命周期事件总线 (EventBus & Listener)
// ==========================================

// EventType 流程与任务全生命周期强类型事件枚举
type EventType string

const (
	// EventProcessStarted 流程实例启动并产生初始任务
	EventProcessStarted EventType = "PROCESS_STARTED"
	// EventProcessFinished 流程实例全单办结 (流向 NodeEnd)
	EventProcessFinished EventType = "PROCESS_FINISHED"
	// EventProcessRejected 流程实例被直接终止驳回
	EventProcessRejected EventType = "PROCESS_REJECTED"
	// EventProcessCancelled 流程实例被提单人主动撤回作废
	EventProcessCancelled EventType = "PROCESS_CANCELLED"

	// EventNodeEntered 节点被激活进入 (用于抄送知会、群通知、外部联动等)
	EventNodeEntered EventType = "NODE_ENTERED"

	// EventTaskCreated 新待办任务生成并派发
	EventTaskCreated EventType = "TASK_CREATED"
	// EventTaskPassed 任务被审批人同意通过
	EventTaskPassed EventType = "TASK_PASSED"
	// EventTaskRejected 任务被审批人驳回
	EventTaskRejected EventType = "TASK_REJECTED"
	// EventTaskRevoked 任务因提单人撤回被取消
	EventTaskRevoked EventType = "TASK_REVOKED"
	// EventTaskSkipped 任务因或签竞争或分支未满足被跳过
	EventTaskSkipped EventType = "TASK_SKIPPED"
	// EventTaskTransferred 任务被转签指派给新处理人
	EventTaskTransferred EventType = "TASK_TRANSFERRED"
)

// Event 流程运行时生命周期不可变事件快照
type Event struct {
	Type      EventType       `json:"type"`                // 事件类型
	Timestamp time.Time       `json:"timestamp"`           // 触发时间戳
	Instance  *model.Instance `json:"instance"`            // 所属流程实例
	Node      *model.NodeSpec `json:"node,omitempty"`      // 触发事件的节点（可选）
	Task      *model.Task     `json:"task,omitempty"`      // 触发事件的任务（可选）
	Operator  string          `json:"operator,omitempty"`  // 操作人
	Comment   string          `json:"comment,omitempty"`   // 操作意见或说明
	Variables model.Variables `json:"variables,omitempty"` // 瞬时运行时变量
}

// Clone 创建事件只读深拷贝副本，供异步监听器/后台 Goroutine 安全读取，彻底杜绝与主流程的数据竞争 (Data Race)
func (e Event) Clone() Event {
	cp := e
	if e.Instance != nil {
		cp.Instance = e.Instance.Clone()
	}
	if e.Task != nil {
		cp.Task = e.Task.Clone()
	}
	if e.Variables != nil {
		// Variables 与 Instance.Variables 同源（三个构造器都直送 inst.Variables），
		// 复用实例已克隆好的变量表，避免同一张 map 在一次 Clone 里被深拷贝两遍
		if cp.Instance != nil {
			cp.Variables = cp.Instance.Variables
		} else {
			cp.Variables = e.Variables.Clone()
		}
	}
	return cp
}

// NewProcessEvent 构建流程级生命周期事件 (启动/办结/驳回/撤回等无具体任务关联的事件)
func NewProcessEvent(typ EventType, inst *model.Instance, operator, comment string) Event {
	return Event{
		Type: typ, Timestamp: time.Now(), Instance: inst,
		Operator: operator, Comment: comment, Variables: inst.Variables,
	}
}

// NewNodeEvent 构建节点级事件 (流程办结等仅关联节点但无任务的场景)
func NewNodeEvent(typ EventType, inst *model.Instance, node *model.NodeSpec, operator, comment string) Event {
	return Event{
		Type: typ, Timestamp: time.Now(), Instance: inst,
		Node: node, Operator: operator, Comment: comment, Variables: inst.Variables,
	}
}

// NewTaskEvent 构建任务级生命周期事件 (创建/通过/驳回/跳过/撤回等与具体任务关联的事件)
func NewTaskEvent(typ EventType, inst *model.Instance, node *model.NodeSpec, task *model.Task, operator, comment string) Event {
	return Event{
		Type: typ, Timestamp: time.Now(), Instance: inst,
		Node: node, Task: task, Operator: operator, Comment: comment, Variables: inst.Variables,
	}
}

// IEventListener 全局事件观察者/监听器契约
type IEventListener interface {
	// InterestIn 声明感兴趣的事件类型切片；若返回空切片则默认接收所有事件
	InterestIn() []EventType
	// OnEvent 接收并响应生命周期事件 (建议用于通知推送、审计归档、外部系统同步)
	OnEvent(ctx context.Context, event Event) error
}

// functionalEventListener 函数式事件监听器适配器
type functionalEventListener struct {
	types []EventType
	fn    func(ctx context.Context, event Event) error
}

func (l *functionalEventListener) InterestIn() []EventType {
	return l.types
}

func (l *functionalEventListener) OnEvent(ctx context.Context, event Event) error {
	return l.fn(ctx, event)
}

// NewEventListener 创建函数式事件监听器
func NewEventListener(types []EventType, fn func(ctx context.Context, event Event) error) IEventListener {
	return &functionalEventListener{
		types: types,
		fn:    fn,
	}
}

// EventBus 事件广播总线（支持多态派发与并发安全监听）
type EventBus struct {
	mu        sync.RWMutex
	listeners []IEventListener
}

func newEventBus() *EventBus {
	return &EventBus{
		listeners: make([]IEventListener, 0),
	}
}

// AddListener 注册观察者监听器
func (b *EventBus) AddListener(listener IEventListener) {
	if listener == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.listeners = append(b.listeners, listener)
}

// Publish 广播生命周期事件给所有匹配的监听器（带只读深拷贝快照与 panic 恢复兜底）
func (b *EventBus) Publish(ctx context.Context, event Event) {
	b.mu.RLock()
	matched := make([]IEventListener, 0, len(b.listeners))
	for _, l := range b.listeners {
		interests := l.InterestIn()
		if len(interests) == 0 || lo.Contains(interests, event.Type) {
			matched = append(matched, l)
		}
	}
	b.mu.RUnlock()

	// 强制制作全量只读深拷贝快照，彻底杜绝异步监听器消费与主流程后续推进产生数据竞争 (Data Race)
	evtSnapshot := event.Clone()

	for _, listener := range matched {
		func(l IEventListener) {
			defer func() {
				if r := recover(); r != nil {
					// 捕获异常输出日志，防止用户自定义监听器中的 panic 导致引擎主流程崩溃
					log.Printf("[flowx.EventBus] 事件监听器执行 panic: %v, eventType=%s", r, evtSnapshot.Type)
				}
			}()
			if err := l.OnEvent(ctx, evtSnapshot); err != nil {
				log.Printf("[flowx.EventBus] 事件监听器处理失败: %v, eventType=%s", err, evtSnapshot.Type)
			}
		}(listener)
	}
}

// IEventPublisher 抽象事件发布契约
type IEventPublisher interface {
	Publish(ctx context.Context, event Event)
}

type pendingEvent struct {
	ctx   context.Context
	event Event
}

// bufferedEventPublisher 事务级事件暂存器（Outbox 模式），防止在未提交的物理事务中提前外发事件
type bufferedEventPublisher struct {
	target IEventPublisher
	mu     sync.Mutex
	events []pendingEvent
}

func newBufferedEventPublisher(target IEventPublisher) *bufferedEventPublisher {
	return &bufferedEventPublisher{
		target: target,
		events: make([]pendingEvent, 0),
	}
}

func (b *bufferedEventPublisher) Publish(ctx context.Context, event Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	// 入队即深拷贝快照：彻底切断活指针引用，保证每个事件严格保留产生时刻的因果状态（杜绝终态污染先发事件）
	b.events = append(b.events, pendingEvent{ctx: ctx, event: event.Clone()})
}

// Flush 物理事务成功提交后，将暂存事件统一有序刷出广播
func (b *bufferedEventPublisher) Flush() {
	b.mu.Lock()
	pending := b.events
	b.events = nil
	b.mu.Unlock()

	for _, pe := range pending {
		b.target.Publish(pe.ctx, pe.event)
	}
}
