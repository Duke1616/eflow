package workflow

import "github.com/ecodeclub/ginx"

var (
	// SystemErrorResult 全局业务系统故障通用错误返回结构
	SystemErrorResult = ginx.Result{Code: 500001, Msg: "系统错误"}
)
