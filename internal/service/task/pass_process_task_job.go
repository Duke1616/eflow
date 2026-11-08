package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/gotomicro/ego/core/elog"
	"gorm.io/gorm"
)

// PassProcessTaskJob 审批通过自动归档/流转推进后台任务
type PassProcessTaskJob struct {
	svc      Service
	engine   engine.IEngine
	limit    int64
	interval time.Duration
	logger   *elog.Component
}

// NewPassProcessTaskJob 实例化通过任务自动向前流转后台任务
func NewPassProcessTaskJob(svc Service, engine engine.IEngine,
	limit int64, interval time.Duration) *PassProcessTaskJob {
	return &PassProcessTaskJob{
		svc:      svc,
		engine:   engine,
		limit:    limit,
		interval: interval,
		logger:   elog.DefaultLogger.With(elog.FieldComponentName("PassProcessTaskJob")),
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
			if err = j.advanceTask(taskCtx, task); err != nil {
				runErr = errors.Join(runErr, err)
			}
		}

		if int64(len(tasks)) < j.limit {
			break
		}
	}
	return runErr
}

func (j *PassProcessTaskJob) advanceTask(ctx context.Context, task domain.Task) error {
	shouldAdvance, engineType, checkErr := j.svc.ShouldAdvanceProcessTask(ctx, task.ID)
	if checkErr != nil {
		if errors.Is(checkErr, ErrWithdrawalInProgress) {
			return nil
		}
		return fmt.Errorf("检查自动化任务流程状态失败: task_id=%d: %w", task.ID, checkErr)
	}
	if !shouldAdvance {
		if err := j.svc.MarkTaskAsAutoPassed(ctx, task.ID); err != nil {
			return fmt.Errorf("标记已归档流程任务无需推进失败: task_id=%d: %w", task.ID, err)
		}
		return nil
	}
	if !engineType.IsValid() {
		engineType = domain.EngineTypeEasyFlow
	}
	mt, err := j.engine.GetAutomationTask(ctx, engineType, task.InstanceID, task.NodeID)
	if err != nil {
		// 节点在流程引擎中已不存在未完成记录（可能已被网关自动跳过、兄弟分支竞争通过或提前完结），直接标记推进避免死循环
		if errors.Is(err, gorm.ErrRecordNotFound) {
			j.logger.Info("流程引擎中该节点任务已完结或被跳过，直接标记自动化任务已推进",
				elog.Int64("taskId", task.ID),
				elog.String("procInstId", task.InstanceID.String()),
				elog.String("nodeId", task.NodeID))
			if markErr := j.svc.MarkTaskAsAutoPassed(ctx, task.ID); markErr != nil {
				return fmt.Errorf("标记无需推进任务失败: task_id=%d: %w", task.ID, markErr)
			}
			return nil
		}
		return fmt.Errorf("查询流程自动化节点任务失败: task_id=%d: %w", task.ID, err)
	}

	if !mt.TaskID.IsZero() {
		err = j.engine.Pass(ctx, domain.TaskPassReq{
			EngineType: engineType,
			TaskId:     mt.TaskID,
			Operator:   "SYSTEM",
			Comment:    "任务执行完成",
		})
		if err != nil {
			return fmt.Errorf("通过自动化节点失败: task_id=%d: %w", task.ID, err)
		}
	}

	if err = j.svc.MarkTaskAsAutoPassed(ctx, task.ID); err != nil {
		return fmt.Errorf("标记自动化任务已推进失败: task_id=%d: %w", task.ID, err)
	}
	return nil
}
