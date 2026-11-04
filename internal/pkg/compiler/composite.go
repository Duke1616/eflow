package compiler

import (
	"context"
	"fmt"
	"sync"

	"github.com/Duke1616/eflow/internal/domain"
)

// ICompositeCompiler 具备多引擎路由能力的多态复合编译器接口
type ICompositeCompiler interface {
	// Compile 根据指定引擎类型将 Workflow 画布编译为目标定义
	Compile(ctx context.Context, engineType string, wf domain.Workflow) (CompileResult, error)
	// Register 注册或重写指定引擎的编译器实现
	Register(compiler IWorkflowCompiler)
}

type compositeCompiler struct {
	mu        sync.RWMutex
	compilers map[string]IWorkflowCompiler
}

// NewDefaultCompositeCompiler 创建并初始化包含 flowx 与 easyflow 的默认多态编译器
func NewDefaultCompositeCompiler() ICompositeCompiler {
	c := &compositeCompiler{
		compilers: make(map[string]IWorkflowCompiler),
	}
	c.Register(NewFlowxCompiler())
	c.Register(NewEasyflowCompiler())
	return c
}

func (c *compositeCompiler) Register(compiler IWorkflowCompiler) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.compilers[compiler.EngineType()] = compiler
}

func (c *compositeCompiler) Compile(ctx context.Context, engineType string, wf domain.Workflow) (CompileResult, error) {
	if engineType == "" {
		engineType = EngineTypeFlowx // 默认优先采用现代化 flowx 引擎
	}

	c.mu.RLock()
	compiler, ok := c.compilers[engineType]
	c.mu.RUnlock()

	if !ok {
		return CompileResult{}, fmt.Errorf("未注册对应的流程编译器 [engine_type=%s]", engineType)
	}

	return compiler.Compile(ctx, wf)
}
