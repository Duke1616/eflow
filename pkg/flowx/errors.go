package flowx

import (
	"errors"

	"github.com/Duke1616/eflow/pkg/flowx/storage"
)

var (
	// ErrInstanceNotFound 流程实例未找到
	ErrInstanceNotFound = storage.ErrInstanceNotFound
	// ErrTaskNotFound 流程任务未找到
	ErrTaskNotFound = storage.ErrTaskNotFound
	// ErrDefinitionNotFound 流程定义未找到
	ErrDefinitionNotFound = storage.ErrDefinitionNotFound
	// ErrInvalidDefinition 流程定义静态校验未通过
	ErrInvalidDefinition = errors.New("流程图定义静态校验未通过")

	// ErrInstanceNotRunning 流程实例未处于运行态，禁止继续流转
	ErrInstanceNotRunning = errors.New("流程实例已不处于运行中状态")
	// ErrTaskNotPending 任务已被审批或取消，禁止重复操作
	ErrTaskNotPending = errors.New("流程任务已被处理，禁止重复操作")
	// ErrNoAvailablePath 节点后续无满足条件的有效流转出边
	ErrNoAvailablePath = errors.New("流程无有效流转路径")
	// ErrNodeDefinitionMissing 运行时根据 NodeID 找不到对应的静态节点定义
	ErrNodeDefinitionMissing = errors.New("节点规格定义丢失")
	// ErrHookAborted 流程流转在生命周期钩子中被主动阻断
	ErrHookAborted = errors.New("流程流转在钩子处理中被阻断")
	// ErrEmptyAssignees 审批节点未指派或未能解析到有效审批人
	ErrEmptyAssignees = errors.New("审批节点未指派或未解析到有效审批人")
	// ErrWorkerNotFound 自动化任务节点未找到对应的注册执行器
	ErrWorkerNotFound = errors.New("未注册对应的自动化任务执行器")
	// ErrInvalidTargetNode 指定的驳回目标节点不合法
	ErrInvalidTargetNode = errors.New("指定的驳回目标节点不合法")
	// ErrUnsupportedNodeType 不支持的节点类型
	ErrUnsupportedNodeType = errors.New("不支持的节点类型")
)
