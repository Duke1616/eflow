package ticket

import (
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/idx"
)

// TaskID 审批任务强类型自适应标识，直接对齐底层通用包 pkg/idx.ID
type TaskID = idx.ID

// InstanceID 流程实例强类型自适应标识，直接对齐底层通用包 pkg/idx.ID
type InstanceID = idx.ID

type CreateTicketReq struct {
	TemplateId int64                  `json:"template_id"`
	WorkflowId int64                  `json:"workflow_id"`
	Data       map[string]interface{} `json:"data"`
}

type DetailProcessInstIdReq struct {
	ProcessInstanceId idx.ID `json:"process_instance_id"`
}

// RestartProcessReq 表示重新启动指定工单流程的请求参数。
type RestartProcessReq struct {
	TicketID int64 `json:"ticket_id" validate:"required"`
}

type RecordTaskReq struct {
	ProcessInstId idx.ID `json:"process_inst_id"`
	Offset        int64  `json:"offset"`
	Limit         int64  `json:"limit"`
}

// TaskTimelineReq 按节点执行批次分页查询工单流转时间线。
type TaskTimelineReq struct {
	ProcessInstId idx.ID `json:"process_inst_id"`
	Offset        int64  `json:"offset"`
	Limit         int64  `json:"limit"`
}

type Todo struct {
	UserId      string `json:"user_id" validate:"required"`
	ProcessName string `json:"process_name"`
	SortByAsc   bool   `json:"sort_by_asc"`
	Offset      int64  `json:"offset"`
	Limit       int64  `json:"limit"`
}

type HistoryReq struct {
	UserId string `json:"user_id"`
	Offset int64  `json:"offset"`
	Limit  int64  `json:"limit"`
}

type StartUserReq struct {
	Offset int64 `json:"offset"`
	Limit  int64 `json:"limit"`
}

type PassOrderReq struct {
	TicketId  int64                  `json:"ticket_id"`
	TaskId    TaskID                 `json:"task_id"`
	Comment   string                 `json:"comment"`
	ExtraData map[string]interface{} `json:"extra_data"`
}

type RejectOrderReq struct {
	TicketId     int64  `json:"ticket_id"`
	TaskId       TaskID `json:"task_id"`
	Comment      string `json:"comment"`
	TargetNodeId string `json:"target_node_id,omitempty"`
	Terminate    bool   `json:"terminate,omitempty"`
}

type RejectTargetsReq struct {
	TicketId int64  `json:"ticket_id"`
	TaskId   TaskID `json:"task_id"`
}

type RejectTargetVO struct {
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	NodeType  string `json:"node_type"`
	IsStarter bool   `json:"is_starter"`
}

type TransferReq struct {
	TicketId  int64    `json:"ticket_id"`
	TaskId    TaskID   `json:"task_id"`
	Usernames []string `json:"usernames"`
}

type RevokeOrderReq struct {
	InstanceId idx.ID `json:"instance_id"`
	Force      bool   `json:"force"`
	Reason     string `json:"reason"`
}

type SubmitRatingReq struct {
	TicketID int64  `json:"ticket_id"`
	Score    uint8  `json:"score"`
	Comment  string `json:"comment"`
}

type TaskFormConfigReq struct {
	TicketId   int64  `json:"ticket_id"`
	WorkflowId int64  `json:"workflow_id"`
	TaskId     TaskID `json:"task_id"`
}

type TaskFormConfigResp struct {
	Fields []flowdsl.Field        `json:"fields"`
	Values map[string]interface{} `json:"values"`
}

// LatestRejectVO 工单最近一次被驳回的展示视图
type LatestRejectVO struct {
	RejectedBy     string `json:"rejected_by"`      // 驳回人账号
	RejectedByName string `json:"rejected_by_name"` // 驳回人展示姓名
	NodeID         string `json:"node_id"`          // 驳回节点标识
	NodeName       string `json:"node_name"`        // 驳回环节名称
	Reason         string `json:"reason"`           // 驳回原因/修改意见
	RejectedAt     string `json:"rejected_at"`      // 驳回时间
}

// PriorCommentsVO 审批面板的前序意见摘要：Items 是最近若干条，Total 是链路意见总数（供前端折叠提示）
type PriorCommentsVO struct {
	Items []PriorCommentVO `json:"items"`
	Total int64            `json:"total"`
}

