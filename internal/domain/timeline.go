package domain

import "github.com/samber/lo"

// TaskRecord 任务执行记录领域模型，承载时间线与流转历史中各节点的审批明细
type TaskRecord struct {
	TaskID       int         // 任务实例标识
	NodeID       string      // 流程节点定义标识
	PrevNodeID   string      // 上游前驱节点标识
	BatchCode    string      // 同节点流转批次号（用于会签/循环聚合）
	NodeName     string      // 流程节点展示名称
	ApprovedBy   string      // 任务指派人或审批人标识
	IsCosigned   int         // 是否会签任务（0: 否, 1: 是）
	Status       int         // 审批裁决状态（0: 待办, 1: 通过, 2: 驳回）
	Comment      string      // 审批意见或流转批注
	IsFinished   int         // 任务是否已物理办结（0: 否, 1: 是）
	FinishedTime interface{} // 审批完结时间快照
	FormValues   []FormValue // 当前节点填写的动态表单数据快照
}

// IsAudited 断言该流转步骤是否已由处理人执行实际审批裁决（1: 通过, 2: 驳回）
func (r TaskRecord) IsAudited() bool {
	return r.Status == 1 || r.Status == 2
}

// IsApproved 断言该流转步骤是否审批通过
func (r TaskRecord) IsApproved() bool {
	return r.Status == 1
}

// IsRejected 断言该流转步骤是否被驳回
func (r TaskRecord) IsRejected() bool {
	return r.Status == 2
}

// IsFinishedTask 断言该流转步骤是否已物理归档完结
func (r TaskRecord) IsFinishedTask() bool {
	return r.IsFinished == 1
}

// TaskTimelineEvent 节点级时间线聚合事件领域模型（充血实体）
type TaskTimelineEvent struct {
	ID         string
	NodeID     string
	NodeName   string
	BatchCode  string
	IsCosigned int
	OccurredAt string
	Actors     []string
	Summary    TaskTimelineSummary
	Members    []TaskRecord
}

// BuildID 根据节点与批次代码计算全局唯一的事件标识
func (e TaskTimelineEvent) BuildID() string {
	if e.BatchCode == "" {
		return e.NodeID
	}
	return e.NodeID + ":" + e.BatchCode
}

// ResolveActors 提取当前节点批次下实际参与过审批裁决的处理人名单，自动过滤并去重
func (e TaskTimelineEvent) ResolveActors() []string {
	auditedMembers := lo.Filter(e.Members, func(r TaskRecord, _ int) bool {
		return r.IsAudited()
	})
	return lo.Uniq(lo.FilterMap(auditedMembers, func(r TaskRecord, _ int) (string, bool) {
		return r.ApprovedBy, r.ApprovedBy != ""
	}))
}

// NewTaskTimelineEvent 工厂方法：自适应生成聚合事件，内聚计算 ID 与参与人名单
func NewTaskTimelineEvent(
	nodeID, nodeName, batchCode string,
	isCosigned int,
	occurredAt string,
	summary TaskTimelineSummary,
	members []TaskRecord,
) TaskTimelineEvent {
	event := TaskTimelineEvent{
		NodeID:     nodeID,
		NodeName:   nodeName,
		BatchCode:  batchCode,
		IsCosigned: isCosigned,
		OccurredAt: occurredAt,
		Summary:    summary,
		Members:    members,
	}
	event.ID = event.BuildID()
	event.Actors = event.ResolveActors()
	return event
}

// TaskTimelineSummary 时间线状态汇总统计
type TaskTimelineSummary struct {
	Total          int
	Passed         int
	Rejected       int
	SystemPassed   int
	SystemRejected int
	Skipped        int
	Linked         int
	Pending        int
}
