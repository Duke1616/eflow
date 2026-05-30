package ticket

type CreateTicketReq struct {
	TemplateId int64                  `json:"template_id"`
	WorkflowId int64                  `json:"workflow_id"`
	Data       map[string]interface{} `json:"data"`
	CreateBy   string                 `json:"create_by"`
}

type DetailProcessInstIdReq struct {
	ProcessInstanceId int `json:"process_instance_id"`
}

type RecordTaskReq struct {
	ProcessInstId int   `json:"process_inst_id"`
	Offset        int64 `json:"offset"`
	Limit         int64 `json:"limit"`
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
	TaskId    int                    `json:"task_id"`
	Comment   string                 `json:"comment"`
	ExtraData map[string]interface{} `json:"extra_data"`
}

type RejectOrderReq struct {
	TaskId  int    `json:"task_id"`
	Comment string `json:"comment"`
}

type TransferReq struct {
	TaskId    int      `json:"task_id"`
	Usernames []string `json:"usernames"`
}

type RevokeOrderReq struct {
	InstanceId int    `json:"instance_id"`
	Force      bool   `json:"force"`
}

type TaskFormConfigReq struct {
	WorkflowId int64 `json:"workflow_id"`
	TaskId     int   `json:"task_id"`
}

type RetrieveTickets struct {
	Total int64    `json:"total"`
	Tasks []Ticket `json:"tasks"`
}

type RetrieveTaskRecords struct {
	TaskRecords []TaskRecord `json:"task_records"`
	Total       int64        `json:"total"`
}

type TaskRecord struct {
	Nodename     string      `json:"node_name"`
	ApprovedBy   string      `json:"approved_by"`
	IsCosigned   int         `json:"is_cosigned"`
	Status       int         `json:"status"`
	Comment      string      `json:"comment"`
	IsFinished   int         `json:"is_finished"`
	FinishedTime interface{} `json:"finished_time"`
	FormValues   []FormValue `json:"form_values"`
}

type Ticket struct {
	Id                 int64                  `json:"id"`
	TaskId             int                    `json:"task_id"`
	TemplateId         int64                  `json:"template_id"`
	Starter            string                 `json:"starter"`
	Status             uint8                  `json:"status"`
	Provide            uint8                  `json:"provide"`
	ProcessInstanceId  int                    `json:"process_instance_id"`
	WorkflowId         int64                  `json:"workflow_id"`
	Ctime              string                 `json:"ctime"`
	Wtime              string                 `json:"wtime"`
	Data               map[string]interface{} `json:"data"`
	CurrentStep        string                 `json:"current_step"`
	ApprovedBy         string                 `json:"approved_by"`
	ProcInstCreateTime interface{}            `json:"proc_inst_create_time"`
}

type FormValue struct {
	Name  string      `json:"name"`
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

type Steps struct {
	CurrentStep string   `json:"current_step"`
	ApprovedBy  []string `json:"approved_by"`
}

type ProgressReq struct {
	TargetUrl string `json:"target_url"`
}

type Rule struct {
	Type  string `json:"type"`
	Field string `json:"field"`
	Title string `json:"title"`
}
