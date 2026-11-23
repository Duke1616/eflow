package domain

import "github.com/Duke1616/eflow/pkg/mapx"

// Variables 表示工单与流程共享的业务变量字典。
type Variables = mapx.Map

// NewVariables 创建已初始化的业务变量字典。
func NewVariables() Variables {
	return mapx.New()
}

// VarKey 将业务变量键名与其值类型、默认值绑定为契约。
type VarKey[T any] = mapx.Key[T]

// NewVarKey 创建类型安全的业务变量键。
func NewVarKey[T any](key string, defaultValue ...T) VarKey[T] {
	return mapx.NewKey[T](key, defaultValue...)
}

var (
	// VarTicketID 工单全局业务标识键。
	VarTicketID = NewVarKey[int64]("ticket_id", 0)
	// VarStarter 流程发起人账号键。
	VarStarter = NewVarKey[string]("starter", "")
	// VarTemplateName 工单所属模板名称键。
	VarTemplateName = NewVarKey[string]("template_name", "")
	// VarOperator 当前任务处理人或操作人账号键。
	VarOperator = NewVarKey[string]("operator", "")
	// VarComment 任务流转审批意见键。
	VarComment = NewVarKey[string]("comment", "")
)

// systemVariableKeys 记录所有流程引擎系统级保留内置变量键
var systemVariableKeys = map[string]struct{}{
	VarTicketID.Name():     {},
	VarStarter.Name():      {},
	VarTemplateName.Name(): {},
	VarOperator.Name():     {},
	VarComment.Name():      {},
}

// IsSystemVariable 判断指定键名是否为流程引擎系统级保留变量。
// 设计原因：系统级流程变量（如发起人、工单ID、模板名）属于流程运行上下文，不属于业务表单数据，
// 在向通知卡片注入或解析字段时应严格隔离，防止在卡片正文中泄漏英文原始 Key。
func IsSystemVariable(key string) bool {
	_, exists := systemVariableKeys[key]
	return exists
}
