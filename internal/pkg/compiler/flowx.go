package compiler

import (
	"context"
	"fmt"

	"github.com/Duke1616/eflow/internal/domain"
	flowxconv "github.com/Duke1616/eflow/internal/pkg/compiler/flowx"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
)

// flowxCompiler 现代自研 flowx 引擎的画布编译器实现
type flowxCompiler struct {
	converter flowxconv.IConverter
}

// NewFlowxCompiler 创建 flowx 流程引擎画布编译器
func NewFlowxCompiler() IWorkflowCompiler {
	return &flowxCompiler{
		converter: flowxconv.NewConverter(),
	}
}

func (c *flowxCompiler) EngineType() EngineType {
	return EngineTypeFlowx
}

func (c *flowxCompiler) Compile(_ context.Context, wf domain.Workflow) (CompileResult, error) {
	procKey := domain.FormatFlowxProcessKey(wf.Id)
	if wf.Id <= 0 {
		procKey = fmt.Sprintf("wf_temp_%s", wf.Name)
	}

	graph, err := flowdsl.Parse(wf.FlowData)
	if err != nil {
		return CompileResult{}, fmt.Errorf("解析流程画布失败: %w", err)
	}

	def, err := c.converter.Convert(procKey, wf.Name, graph)
	if err != nil {
		return CompileResult{}, fmt.Errorf("flowx 画布编译失败: %w", err)
	}

	return CompileResult{
		EngineType: EngineTypeFlowx,
		RawDef:     def,
	}, nil
}
