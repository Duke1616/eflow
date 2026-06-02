package task

import (
	"context"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/gotomicro/ego/core/elog"
)

// PassProcessTaskJob 审批通过自动归档/流转推进后台任务
type PassProcessTaskJob struct {
	svc       Service
	engineSvc engine.Service
	limit     int64
	interval  time.Duration
	minutes   int64
	seconds   int64
	logger    *elog.Component
}

// NewPassProcessTaskJob 实例化通过任务自动向前流转后台任务
func NewPassProcessTaskJob(svc Service, engineSvc engine.Service, limit int64, interval time.Duration, minutes, seconds int64) *PassProcessTaskJob {
	return &PassProcessTaskJob{
		svc:       svc,
		engineSvc: engineSvc,
		limit:     limit,
		interval:  interval,
		minutes:   minutes,
		seconds:   seconds,
		logger:    elog.DefaultLogger.With(elog.FieldComponentName("PassProcessTaskJob")),
	}
}

// Start 启动已完成任务驱动流程物理向前通过的轮询协程
func (j *PassProcessTaskJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if err := j.run(ctx); err != nil {
			j.logger.Error("通过自动化节点任务失败", elog.FieldErr(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *PassProcessTaskJob) run(ctx context.Context) error {
	utime := time.Now().Add(time.Duration(-j.minutes)*time.Minute + time.Duration(-j.seconds)*time.Second).UnixMilli()
	offset := int64(0)
	for {
		tasks, total, err := j.svc.ListSuccessTasksByUtime(ctx, offset, j.limit, utime)
		if err != nil {
			return fmt.Errorf("查询成功执行任务失败: %w", err)
		}

		for _, task := range tasks {
			j.logger.Info("任务开启自动通过逻辑", elog.Int64("id", task.Id))
			mt, err1 := j.engineSvc.GetAutomationTask(ctx, task.CurrentNodeId, task.ProcessInstId)
			if err1 != nil {
				continue
			}

			if mt.TaskID != 0 {
				err = j.engineSvc.Pass(ctx, mt.TaskID, "任务执行完成")
				if err != nil {
					return fmt.Errorf("通过自动化节点失败: %w", err)
				}
			}

			err = j.svc.MarkTaskAsAutoPassed(ctx, task.Id)
			if err != nil {
				j.logger.Error("数据标记失败", elog.FieldErr(err))
			}
		}

		if int64(len(tasks)) < j.limit {
			break
		}
		offset += j.limit
		if offset >= total {
			break
		}
	}
	return nil
}
