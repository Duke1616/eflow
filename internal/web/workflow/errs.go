package workflow

import (
	"errors"

	"github.com/Duke1616/eflow/internal/errs"
	easyflow "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/ecodeclub/ginx"
)

var (
	// SystemErrorResult 全局业务系统故障通用错误返回结构
	SystemErrorResult = ginx.Result{Code: 500001, Msg: "系统错误"}

	// ErrWorkflowInvalidID 流程定义 ID 参数非法
	ErrWorkflowInvalidID = ginx.Result{Code: 400001, Msg: "流程定义 ID 格式错误"}

	// ErrWorkflowNotFound 流程定义不存在或已下线
	ErrWorkflowNotFound = ginx.Result{Code: 404001, Msg: "流程定义不存在或已删除"}

	// deployValidateResult 流程图拓扑校验失败错误码
	// NOTE: Code=400002 用于前端区分「用户操作有误」vs「系统内部故障」
	deployValidateResult = ginx.Result{Code: 400002, Msg: ""}
)

// toDeployResult 将发布错误转换为前端友好的响应结果。
// 若为流程图校验失败（拓扑非法），直接将校验信息透传给前端展示；
// 其他错误统一返回系统错误。
func toDeployResult(err error) ginx.Result {
	if bizErr, ok := errors.AsType[*errs.Error](err); ok {
		return ginx.Result{Code: int(bizErr.Code), Msg: bizErr.Message}
	}
	if validateErr, ok := errors.AsType[*easyflow.ValidateError](err); ok {
		res := deployValidateResult
		res.Msg = validateErr.Error()
		return res
	}
	if errors.Is(err, flowx.ErrInvalidDefinition) {
		res := deployValidateResult
		res.Msg = err.Error()
		return res
	}
	return SystemErrorResult
}
