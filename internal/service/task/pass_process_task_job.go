package task

import (
	"context"
	"errors"
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
	logger    *elog.Component
}

// NewPassProcessTaskJob 实例化通过任务自动向前流转后台任务
func NewPassProcessTaskJob(svc Service, engineSvc engine.Service,
	limit int64, interval time.Duration) *PassProcessTaskJob {
	return &PassProcessTaskJob{
		svc:       svc,
		engineSvc: engineSvc,
		limit:     limit,
		interval:  interval,
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
	var afterID int64
	var runErr error
	for {
		tasks, err := j.svc.ListUnadvancedSuccessTasks(ctx, j.limit, afterID)
		if err != nil {
			return fmt.Errorf("查询成功执行任务失败: %w", err)
		}

		for _, task := range tasks {
			afterID = task.ID
			taskCtx := tenantContext(ctx, task.TenantID)
			j.logger.Info("任务开启自动通过逻辑", elog.Int64("id", task.ID))
			mt, err1 := j.engineSvc.GetAutomationTask(taskCtx, task.NodeID, task.ProcessInstanceID)
			if err1 != nil {
				runErr = errors.Join(runErr,
					fmt.Errorf("查询流程自动化节点任务失败: task_id=%d: %w", task.ID, err1))
				continue
			}

			if mt.TaskID != 0 {
				err = j.engineSvc.Pass(taskCtx, mt.TaskID, "任务执行完成")
				if err != nil {
					runErr = errors.Join(runErr,
						fmt.Errorf("通过自动化节点失败: task_id=%d: %w", task.ID, err))
					continue
				}
			}

			err = j.svc.MarkTaskAsAutoPassed(taskCtx, task.ID)
			if err != nil {
				runErr = errors.Join(runErr,
					fmt.Errorf("标记自动化任务已推进失败: task_id=%d: %w", task.ID, err))
			}
		}

		if int64(len(tasks)) < j.limit {
			break
		}
	}
	return runErr
}
