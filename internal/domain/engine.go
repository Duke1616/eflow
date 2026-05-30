package domain

import (
	"time"
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
