package workflow

// CreateReq 创建工作流流程模板请求参数封装
type CreateReq struct {
	TemplateId   int64      `json:"template_id"`   // 挂载绑定的工单模板 ID
	Name         string     `json:"name"`          // 工作流展示名字
	Icon         string     `json:"icon"`          // 画布展现图标
	Owner        string     `json:"owner"`         // 流程负责人邮箱
	Desc         string     `json:"desc"`          // 流程作用详细描述
	IsNotify     bool       `json:"is_notify"`     // 节点流转时是否触发推送通知
	NotifyMethod uint8      `json:"notify_method"` // 流程的默认第一通知渠道类型
	FlowData     *LogicFlow `json:"flow_data,omitempty"` // 画布原始图结构数据 (可选)
}

// Page 通用分页请求封装
type Page struct {
	Offset int64 `json:"offset,omitempty"` // 查询偏移量
	Limit  int64 `json:"limit,omitempty"`  // 每页数据大小限制
}

// ListReq 分页检索工作流列表请求参数
type ListReq struct {
	Page
}

// ByKeywordReq 按照关键字模糊查询工作流列表请求参数
type ByKeywordReq struct {
	Keyword string `json:"keyword"` // 模糊查询关键字 (匹配名字或描述)
	Page
}

// DeployReq 引擎流程发布请求参数
type DeployReq struct {
	Id int64 `json:"id"` // 流程模板定义的主键 ID
}

// UpdateReq 更新工作流流程配置请求参数封装
type UpdateReq struct {
	Id           int64      `json:"id"`            // 更新的工作流唯一自增 ID
	Name         string     `json:"name"`          // 流程名称
	Desc         string     `json:"desc"`          // 流程描述
	Owner        string     `json:"owner"`         // 流程管理员/设计者
	IsNotify     bool       `json:"is_notify"`     // 是否支持流转通知推送
	NotifyMethod uint8      `json:"notify_method"` // 推送渠道
	FlowData     *LogicFlow `json:"flow_data,omitempty"` // 图画布原始节点拓扑 json
}

// DeleteReq 删除流程定义请求参数
type DeleteReq struct {
	Id int64 `json:"id"` // 被删除的工作流模板唯一自增 ID
}

// Workflow 表现层统一输出的 Workflow 数据模型
type Workflow struct {
	Id           int64      `json:"id"`            // 唯一 ID
	TemplateId   int64      `json:"template_id"`   // 绑定的工单模板 ID
	Name         string     `json:"name"`          // 展示名
	Icon         string     `json:"icon"`          // 展示图标
	Owner        string     `json:"owner"`         // 流程所有人
	Desc         string     `json:"desc"`          // 描述
	IsNotify     bool       `json:"is_notify"`     // 是否通知
	NotifyMethod uint8      `json:"notify_method"` // 默认渠道类型
	FlowData     *LogicFlow `json:"flow_data,omitempty"` // 前端流程图画布数据
}

// LogicFlow 前端 LogicFlow 画布对应的 Edge 与 Node 数据列表封装
type LogicFlow struct {
	Edges []map[string]interface{} `json:"edges,omitempty"` // 线关系集合
	Nodes []map[string]interface{} `json:"nodes,omitempty"` // 节点属性集合
}

// RetrieveWorkflows 分页拉取列表结果承载 VO
type RetrieveWorkflows struct {
	Total     int64      `json:"total"`     // 当前租户符合条件的总条数
	Workflows []Workflow `json:"workflows"` // 这一页的流程定义详细明细
}

// OrderGraphReq 工单流转地图查看请求参数
type OrderGraphReq struct {
	Id                int64 `json:"id"`                  // 流程定义 ID
	Status            uint8 `json:"status"`              // 实例当前的状态
	ProcessInstanceId int   `json:"process_instance_id"` // 底层流程引擎执行实例的 ProcessInstanceId
}

// RetrieveOrderGraph 渲染工单流程轨迹流转地图所需的前端出参模型
type RetrieveOrderGraph struct {
	EdgeIds  []string `json:"edge_ids"` // 已走过且被点亮激活的连线 ID 列表
	Workflow Workflow `json:"workflow"` // 锁定的历史版本画布图元数据 (含点亮状态的 Edge)
}
