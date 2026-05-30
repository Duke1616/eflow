package task

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/gotomicro/ego/core/elog"
)

// TaskRecoveryJob 宕线补偿与自动重试后台任务
type TaskRecoveryJob struct {
	svc      Service
	limit    int64
	interval time.Duration
	logger   *elog.Component
}

// NewTaskRecoveryJob 实例化宕机恢复与自动重试补偿任务
func NewTaskRecoveryJob(svc Service, limit int64, interval time.Duration) *TaskRecoveryJob {
	return &TaskRecoveryJob{svc: svc, limit: limit, interval: interval, logger: elog.DefaultLogger}
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
	tasks, _, err := j.svc.ListTaskByStatus(ctx, 0, j.limit, domain.SCHEDULED.ToUint8())
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, task := range tasks {
		if task.IsTiming && now < task.ScheduledTime {
			continue
		}
		if !task.IsTiming && now < task.Utime+60*1000 {
			continue
		}
		task := task
		go func() {
			_ = j.svc.AutoRetryTask(context.Background(), task.Id)
		}()
	}
	return nil
}
