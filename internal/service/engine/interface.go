package engine

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
)

//go:generate mockgen -source=./interface.go -package=enginemocks -destination=./mocks/engine.mock.go IEngine

// IProcessLauncher 面向工单创建消费者（ProcessEventConsumer）的最小启动器接口
type IProcessLauncher interface {
	Start(ctx context.Context, req domain.ProcessLaunchReq) (domain.ProcessLaunchResult, error)
}

// IEngine 工作流引擎统一接口契约
// 具体流程引擎（如 flowx、easyflow）以及顶层的聚合分发器（engineDispatcher）均实现此接口。
// 业务层（Ticket / Workflow / Withdrawal）统一注入并调用 IEngine，无需感知底层引擎细节。
type IEngine interface {
	IProcessLauncher

	// EngineType 返回当前引擎的类型标识（分发器本身无单一类型，可返回空）
	EngineType() domain.EngineType

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

	// GetTaskContext 获取指定任务的轻量上下文快照（含所属引擎 EngineType、状态、处理人等）
	GetTaskContext(ctx context.Context, taskID domain.ID) (domain.TaskContext, error)

	// GetTimeline 获取指定流程实例的时间线流转历史节点列表
	GetTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID) ([]domain.TimelineNode, error)

	// GetAutomationTask 获取流程实例在当前自动化节点下待执行的任务记录
	GetAutomationTask(ctx context.Context, instanceID domain.ID, nodeID string) (domain.TaskRecord, error)

	// GetInstanceExecutionView 获取指定流程实例的运行态点亮视图（含已流转边与节点状态映射）
	GetInstanceExecutionView(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, status uint8) (domain.InstanceExecutionView, error)

	// ListTodoTasks 分页检索指派给当前用户的待办任务列表，由具体引擎适配实现，分发器支持跨引擎并发聚合
	ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error)

	// ListPendingSteps 批量获取指定流程实例列表中当前挂起/待处理的步骤信息（供“我发起的工单”展开当前处理人与节点）
	ListPendingSteps(ctx context.Context, instanceIDs []domain.ID) ([]domain.TodoTask, error)

	// GetTaskRecords 分页获取指定流程实例的流转执行记录
	GetTaskRecords(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error)

	// GetTaskTimeline 分页获取指定流程实例的聚合时间线事件
	GetTaskTimeline(ctx context.Context, engineType domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error)
}