// PriorCommentVO 单条前序审批意见的展示视图
type PriorCommentVO struct {
	NodeID         string `json:"node_id"`
	NodeName       string `json:"node_name"`
	ApprovedBy     string `json:"approved_by"`
	ApprovedByName string `json:"approved_by_name"`
	Status         int    `json:"status"`
	Comment        string `json:"comment"`
	OccurredAt     string `json:"occurred_at"`
}

type RetrieveTickets struct {
	Total int64    `json:"total"`
	Tasks []Ticket `json:"tasks"`
}

type RetrieveTaskRecords struct {
	TaskRecords []TaskRecord `json:"task_records"`
	Total       int64        `json:"total"`
}

// RetrieveTaskTimeline 是面向时间线展示的聚合结果；Total 统计节点执行批次数，而非任务数。
type RetrieveTaskTimeline struct {
	Events []TaskTimelineEvent `json:"events"`
	Total  int64               `json:"total"`
}

// TaskTimelineEvent 将同一节点、同一批次的多人任务合并为一个顶层时间线事件。
type TaskTimelineEvent struct {
	ID         string              `json:"id"`
	NodeID     string              `json:"node_id"`
	NodeName   string              `json:"node_name"`
	BatchCode  string              `json:"batch_code,omitempty"`
	IsCosigned int                 `json:"is_cosigned"`
	OccurredAt string              `json:"occurred_at"`
	Actors     []string            `json:"actors"`
	Summary    TaskTimelineSummary `json:"summary"`
	Members    []TaskRecord        `json:"members"`
}

// TaskTimelineSummary 给前端提供无需重新解释引擎状态码的汇总信息。
type TaskTimelineSummary struct {
	Total          int `json:"total"`
	Passed         int `json:"passed"`
	Rejected       int `json:"rejected"`
	SystemPassed   int `json:"system_passed"`
	SystemRejected int `json:"system_rejected"`
	Skipped        int `json:"skipped"`
	Linked         int `json:"linked"`
	Pending        int `json:"pending"`
}

type TaskRecord struct {
	NodeName     string      `json:"node_name"`
	ApprovedBy   string      `json:"approved_by"`
	IsCosigned   int         `json:"is_cosigned"`
	Status       int         `json:"status"`
	Comment      string      `json:"comment"`
	IsFinished   int         `json:"is_finished"`
	FinishedTime interface{} `json:"finished_time"`
	FormValues   []FormValue `json:"form_values"`
}

type Ticket struct {
	Id                   int64                  `json:"id"`
	EngineType           string                 `json:"engine_type"`
	TaskId               TaskID                 `json:"task_id"`
	TemplateId           int64                  `json:"template_id"`
	Starter              string                 `json:"starter"`
	Status               uint8                  `json:"status"`
	Provide              uint8                  `json:"provide"`
	ProcessInstanceId    InstanceID             `json:"process_instance_id"`
	WorkflowId           int64                  `json:"workflow_id"`
	Ctime                string                 `json:"ctime"`
	Wtime                string                 `json:"wtime"`
	Data                 map[string]interface{} `json:"data"`
	CurrentStep          string                 `json:"current_step"`
	ApprovedBy           string                 `json:"approved_by"`
	ProcInstCreateTime   interface{}            `json:"proc_inst_create_time"`
	CanRate              bool                   `json:"can_rate"`
	Rating               *Rating                `json:"rating,omitempty"`
	RevokeReason         string                 `json:"revoke_reason,omitempty"`
	ProcessStartError    string                 `json:"process_start_error,omitempty"`
	ProcessStartAttempts int                    `json:"process_start_attempts,omitempty"`
	LatestReject         *LatestRejectVO        `json:"latest_reject,omitempty"`
	PriorComments        *PriorCommentsVO       `json:"prior_comments,omitempty"`
	FlowPolicy           *FlowPolicyVO          `json:"flow_policy,omitempty"`
}

// FlowPolicyVO 流程审批与流转治理策略视图模型
type FlowPolicyVO struct {
	AllowTransfer bool     `json:"allow_transfer"`
	AllowReject   bool     `json:"allow_reject"`
	RejectModes   []string `json:"reject_modes"`
}

type Rating struct {
	Score   uint8  `json:"score"`
	Comment string `json:"comment"`
	Rater   string `json:"rater"`
	RatedAt int64  `json:"rated_at"`
}

type FormValue struct {
	Name  string      `json:"name"`
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}
