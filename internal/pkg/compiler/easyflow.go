package compiler

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	easyflowconv "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
)

// easyflowCompiler 遗留 easy-workflow 引擎的画布编译器实现
type easyflowCompiler struct {
	converter easyflowconv.Converter
}

// NewEasyflowCompiler 创建 easy-workflow 引擎画布编译器
func NewEasyflowCompiler() IWorkflowCompiler {
	return &easyflowCompiler{
		converter: easyflowconv.NewLogicFlowToEngineConvert(),
	}
}

func (c *easyflowCompiler) EngineType() string {
	return EngineTypeEasyFlow
}

func (c *easyflowCompiler) Compile(_ context.Context, wf domain.Workflow) (CompileResult, error) {
	process, err := c.converter.Convert(easyflowconv.FromDomainWorkflow(wf))
	if err != nil {
		return CompileResult{}, fmt.Errorf("easy-workflow 画布编译失败: %w", err)
	}

	return CompileResult{
		EngineType: EngineTypeEasyFlow,
		RawDef:     process,
	}, nil
}
