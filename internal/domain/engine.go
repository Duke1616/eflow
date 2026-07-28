package domain

import (
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/database"
)

// Instance 流程在引擎中运行流转的实例领域模型
// engine 模块下的 Instance。代表一个正在引擎中流转或已结束的流程实例快照。
type Instance struct {
	TaskID          int        // 任务 ID
	ProcInstID      int        // 流程实例 ID
	ProcID          int        // 流程 ID
	ProcName        string     // 流程名称
	ProcVersion     int        // 流程版本号
	BusinessID      string     // 关联的工单系统生成的业务唯一单号（对应 Ticket 的 Key 或是 ID）
	Starter         string     // 流程发起人显示名称或用户 ID
	CurrentNodeID   string     // 流程当前正处在的节点物理 ID
	CurrentNodeName string     // 流程当前正处在的节点展示名称
	CreateTime      *time.Time // 实例整体的发起/创建时间
	ApprovedBy      string     // 当前处理人
	Status          int        // 实例状态（0: 未完成审批中, 1: 已完成通过, 2: 撤销）
}

// TaskTimelineGroup 是面向工单流转时间线的节点执行汇总。
// 同一节点在同一批次内会为多个处理人创建任务；时间线以该组为展示单位，
// 而不是将每个处理人任务都渲染为独立的顶层事件。
type TaskTimelineGroup struct {
	NodeID              string             // 流程节点 ID
	NodeName            string             // 节点展示名称
	BatchCode           string             // 节点执行批次；驳回重走会产生新批次
	IsCosigned          int                // 0: 或签，1: 会签
	TaskCount           int                // 该节点批次中的任务总数
	PassedCount         int                // 人工通过任务数
	RejectedCount       int                // 人工驳回任务数
	SystemPassedCount   int                // 因联动被系统通过的任务数
	SystemRejectedCount int                // 因联动被系统驳回的任务数
	SkippedCount        int                // 条件不满足而跳过的任务数
	LinkedCount         int                // 节点或签联动结束、但未改变原始状态的任务数
	PendingCount        int                // 尚未完成的任务数
	OccurredAt          database.LocalTime // 本次节点执行的最新活动时间
}
