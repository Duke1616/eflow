package engine

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
)

//go:generate mockgen -source=./interface.go -package=enginemocks -destination=./mocks/engine.mock.go IEngine,IDriver

// IProcessLauncher 面向工单创建消费者（ProcessEventConsumer）的最小启动器接口
type IProcessLauncher interface {
	Start(ctx context.Context, req domain.ProcessLaunchReq) (domain.ProcessLaunchResult, error)
}

// IEngine 面向业务层的工作流引擎统一门面契约
// 业务层（Ticket / Workflow / Withdrawal / Task / Channel）统一注入并调用本接口，无需感知底层引擎细节。
// engineType 是门面独有的「寻址路由键」：门面据此挑选驱动，随后不再向下透传。
// 只有 engineDispatcher 实现本接口；具体引擎驱动实现的是 IDriver。
type IEngine interface {
	IProcessLauncher

	// Deploy 部署/发布流程定义图 DSL，返回发布后的版本元数据
	Deploy(ctx context.Context, engineType domain.EngineType, rawDef any) (domain.DeployInfo, error)

	// Pass 审批通过特定任务，推进流程向下流转
	Pass(ctx context.Context, req domain.TaskPassReq) error

	// Reject 审批驳回特定任务，触发流程回退或终结
	Reject(ctx context.Context, req domain.TaskRejectReq) error

	// Transfer 将特定审批任务转签指派给新目标用户
	Transfer(ctx context.Context, req domain.TaskTransferReq) error

	// Revoke 撤销/终止特定流程实例
	Revoke(ctx context.Context, req domain.RevokeReq) error

	// GetTaskContext 获取指定流程引擎下特定任务的轻量上下文快照
	GetTaskContext(ctx context.Context, engineType domain.EngineType, taskID domain.ID) (domain.TaskContext, error)

	// GetTimeline 获取指定流程实例的时间线流转历史节点列表
	GetTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) ([]domain.TimelineNode, error)

	// GetAutomationTask 获取流程实例在当前自动化节点下待执行的任务记录
	GetAutomationTask(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, nodeID string) (domain.TaskRecord, error)

	// GetTasksByCurrentNode 获取指定流程实例在特定节点下当前挂起/未完成的任务记录集合
	GetTasksByCurrentNode(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error)

	// GetInstanceExecutionView 获取指定流程实例的运行态点亮视图（含已流转边与节点状态映射）
	GetInstanceExecutionView(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) (domain.InstanceExecutionView, error)

	// ListTodoTasks 分页检索指派给当前用户的待办任务列表，由具体引擎适配实现，分发器支持跨引擎并发聚合
	ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error)

	// ListPendingSteps 定向批量获取指定流程实例列表中当前挂起/待处理的步骤信息（基于工单绑定的 EngineType 精准路由，避免全引擎盲查）
	ListPendingSteps(ctx context.Context, processes []domain.Process) ([]domain.TodoTask, error)

	// GetTaskRecords 分页获取指定流程实例的流转执行记录
	GetTaskRecords(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error)

	// GetTaskTimeline 分页获取指定流程实例的聚合时间线事件
	GetTaskTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error)

	// GetRejectTargets 获取指定任务节点在当前流转状态下允许驳回的目标节点候选列表
	GetRejectTargets(ctx context.Context, engineType domain.EngineType, taskID domain.ID) ([]domain.RejectTarget, error)

	// GetActiveReject 获取流程实例当前正处于待处理/待响应状态的最新驳回上下文（若无驳回或已重新推进闭环，返回 nil）
	GetActiveReject(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) (*domain.RejectSnapshot, error)

	// GetPriorComments 精确获取流程实例最近 limit 条人工裁决意见与意见总数，按办结时间倒序。
	// 与 GetTaskRecords 的区别：过滤与排序下推到各引擎自身存储，调用方无需全量取回再内存排序
	GetPriorComments(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, limit int) ([]domain.PriorComment, int64, error)
}

