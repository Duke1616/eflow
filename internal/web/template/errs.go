package template

import (
	"github.com/ecodeclub/ginx"
)

var (
	// SystemErrorResult 通用系统错误
	SystemErrorResult = ginx.Result{Code: 500001, Msg: "系统错误"}
	// ErrTemplateInvalidId 模板 ID 格式不正确或为空
	ErrTemplateInvalidId = ginx.Result{Code: 4010707, Msg: "工单模板 ID 非法"}
	// ErrTemplateGroupInvalidId 模板分组 ID 格式不正确或为空
	ErrTemplateGroupInvalidId = ginx.Result{Code: 4010708, Msg: "工单模板分组 ID 非法"}
	// ErrTemplateGroupNotEmpty 模板分组下存在模板时拒绝删除
	ErrTemplateGroupNotEmpty = ginx.Result{Code: 4010709, Msg: "请先删除分组下的模板后再删除分组"}
)

// ErrInvalidParameter 动态产生参数校验失败的 ginx.Result 响应结果
func ErrInvalidParameter(err error) ginx.Result {
	return ginx.Result{
		Code: 400001,
		Msg:  err.Error(),
	}
}
