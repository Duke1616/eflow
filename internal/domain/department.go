package domain

// Department 流程节点审批人中主管/分管领导解析所需的本地部门领域模型
type Department struct {
	Id         int64    `json:"id"`
	Pid        int64    `json:"pid"`
	Name       string   `json:"name"`
	Sort       int64    `json:"sort"`
	Enabled    bool     `json:"enabled"`
	Leaders    []string `json:"leaders"`
	MainLeader string   `json:"main_leader"`
}
