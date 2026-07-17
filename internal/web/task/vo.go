package task

import "github.com/Duke1616/eflow/internal/domain"

type Page struct {
	Offset int64 `json:"offset"`
	Limit  int64 `json:"limit"`
}

type ListTaskReq struct{ Page }

type ListTaskByInstanceIDReq struct {
	InstanceID int `json:"instance_id"`
	Page
}

type RetryReq struct {
	ID int64 `json:"id"`
}

type ListAttemptsReq struct {
	TaskID int64 `json:"task_id"`
}

type LogsReq struct {
	AttemptID int64 `json:"attempt_id"`
	MinID     int64 `json:"min_id"`
	Limit     int   `json:"limit"`
}

type Task struct {
	ID                int64  `json:"id"`
	TicketID          int64  `json:"ticket_id"`
	ProcessInstanceID int    `json:"process_instance_id"`
	NodeID            string `json:"node_id"`
	NodeName          string `json:"node_name"`
	ProcessVersion    int    `json:"process_version"`
	Status            uint8  `json:"status"`
	Phase             string `json:"phase"`
	ScheduledAt       int64  `json:"scheduled_at"`
	CurrentAttemptID  int64  `json:"current_attempt_id"`
	AdvancedAt        int64  `json:"advanced_at"`
	LastError         string `json:"last_error"`
	CTime             int64  `json:"ctime"`
	UTime             int64  `json:"utime"`
}

type Attempt struct {
	ID          int64           `json:"id"`
	TaskID      int64           `json:"task_id"`
	AttemptNo   int             `json:"attempt_no"`
	RequestID   string          `json:"request_id"`
	RunnerID    int64           `json:"runner_id"`
	ExecutionID int64           `json:"execution_id"`
	Status      string          `json:"status"`
	Input       domain.TaskArgs `json:"input"`
	Output      string          `json:"output"`
	Error       string          `json:"error"`
	SubmittedAt int64           `json:"submitted_at"`
	CompletedAt int64           `json:"completed_at"`
	CTime       int64           `json:"ctime"`
	UTime       int64           `json:"utime"`
}

type ExecutionLog struct {
	ID      int64  `json:"id"`
	Time    int64  `json:"time"`
	Content string `json:"content"`
}

type RetrieveTasks struct {
	Total int64  `json:"total"`
	Tasks []Task `json:"tasks"`
}

type ListAttemptsResp struct {
	Attempts []Attempt `json:"attempts"`
}

type LogsResp struct {
	Logs  []ExecutionLog `json:"logs"`
	MaxID int64          `json:"max_id"`
}
