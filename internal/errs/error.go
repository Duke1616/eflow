package errs

import "errors"

// ErrInvalidParameter 通用参数校验错误定义
var ErrInvalidParameter = errors.New("参数校验错误")
