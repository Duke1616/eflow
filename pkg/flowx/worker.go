package flowx

import (
	"context"
	"sync"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// TaskContext 自动化任务执行上下文
type TaskContext struct {
	Instance  *model.Instance
	Node      *model.NodeSpec
	Variables model.Variables
	Attempt   int // 当前执行轮次 (从 1 开始)
}

// TaskResult 自动化任务执行产物
type TaskResult struct {
	// Variables 任务产出的增量业务变量，将安全合并至流程实例
	Variables model.Variables
}

// TaskWorker 自动化任务执行器函数签名
type TaskWorker func(ctx context.Context, taskCtx *TaskContext) (*TaskResult, error)

type workerRegistry struct {
	mu      sync.RWMutex
	workers map[string]TaskWorker
}

func newWorkerRegistry() *workerRegistry {
	return &workerRegistry{
		workers: make(map[string]TaskWorker),
	}
}

func (r *workerRegistry) Register(workerType string, worker TaskWorker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[workerType] = worker
}

func (r *workerRegistry) Get(workerType string) (TaskWorker, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.workers[workerType]
	return w, ok
}
