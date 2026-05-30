package runner

import (
	"github.com/ecodeclub/ginx"
)

var (
	SystemErrorResult  = ginx.Result{Code: 500001, Msg: "系统错误"}
	ErrRunnerInvalidId = ginx.Result{Code: 4010606, Msg: "执行器 ID 非法"}
)

// ErrInvalidParameter 动态产生参数校验失败的 ginx.Result 响应结果
func ErrInvalidParameter(err error) ginx.Result {
	return ginx.Result{
		Code: 400001,
		Msg:  err.Error(),
	}
}