// IDriver 底层流程引擎驱动 SPI（Service Provider Interface），由 flowx / easyflow 适配层实现
// 与 IEngine 门面的本质区别：驱动天然知道自己的身份（见 EngineType），因此契约中严禁出现 engineType 路由参数。
// 设计约束：把门面签名直接复用为 SPI，会迫使每个驱动接收一个对自身恒定的同义反复参数再丢弃它，
// 既污染实现签名，又让「传错引擎类型」这类缺陷只能在运行期靠试探兜底暴露。
type IDriver interface {
	IProcessLauncher

	// EngineType 返回当前驱动的类型标识，供分发器注册与寻址
	EngineType() domain.EngineType

	// Deploy 部署/发布流程定义图 DSL，返回发布后的版本元数据
	Deploy(ctx context.Context, rawDef any) (domain.DeployInfo, error)

	// Pass 审批通过特定任务，推进流程向下流转
	Pass(ctx context.Context, req domain.TaskPassReq) error

	// Reject 审批驳回特定任务，触发流程回退或终结
	Reject(ctx context.Context, req domain.TaskRejectReq) error

	// Transfer 将特定审批任务转签指派给新目标用户
	Transfer(ctx context.Context, req domain.TaskTransferReq) error

	// Revoke 撤销/终止特定流程实例
	Revoke(ctx context.Context, req domain.RevokeReq) error

	// GetTaskContext 获取特定任务的轻量上下文快照
	GetTaskContext(ctx context.Context, taskID domain.ID) (domain.TaskContext, error)

	// GetTimeline 获取指定流程实例的时间线流转历史节点列表
	GetTimeline(ctx context.Context, instanceID domain.ID) ([]domain.TimelineNode, error)

	// GetAutomationTask 获取流程实例在当前自动化节点下待执行的任务记录
	GetAutomationTask(ctx context.Context, instanceID domain.ID, nodeID string) (domain.TaskRecord, error)

	// GetTasksByCurrentNode 获取指定流程实例在特定节点下当前挂起/未完成的任务记录集合
	GetTasksByCurrentNode(ctx context.Context, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error)

	// GetInstanceExecutionView 获取指定流程实例的运行态点亮视图（含已流转边与节点状态映射）
	GetInstanceExecutionView(ctx context.Context, instanceID domain.ID) (domain.InstanceExecutionView, error)

	// ListTodoTasks 分页检索指派给当前用户的待办任务列表
	ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error)

	// ListPendingSteps 定向批量获取本驱动所辖流程实例列表中当前挂起/待处理的步骤信息
	ListPendingSteps(ctx context.Context, processes []domain.Process) ([]domain.TodoTask, error)

	// GetTaskRecords 分页获取指定流程实例的流转执行记录
	GetTaskRecords(ctx context.Context, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error)

	// GetTaskTimeline 分页获取指定流程实例的聚合时间线事件
	GetTaskTimeline(ctx context.Context, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error)

	// GetRejectTargets 获取指定任务节点在当前流转状态下允许驳回的目标节点候选列表
	GetRejectTargets(ctx context.Context, taskID domain.ID) ([]domain.RejectTarget, error)

	// GetActiveReject 获取流程实例当前正处于待处理/待响应状态的最新驳回上下文（若无驳回或已重新推进闭环，返回 nil）
	GetActiveReject(ctx context.Context, instanceID domain.ID) (*domain.RejectSnapshot, error)

	// GetPriorComments 精确获取最近 limit 条人工裁决意见与意见总数，按办结时间倒序。
	// 过滤、排序与时间格式化必须在本驱动内完成：返回的 OccurredAt 已是标准字符串，
	// 调用方不该再感知各引擎的时间形态差异；ApprovedByName 留空由服务层解析真名填充
	GetPriorComments(ctx context.Context, instanceID domain.ID, limit int) ([]domain.PriorComment, int64, error)
}
