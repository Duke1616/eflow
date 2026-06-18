package template

// ToggleFavoriteReq 切换收藏请求
type ToggleFavoriteReq struct {
	TemplateId int64 `json:"template_id"`
}

// CreateTemplateReq 创建模板请求
type CreateTemplateReq struct {
	Name       string `json:"name"`
	WorkflowId int64  `json:"workflow_id"`
	GroupId    int64  `json:"group_id"`
	Icon       string `json:"icon"`
	Rules      string `json:"rules"`
	Options    string `json:"options"`
	Desc       string `json:"desc"`
}

// DetailTemplateReq 获取模板详情请求
type DetailTemplateReq struct {
	Id int64 `json:"id"`
}

// Page 分页基类
type Page struct {
	Offset int64 `json:"offset,omitempty"`
	Limit  int64 `json:"limit,omitempty"`
}

// ListTemplateReq 分页获取模板列表请求
type ListTemplateReq struct {
	Page
	GroupId int64  `json:"group_id,omitempty"`
	Keyword string `json:"keyword,omitempty"`
}

// FindByTemplateIds 批量模板 ID 请求
type FindByTemplateIds struct {
	Ids []int64 `json:"ids"`
}

// DeleteTemplateReq 删除模板请求
type DeleteTemplateReq struct {
	Id int64 `json:"id"`
}

// CreateType 模板创建方式
type CreateType uint8

// Template 模板通用响应模型 (规则为字符串)
type Template struct {
	Id         int64      `json:"id"`
	Name       string     `json:"name"`
	WorkflowId int64      `json:"workflow_id"`
	Icon       string     `json:"icon"`
	GroupId    int64      `json:"group_id"`
	CreateType CreateType `json:"create_type"`
	Rules      string     `json:"rules"`
	Options    string     `json:"options"`
	Desc       string     `json:"desc"`
}

// TemplateJson 模板通用 JSON 响应模型 (规则为原生 Map)
type TemplateJson struct {
	Id         int64                    `json:"id"`
	Name       string                   `json:"name"`
	WorkflowId int64                    `json:"workflow_id"`
	Icon       string                   `json:"icon"`
	GroupId    int64                    `json:"group_id"`
	CreateType CreateType               `json:"create_type"`
	Rules      []map[string]interface{} `json:"rules"`
	Options    map[string]interface{}   `json:"options"`
	Desc       string                   `json:"desc"`
}

// RetrieveTemplates 响应数据结构包
type RetrieveTemplates struct {
	Total     int64          `json:"total"`
	Templates []TemplateJson `json:"templates"`
}

// GetRulesByWorkFlowIdReq 通过工作流检索校验规则请求
type GetRulesByWorkFlowIdReq struct {
	WorkFlowId int64 `json:"workflow_id"`
}

// Rule 单个解析出来的规则模型
type Rule struct {
	Type  string `json:"type"`
	Field string `json:"field"`
	Title string `json:"title"`
}

// TemplateRules 模板关联规则模型
type TemplateRules struct {
	Id    int64  `json:"id"`
	Name  string `json:"name"`
	Rules []Rule `json:"rules"`
}

// RetrieveTemplateRules 响应数据包装结构
type RetrieveTemplateRules struct {
	TemplateRules []TemplateRules `json:"template_rules"`
}

// UpdateTemplateReq 模板更新请求
type UpdateTemplateReq struct {
	Id         int64  `json:"id"`
	GroupId    int64  `json:"group_id"`
	Icon       string `json:"icon"`
	WorkflowId int64  `json:"workflow_id"`
	Name       string `json:"name"`
	Desc       string `json:"desc"`
	Rules      string `json:"rules"`
	Options    string `json:"options"`
}

// GetTemplatesByWorkFlowIdReq 关联工作流模板详情请求
type GetTemplatesByWorkFlowIdReq struct {
	WorkFlowId int64 `json:"workflow_id"`
}

// CreateTemplateGroupReq 创建模板分组请求
type CreateTemplateGroupReq struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// UpdateTemplateGroupReq 修改模板分组请求
type UpdateTemplateGroupReq struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// TemplateGroup 模板分类分组信息
type TemplateGroup struct {
	Id   int64  `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// TemplateGroupSummary 模板分组摘要信息
type TemplateGroupSummary struct {
	Id    int64  `json:"id"`
	Name  string `json:"name"`
	Icon  string `json:"icon"`
	Total int64  `json:"total"`
}

// RetrieveTemplateGroup 分页模板分类组详情响应
type RetrieveTemplateGroup struct {
	TemplateGroups []TemplateGroup `json:"template_groups"`
	Total          int64           `json:"total"`
}

// RetrieveTemplateGroupSummary 模板分组摘要响应
type RetrieveTemplateGroupSummary struct {
	TemplateGroups []TemplateGroupSummary `json:"template_groups"`
	Total          int64                  `json:"total"`
}

// TemplateCombination 分类下的组合数据结构
type TemplateCombination struct {
	Id        int64      `json:"id,omitempty"`
	Name      string     `json:"name,omitempty"`
	Icon      string     `json:"icon,omitempty"`
	Total     int64      `json:"total"`
	Templates []Template `json:"templates"`
}
