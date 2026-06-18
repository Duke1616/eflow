package domain

import (
	"github.com/xen0n/go-workwx"
)

// CreateType 模板创建/来源类型定义
type CreateType uint8

// ToUint8 转换类型为基本 uint8 表达
func (s CreateType) ToUint8() uint8 {
	return uint8(s)
}

const (
	// SystemCreate 系统创建的模板
	SystemCreate CreateType = 1
	// WechatCreate 企业微信同步或创建的模板
	WechatCreate CreateType = 2
)

// Rule 单个工单控件的动态校验规则
type Rule map[string]interface{}

// TemplateOptions 工单模板附加选项参数
type TemplateOptions map[string]interface{}

// Template 工单页面表单渲染与控件模板领域模型
type Template struct {
	Id                 int64                     // 模板主键 ID
	Name               string                    // 模板名称
	WorkflowId         int64                     // 关联的工作流流程定义 ID
	GroupId            int64                     // 关联的分组 ID
	Icon               string                    // 模板图标 (SVG 或是 URL)
	CreateType         CreateType                // 模板创建来源
	UniqueHash         string                    // 用以排重的唯一性哈希
	ExternalTemplateId string                    // 外部关联模板 ID (如企微 OA 模板 ID)
	WechatOAControls   workwx.OATemplateControls // 企业微信 OA 模板控制属性字段映射
	Rules              []Rule                    // 前端页面的控件校验和渲染规则属性
	Options            TemplateOptions           // 额外的模板选项配置数据
	Desc               string                    // 模板详细说明
}

// TemplateGroup 工单模板所属分类分组模型
type TemplateGroup struct {
	Id   int64  // 分组 ID
	Name string // 分组名称（如：CMDB管理类、权限申请类）
	Icon string // 分组图标样式
}

// TemplateGroupSummary 模板分组摘要，用于管理页按分组懒加载模板列表
type TemplateGroupSummary struct {
	Id    int64  // 分组 ID
	Name  string // 分组名称
	Icon  string // 分组图标样式
	Total int64  // 分组下模板数量
}

// WechatInfo 用于企业微信与本地进行模板绑定和信息交互的承载对象
type WechatInfo struct {
	TemplateId   string // 微信模板唯一 ID
	TemplateName string // 微信模板标题名称
	SpNo         string // 审批申请单号
}
