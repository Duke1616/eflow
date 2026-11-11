package flowx

import (
	"context"
	"errors"
	"sync"
	"time"

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

// wrapWithRetry 使用函数式装饰器模式为 TaskWorker 赋予弹性退避重试能力，调度器主干零侵入
func wrapWithRetry(worker TaskWorker, policy *model.RetryPolicy) TaskWorker {
	if worker == nil || policy == nil || policy.MaxAttempts <= 1 {
		return worker
	}

	return func(ctx context.Context, taskCtx *TaskContext) (*TaskResult, error) {
		interval := policy.InitialInterval
		if interval <= 0 {
			interval = 100 * time.Millisecond
		}
		factor := policy.BackoffFactor
		if factor < 1.0 {
			factor = 1.0
		}

		for attempt := 1; attempt <= policy.MaxAttempts; attempt++ {
			if taskCtx != nil {
				taskCtx.Attempt = attempt
			}

			res, err := worker(ctx, taskCtx)
			if err == nil || attempt == policy.MaxAttempts {
				return res, err
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(interval):
			}

			if factor > 1.0 {
				nextInterval := time.Duration(float64(interval) * factor)
				if policy.MaxInterval > 0 && nextInterval > policy.MaxInterval {
					nextInterval = policy.MaxInterval
				}
				interval = nextInterval
			}
		}
		return nil, errors.New("自动化任务达到最大重试上限")
	}
}
