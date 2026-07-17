package task

import (
	"context"
	"time"

	"github.com/gotomicro/ego/core/elog"
	"golang.org/x/sync/errgroup"
)

// StartTaskJob 就绪任务周期性自动派发后台任务
type StartTaskJob struct {
	svc         Service
	limit       int64
	concurrency int
	interval    time.Duration
	taskTimeout time.Duration
	logger      *elog.Component
}

// NewStartTaskJob 实例化就绪任务自动下派任务
func NewStartTaskJob(svc Service, limit int64, concurrency int,
	interval, taskTimeout time.Duration) *StartTaskJob {
	return &StartTaskJob{
		svc: svc, limit: limit, concurrency: concurrency,
		interval: interval, taskTimeout: taskTimeout, logger: elog.DefaultLogger,
	}
}

// Start 启动就绪任务自动下发周期轮询协程
func (j *StartTaskJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if err := j.run(ctx); err != nil {
			j.logger.Error("就绪任务启动失败", elog.FieldErr(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *StartTaskJob) run(ctx context.Context) error {
	tasks, err := j.svc.ListReadyTasks(ctx, j.limit)
	if err != nil {
		return err
	}
	var group errgroup.Group
	group.SetLimit(j.concurrency)
	for _, task := range tasks {
		taskID := task.ID
		group.Go(func() error {
			taskCtx, cancel := context.WithTimeout(ctx, j.taskTimeout)
			defer cancel()
			if startErr := j.svc.StartTask(taskCtx, taskID); startErr != nil {
				j.logger.Error("任务启动失败", elog.FieldErr(startErr), elog.Int64("taskID", taskID))
			}
			return nil
		})
	}
	return group.Wait()
}
