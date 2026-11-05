package engine

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
)

//go:generate mockgen -source=./adapter.go -package=enginemocks -destination=./mocks/adapter.mock.go IEngineAdapter

// IEngineAdapter 工作流引擎底层驱动契约（多引擎可扩展核心）
// 无论是自研 flowx 还是遗留 easy-workflow，只需实现该统一驱动接口
type IEngineAdapter interface {
	// EngineType 返回当前驱动对应的引擎类型
	EngineType() domain.EngineType

	// Deploy 部署/发布流程定义图 DSL，返回发布后的元数据
	Deploy(ctx context.Context, rawDef any) (DeployInfo, error)

	// Start 启动流程实例，激活初始节点
	Start(ctx context.Context, req ProcessLaunchReq) (ProcessLaunchResult, error)

	// Pass 审批通过特定任务，推进流程向下流转
	Pass(ctx context.Context, req TaskPassReq) error

	// Reject 审批驳回特定任务，触发流程回退或终结
	Reject(ctx context.Context, req TaskRejectReq) error

	// GetTaskContext 获取指定任务的轻量上下文快照（供业务鉴权与表单定位）
	GetTaskContext(ctx context.Context, taskID ID) (TaskContext, error)

	// GetTimeline 获取指定流程实例的时间线流转历史节点列表
	GetTimeline(ctx context.Context, instanceID ID) ([]TimelineNode, error)

	// Revoke 撤销/终止特定流程实例
	Revoke(ctx context.Context, instanceID ID, force bool, operator string) error

	// GetInstanceExecutionView 获取指定流程实例的运行态视图（含版本快照索引与已流转边集合）
	GetInstanceExecutionView(ctx context.Context, instanceID ID, status uint8) (InstanceExecutionView, error)
}

// InstanceExecutionView 流程实例运行时轨迹与点亮视图
type InstanceExecutionView struct {
	ProcessID      int
	ProcessVersion int
	NodeStatusMap  map[string]int      // 节点状态映射 [NodeID] -> 状态编码
	TraversedEdges map[string][]string // 已激活流转的拓扑边映射 [SourceID] -> []TargetID
}
