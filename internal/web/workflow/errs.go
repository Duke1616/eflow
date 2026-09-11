package workflow

import (
	"errors"

	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/ecodeclub/ginx"
)

var (
	// SystemErrorResult 全局业务系统故障通用错误返回结构
	SystemErrorResult = ginx.Result{Code: 500001, Msg: "系统错误"}

	// deployValidateResult 流程图拓扑校验失败错误码
	// NOTE: Code=400002 用于前端区分「用户操作有误」vs「系统内部故障」
	deployValidateResult = ginx.Result{Code: 400002, Msg: ""}
)

// toDeployResult 将发布错误转换为前端友好的响应结果。
// 若为流程图校验失败（拓扑非法），直接将校验信息透传给前端展示；
// 其他错误统一返回系统错误。
func toDeployResult(err error) ginx.Result {
	var validateErr *easyflow.ValidateError
	if errors.As(err, &validateErr) {
		res := deployValidateResult
		res.Msg = validateErr.Error()
		return res
	}
	return SystemErrorResult
}
