package event

const (
	ExecuteResultEventName     = "complete_topic"
	CreateProcessEventName     = "create_process_events"
	OrderStatusModifyEventName = "order_status_modify_events"
)

type Status uint8

func (s Status) ToUint8() uint8 {
	return uint8(s)
}

const (
	SUCCESS Status = 1
	FAILED  Status = 2
)

type ExecuteResultEvent struct {
	TaskId     int64  `json:"taskId"`     // Etask 的任务 ID (若有)
	ExecID     int64  `json:"execId"`     // etask 执行实例 ID
	ExecStatus string `json:"execStatus"` // 执行状态，例如 SUCCESS / FAILED 等
	TaskResult string `json:"taskResult"` // 执行输出或日志结果
	Source     string `json:"source"`
	RequestID  string `json:"requestId"`
}

type Variables struct {
	Key    string `json:"key"`
	Value  any    `json:"value"`
	Secret bool   `json:"secret"`
}

type Provide uint8

const (
	SYSTEM Provide = 1
	WECHAT Provide = 2
	ALERT  Provide = 3
)

type TicketEvent struct {
	Id         int64                  `json:"id"`
	Provide    Provide                `json:"provide"`
	WorkflowId int64                  `json:"workflow_id"`
	Data       map[string]interface{} `json:"data"`
	Variables  string                 `json:"variables"`
}

const (
	START    uint8 = 1
	PROCESS  uint8 = 2
	END      uint8 = 3
	WITHDRAW uint8 = 4
)

type TicketStatusModifyEvent struct {
	ProcessInstanceId int   `json:"process_instance_id"`
	Status            uint8 `json:"status"`
}
