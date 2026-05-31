package task

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	executorv1 "github.com/Duke1616/ecmdb/api/proto/gen/etask/executor/v1"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/gotomicro/ego/core/elog"
)

// TaskExecutionSyncJob 远程分布式任务执行状态定时拉取与同步后台任务
type TaskExecutionSyncJob struct {
	svc         Service
	executorSvc executorv1.TaskExecutionServiceClient
	limit       int64
	interval    time.Duration
	logger      *elog.Component
}

// NewTaskExecutionSyncJob 实例化远程分布式任务状态同步任务
func NewTaskExecutionSyncJob(svc Service, executorSvc executorv1.TaskExecutionServiceClient, limit int64, interval time.Duration) *TaskExecutionSyncJob {
	return &TaskExecutionSyncJob{
		svc:         svc,
		executorSvc: executorSvc,
		limit:       limit,
		interval:    interval,
		logger:      elog.DefaultLogger.With(elog.FieldComponentName("TaskExecutionSyncJob")),
	}
}

// Start 启动分布式任务执行结果状态的定时拉取与校准同步协程
func (j *TaskExecutionSyncJob) Start(ctx context.Context) {
	ticker := time.NewTicker(j.interval)
	defer ticker.Stop()
	for {
		if err := j.run(ctx); err != nil {
			j.logger.Error("状态自动同步失败", elog.FieldErr(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (j *TaskExecutionSyncJob) run(ctx context.Context) error {
	offset := int64(0)
	for {
		tasks, total, err := j.svc.ListTaskByStatusAndKind(ctx, offset, j.limit,
			domain.RUNNING.ToUint8(),
			domain.GRPC.ToString())
		if err != nil {
			return fmt.Errorf("获取运行中任务列表失败: %w", err)
		}

		var syncTasks []domain.Task
		var taskIds []int64

		for _, task := range tasks {
			if task.ExternalId == "" {
				continue
			}

			taskId, err := strconv.ParseInt(task.ExternalId, 10, 64)
			if err != nil {
				j.logger.Error("解析 external_id 失败", elog.FieldErr(err), elog.String("external_id", task.ExternalId))
				continue
			}

			syncTasks = append(syncTasks, task)
			taskIds = append(taskIds, taskId)
		}

		if len(taskIds) > 0 {
			j.batchSyncTaskExecutions(ctx, taskIds, syncTasks)
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

func (j *TaskExecutionSyncJob) batchSyncTaskExecutions(ctx context.Context, taskIds []int64, syncTasks []domain.Task) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := j.executorSvc.BatchListTaskExecutions(reqCtx, &executorv1.BatchListTaskExecutionsRequest{
		TaskIds: taskIds,
	})
	if err != nil {
		j.logger.Error("批量查询远程任务执行记录失败", elog.FieldErr(err))
		return
	}

	var wg sync.WaitGroup
	for _, task := range syncTasks {
		taskId, _ := strconv.ParseInt(task.ExternalId, 10, 64)
		execList, ok := resp.Results[taskId]

		if !ok || len(execList.Executions) == 0 {
			continue
		}

		var latest *executorv1.TaskExecution
		for _, exec := range execList.Executions {
			if latest == nil || exec.Id > latest.Id {
				latest = exec
			}
		}

		if latest == nil {
			continue
		}

		wg.Add(1)
		go func(t domain.Task, latestExec *executorv1.TaskExecution) {
			defer wg.Done()
			var updateErr error
			switch latestExec.Status {
			case executorv1.ExecutionStatus_SUCCESS:
				_, updateErr = j.svc.UpdateTaskStatus(ctx, domain.TaskResult{
					Id:              t.Id,
					Status:          domain.SUCCESS,
					Result:          "任务执行成功",
					WantResult:      latestExec.TaskResult,
					TriggerPosition: domain.TriggerPositionTaskExecutionSuccess.ToString(),
				})
			case executorv1.ExecutionStatus_FAILED, executorv1.ExecutionStatus_FAILED_RETRYABLE, executorv1.ExecutionStatus_FAILED_RESCHEDULABLE:
				_, updateErr = j.svc.UpdateTaskStatus(ctx, domain.TaskResult{
					Id:              t.Id,
					Status:          domain.FAILED,
					Result:          "任务执行失败",
					WantResult:      latestExec.TaskResult,
					TriggerPosition: domain.TriggerPositionTaskExecutionFailed.ToString(),
				})
			default:
				return
			}

			if updateErr != nil {
				j.logger.Error("更新本地任务状态失败", elog.FieldErr(updateErr), elog.Int64("task_id", t.Id))
			} else {
				j.logger.Info("同步任务状态成功", elog.Int64("task_id", t.Id), elog.Any("status", latestExec.Status))
			}
		}(task, latest)
	}
	wg.Wait()
}
