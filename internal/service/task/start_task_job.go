package task

import (
	"context"
	"time"

	"github.com/gotomicro/ego/core/elog"
)

// StartTaskJob 就绪任务周期性自动派发后台任务
type StartTaskJob struct {
	svc      Service
	limit    int64
	interval time.Duration
	logger   *elog.Component
}

// NewStartTaskJob 实例化就绪任务自动下派任务
func NewStartTaskJob(svc Service, limit int64, interval time.Duration) *StartTaskJob {
	return &StartTaskJob{svc: svc, limit: limit, interval: interval, logger: elog.DefaultLogger}
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
	for _, task := range tasks {
		task := task
		go func() {
			if err = j.svc.StartTask(context.Background(), task.Id); err != nil {
				j.logger.Error("任务启动失败", elog.FieldErr(err), elog.Int64("taskId", task.Id))
			}
		}()
	}
	return nil
}
