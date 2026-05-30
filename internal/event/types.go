package event

const (
	ExecuteResultEventName     = "result_execute_events"
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
	TaskId     int64  `json:"task_id"`
	Result     string `json:"result"`
	WantResult string `json:"want_result"`
	Status     Status `json:"status"`
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
