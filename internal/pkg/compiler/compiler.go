package compiler

import (
	"context"

	"github.com/Duke1616/eflow/internal/domain"
)

// 引擎类型常量
const (
	EngineTypeEasyFlow = "easy-workflow"
	EngineTypeFlowx    = "flowx"
)

// CompileResult 流程画布编译结果包装
type CompileResult struct {
	EngineType string // 目标引擎类型
	RawDef     any    // 目标引擎对应的原生定义模型
}

// IWorkflowCompiler 工作流画布编译器的通用策略契约接口
type IWorkflowCompiler interface {
	// EngineType 声明该编译器适用的目标流程引擎
	EngineType() string
	// Compile 将业务领域层 Workflow 画布编译为对应流程引擎的模型结构
	Compile(ctx context.Context, wf domain.Workflow) (CompileResult, error)
}
