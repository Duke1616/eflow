package template

import (
	"github.com/ecodeclub/ginx"
)

var (
	// SystemErrorResult 通用系统错误
	SystemErrorResult = ginx.Result{Code: 500001, Msg: "系统错误"}
	// ErrTemplateInvalidId 模板 ID 格式不正确或为空
	ErrTemplateInvalidId = ginx.Result{Code: 4010707, Msg: "工单模板 ID 非法"}
)

// ErrInvalidParameter 动态产生参数校验失败的 ginx.Result 响应结果
func ErrInvalidParameter(err error) ginx.Result {
	return ginx.Result{
		Code: 400001,
		Msg:  err.Error(),
	}
}
