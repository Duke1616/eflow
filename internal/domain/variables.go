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
	// VarOperator 当前任务处理人或操作人账号键。
	VarOperator = NewVarKey[string]("operator", "")
	// VarComment 任务流转审批意见键。
	VarComment = NewVarKey[string]("comment", "")
)
