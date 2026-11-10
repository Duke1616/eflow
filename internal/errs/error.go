package errs

import (
	"errors"
	"fmt"
	"net/http"
)

// Error 仿照 Kratos 规范构建的工业级结构化业务错误
// 具备 HTTP 状态码、业务代号(Reason)、面向用户文案(Message)、元数据(Metadata)与根因溯源(cause)能力
type Error struct {
	Code     int32             `json:"code"`
	Reason   string            `json:"reason"`
	Message  string            `json:"message"`
	Metadata map[string]string `json:"metadata,omitempty"`
	cause    error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.cause != nil {
		return fmt.Sprintf("error: code = %d reason = %s message = %s cause = %v", e.Code, e.Reason, e.Message, e.cause)
	}
	return fmt.Sprintf("error: code = %d reason = %s message = %s", e.Code, e.Reason, e.Message)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// Is 支持基于指针相等性或 Reason 业务代号进行语义比对
func (e *Error) Is(target error) bool {
	if target == nil {
		return e == nil
	}
	if e == target {
		return true
	}
	if t, ok := errors.AsType[*Error](target); ok {
		return e.Reason == t.Reason
	}
	return false
}

// WithCause 链式携带底层根因 (供堆栈与日志排查)
func (e *Error) WithCause(cause error) *Error {
	e.cause = cause
	return e
}

// WithMetadata 链式注入上下文元数据键值对
func (e *Error) WithMetadata(md map[string]string) *Error {
	e.Metadata = md
	return e
}

// New 基础业务错误构造器
func New(code int32, reason, message string) *Error {
	return &Error{
		Code:    code,
		Reason:  reason,
		Message: message,
	}
}

// BadRequest 客户端请求入参或语义错误 (HTTP 400)
func BadRequest(reason, message string) *Error {
	return New(http.StatusBadRequest, reason, message)
}

// Unauthorized 身份未认证 (HTTP 401)
func Unauthorized(reason, message string) *Error {
	return New(http.StatusUnauthorized, reason, message)
}

// Forbidden 权限不足或拒绝操作 (HTTP 403)
func Forbidden(reason, message string) *Error {
	return New(http.StatusForbidden, reason, message)
}

// NotFound 目标业务实体未找到 (HTTP 404)
func NotFound(reason, message string) *Error {
	return New(http.StatusNotFound, reason, message)
}

// Conflict 业务状态冲突 (HTTP 409，如已被他人审批、终态不可操作)
func Conflict(reason, message string) *Error {
	return New(http.StatusConflict, reason, message)
}

// InternalServer 真正严重的系统内部故障 (HTTP 500)
func InternalServer(reason, message string) *Error {
	return New(http.StatusInternalServerError, reason, message)
}

// FromError 从任意 error 萃取 *Error 结构实例
func FromError(err error) *Error {
	if err == nil {
		return nil
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e
	}
	return InternalServer("UNKNOWN", err.Error()).WithCause(err)
}

// Reason 提取错误的业务代号
func Reason(err error) string {
	if e := FromError(err); e != nil {
		return e.Reason
	}
	return ""
}

// IsReason 判断给定错误是否匹配指定的业务代号
func IsReason(err error, reason string) bool {
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Reason == reason
	}
	return false
}

// IsBadRequest 判断是否为 400 客户端错误
func IsBadRequest(err error) bool { return Code(err) == http.StatusBadRequest }

// IsForbidden 判断是否为 403 权限拒绝错误
func IsForbidden(err error) bool { return Code(err) == http.StatusForbidden }

// IsNotFound 判断是否为 404 资源未找到错误
func IsNotFound(err error) bool { return Code(err) == http.StatusNotFound }

// IsConflict 判断是否为 409 状态冲突错误
func IsConflict(err error) bool { return Code(err) == http.StatusConflict }

// Code 提取错误对应的 HTTP 状态码
func Code(err error) int32 {
	if err == nil {
		return http.StatusOK
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Code
	}
	return http.StatusInternalServerError
}

// Message 提取面向用户的安全提示信息
func Message(err error) string {
	if err == nil {
		return ""
	}
	if e, ok := errors.AsType[*Error](err); ok {
		return e.Message
	}
	return "系统内部错误"
}

// 常用通用哨兵错误定义（向下兼容存量代码）
var (
	ErrInvalidParameter = BadRequest("INVALID_PARAMETER", "参数校验错误")
)
