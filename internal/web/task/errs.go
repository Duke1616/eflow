package task

import "github.com/ecodeclub/ginx"

var systemErrorResult = ginx.Result{
	Code: 5,
	Msg:  "系统错误",
}

func invalidParameterResult(err error) ginx.Result {
	return ginx.Result{
		Code: 400001,
		Msg:  err.Error(),
	}
}
