package model

import (
	"time"

	"github.com/samber/lo"
)

// InstanceStatus 流程实例运行状态
type InstanceStatus uint8

const (
	InstanceRunning   InstanceStatus = 1 // 流程流转中
	InstanceFinished  InstanceStatus = 2 // 流程正常圆满办结
	InstanceRejected  InstanceStatus = 3 // 流程已被驳回终止
	InstanceCancelled InstanceStatus = 4 // 流程已被发起人撤回
)

// Instance 流程运行实例
type Instance struct {
	ID          string         `json:"id"`
	ProcKey     string         `json:"proc_key"`
	Version     int            `json:"version"`
	Starter     string         `json:"starter"`
	Status      InstanceStatus `json:"status"`
	CurrentNode string         `json:"current_node"` // 当前停留的节点
	Variables   map[string]any `json:"variables"`    // 全局表单变量快照
	CreateTime  time.Time      `json:"create_time"`
	FinishTime  *time.Time     `json:"finish_time,omitempty"`
}

// Finish 将流程实例正常办结并记录完成时间与最终节点
func (i *Instance) Finish(endNodeID string) {
	i.Status = InstanceFinished
	i.CurrentNode = endNodeID
	i.FinishTime = lo.ToPtr(time.Now())
}

// Reject 将流程实例置为驳回终止态并记录结束时间
func (i *Instance) Reject() {
	i.Status = InstanceRejected
	i.FinishTime = lo.ToPtr(time.Now())
}

// Cancel 将流程实例置为撤回/作废态并记录结束时间
func (i *Instance) Cancel() {
	i.Status = InstanceCancelled
	i.FinishTime = lo.ToPtr(time.Now())
}

// IsRunning 判断流程实例是否处于流转运行中
func (i *Instance) IsRunning() bool {
	return i.Status == InstanceRunning
}

// IsFinished 判断流程实例是否已正常圆满办结
func (i *Instance) IsFinished() bool {
	return i.Status == InstanceFinished
}

// IsRejected 判断流程实例是否已被驳回终结
func (i *Instance) IsRejected() bool {
	return i.Status == InstanceRejected
}

// IsCancelled 判断流程实例是否已被发起人撤回作废
func (i *Instance) IsCancelled() bool {
	return i.Status == InstanceCancelled
}

// IsTerminal 判断流程实例是否已达到终态 (非 Running 即为终态)
func (i *Instance) IsTerminal() bool {
	return i.Status != InstanceRunning
}

// MergeVariables 增量合并业务表单变量，保证变量字典初始化安全
func (i *Instance) MergeVariables(patch map[string]any) {
	if len(patch) == 0 {
		return
	}
	if i.Variables == nil {
		i.Variables = make(map[string]any, len(patch))
	}
	for k, v := range patch {
		i.Variables[k] = v
	}
}
