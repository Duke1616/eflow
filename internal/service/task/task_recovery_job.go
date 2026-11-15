package task

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/gotomicro/ego/core/elog"
)

// TaskRecoveryJob 宕线补偿与自动重试后台任务
type TaskRecoveryJob struct {
	svc         Service
	limit       int64
	interval    time.Duration
	staleAfter  time.Duration
	taskTimeout time.Duration
	logger      *elog.Component
}

// NewTaskRecoveryJob 实例化宕机恢复与自动重试补偿任务
func NewTaskRecoveryJob(svc Service, limit int64,
	interval, staleAfter, taskTimeout time.Duration) *TaskRecoveryJob {
	return &TaskRecoveryJob{
		svc: svc, limit: limit, interval: interval,
		staleAfter: staleAfter, taskTimeout: taskTimeout, logger: elog.DefaultLogger,
	}
}

// Start 启动异常或挂起任务的补偿检查周期协程
func (j *TaskRecoveryJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if err := j.run(ctx); err != nil {
			j.logger.Error("任务补偿失败", elog.FieldErr(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *TaskRecoveryJob) run(ctx context.Context) error {
	now := time.Now()
	handlers := []struct {
		status domain.TaskStatus
		handle func(context.Context, int64) error
		log    string
	}{
		{status: domain.TaskStatusSubmitting, handle: j.svc.StartTask, log: "恢复提交中任务失败"},
		{status: domain.TaskStatusFailed, handle: j.svc.AutoRetryTask, log: "自动重试失败任务失败"},
		{status: domain.TaskStatusRunning, handle: j.svc.ReconcileTask, log: "对账运行中任务失败"},
	}
	for _, current := range handlers {
		err := j.scan(ctx, current.status, func(task domain.Task) {
			if now.Before(time.UnixMilli(task.UTime).Add(j.staleAfter)) {
				return
			}
			if handleErr := j.handle(ctx, current.handle, task); handleErr != nil {
				j.logger.Error(current.log, elog.Int64("taskID", task.ID), elog.FieldErr(handleErr))
			}
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// handle 给单次补偿调用套独立超时。etask 客户端拦截器链不含 deadline 也未配 keepalive，
// 连接黑洞化时 RPC 会永久挂起；补偿是串行扫描，不设上限会让一次挂起停摆全部三类补偿。
func (j *TaskRecoveryJob) handle(ctx context.Context,
	handle func(context.Context, int64) error, task domain.Task) error {
	callCtx, cancel := context.WithTimeout(tenantContext(ctx, task.TenantID), j.taskTimeout)
	defer cancel()
	return handle(callCtx, task.ID)
}

func (j *TaskRecoveryJob) scan(ctx context.Context, status domain.TaskStatus,
	handle func(domain.Task)) error {
	var afterID int64
	for {
		tasks, err := j.svc.ListTasksByStatusAfterID(ctx, status, afterID, j.limit)
		if err != nil {
			return err
		}
		for _, task := range tasks {
			afterID = task.ID
			handle(task)
		}
		if int64(len(tasks)) < j.limit {
			return nil
		}
	}
}
