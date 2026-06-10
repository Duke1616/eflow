package dispatch

type CreateDispatchReq struct {
	TemplateId int64  `json:"template_id"`
	RunnerId   int64  `json:"runner_id"`
	Field      string `json:"field"`
	Value      string `json:"value"`
}

type UpdateDispatchReq struct {
	Id       int64  `json:"id"`
	RunnerId int64  `json:"runner_id"`
	Field    string `json:"field"`
	Value    string `json:"value"`
}

type Page struct {
	Offset int64 `json:"offset,omitempty"`
	Limit  int64 `json:"limit,omitempty"`
}

type ListByTemplateId struct {
	Page
	TemplateId int64 `json:"template_id"`
}

type Dispatch struct {
	Id         int64  `json:"id"`
	TemplateId int64  `json:"template_id"`
	RunnerId   int64  `json:"runner_id"`
	Field      string `json:"field"`
	Value      string `json:"value"`
}

type RetrieveDispatches struct {
	Total      int64      `json:"total"`
	Dispatches []Dispatch `json:"dispatches"`
}

type SyncDispatchReq struct {
	TemplateId      int64 `json:"template_id"`
	TemplateGroupId int64 `json:"template_group_id"`
	SyncTemplateId  int64 `json:"sync_template_id"`
}

type DeleteDispatchReq struct {
	Id int64 `json:"id"`
}
