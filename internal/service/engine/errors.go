package engine

import "errors"

// 领域标准哨兵错误（Domain Sentinel Errors）
// 遵循大名老师设计规范：跨层调用必须提供语义明确的哨兵错误，供上层基于 errors.Is 精准识别与分支处理
var (
	// ErrEngineNotFound 未注册对应类型的工作流引擎驱动
	ErrEngineNotFound = errors.New("流程引擎驱动未注册")

	// ErrCapabilityNotSupported 底层流程引擎不支持当前操作能力
	ErrCapabilityNotSupported = errors.New("当前流程引擎不支持该操作能力")

	// ErrTaskNotFound 待审批任务不存在或已处理办结
	ErrTaskNotFound = errors.New("待审批任务不存在或已完结")

	// ErrInstanceNotFound 流程实例不存在或已清理
	ErrInstanceNotFound = errors.New("流程实例不存在")

	// ErrInvalidDefinition 流程定义格式或拓扑结构不合法
	ErrInvalidDefinition = errors.New("流程定义格式非法")

	// ErrInvalidParameter 流程入参校验失败
	ErrInvalidParameter = errors.New("流程操作入参校验失败")
)
