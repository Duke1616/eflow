package ticket

import (
	"strconv"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
)

// TaskID 审批任务强类型标识，自动兼容 JSON 中的整型数字（如 102）与字符串（如 "task_xxx"）
type TaskID string

// UnmarshalJSON 统一解析 JSON 格式的任务 ID，兼顾数字与字符串形态
func (t *TaskID) UnmarshalJSON(b []byte) error {
	raw := strings.Trim(string(b), "\"")
	*t = TaskID(raw)
	return nil
}

// String 返回字符串形态，空指针安全保护
func (t *TaskID) String() string {
	if t == nil {
		return ""
	}
	return string(*t)
}

// Int 安全解析为整型，失败返回 0，空指针安全保护
func (t *TaskID) Int() int {
	if t == nil {
		return 0
	}
	i, _ := strconv.Atoi(string(*t))
	return i
}

type CreateTicketReq struct {
	TemplateId int64                  `json:"template_id"`
	WorkflowId int64                  `json:"workflow_id"`
	Data       map[string]interface{} `json:"data"`
	CreateBy   string                 `json:"create_by"`
}

type DetailProcessInstIdReq struct {
	ProcessInstanceId domain.ID `json:"process_instance_id"`
}

// RestartProcessReq 表示重新启动指定工单流程的请求参数。
type RestartProcessReq struct {
	TicketID int64 `json:"ticket_id" validate:"required"`
}

type RecordTaskReq struct {
	ProcessInstId domain.ID `json:"process_inst_id"`
	Offset        int64     `json:"offset"`
	Limit         int64     `json:"limit"`
}

// TaskTimelineReq 按节点执行批次分页查询工单流转时间线。
type TaskTimelineReq struct {
	ProcessInstId domain.ID `json:"process_inst_id"`
	Offset        int64     `json:"offset"`
	Limit         int64     `json:"limit"`
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
	TaskId    TaskID                 `json:"task_id"`
	Comment   string                 `json:"comment"`
	ExtraData map[string]interface{} `json:"extra_data"`
}

type RejectOrderReq struct {
	TaskId  TaskID `json:"task_id"`
	Comment string `json:"comment"`
}

type TransferReq struct {
	TaskId    TaskID   `json:"task_id"`
	Usernames []string `json:"usernames"`
}

type RevokeOrderReq struct {
	InstanceId domain.ID `json:"instance_id"`
	Force      bool      `json:"force"`
	Reason     string    `json:"reason"`
}

type SubmitRatingReq struct {
	TicketID int64  `json:"ticket_id"`
	Score    uint8  `json:"score"`
	Comment  string `json:"comment"`
}

type TaskFormConfigReq struct {
	WorkflowId int64  `json:"workflow_id"`
	TaskId     TaskID `json:"task_id"`
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
	TaskId               any                    `json:"task_id"`
	TemplateId           int64                  `json:"template_id"`
	Starter              string                 `json:"starter"`
	Status               uint8                  `json:"status"`
	Provide              uint8                  `json:"provide"`
	ProcessInstanceId    int                    `json:"process_instance_id"`
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
