package domain

import "github.com/samber/lo"

// TaskRecord 任务执行记录领域模型，承载时间线与流转历史中各节点的审批明细
type TaskRecord struct {
	TaskID       ID          // 任务实例全局强类型自适应标识 (兼容整型自增与现代字符串形态)
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

// RejectSnapshot 流程引擎导出的处于激活待响应状态的驳回上下文快照
type RejectSnapshot struct {
	Operator   string // 驳回处理人账号
	NodeID     string // 驳回环节节点标识
	NodeName   string // 驳回环节展示名称
	Comment    string // 驳回意见或修改批注
	FinishTime string // 驳回办结时间 (标准格式 YYYY-MM-DD HH:mm:ss)
}

// AuditContext 工单详情页审批面板的只读上下文聚合，让审批人在填写自己的意见前就能读到链路历史。
// 两个区块各自独立降级：任一查询失败只丢该区块，不连带工单详情主体打不开
type AuditContext struct {
	LatestReject      *LatestReject  // 最近一次激活未闭环的驳回快照，无驳回或已走回来时为 nil
	PriorComments     []PriorComment // 按办结时间倒序的最近若干条前序审批意见
	PriorCommentTotal int64          // 可见前序意见总数，供前端折叠展开
}

// PriorComment 前序审批意见摘要条目，让审批人在填写自己的意见前就能读到链路历史，
// 而不必先切到审批记录页；账号与姓名同时保留，避免同名展示姓名无法区分是否本人意见
type PriorComment struct {
	NodeID         string // 流程节点定义标识
	NodeName       string // 流程节点展示名称
	ApprovedBy     string // 处理人账号
	ApprovedByName string // 处理人展示姓名
	Status         int    // 审批裁决状态（1: 通过, 2: 驳回）
	Comment        string // 审批意见
	OccurredAt     string // 审批办结时间 (标准格式 YYYY-MM-DD HH:mm:ss)
}
