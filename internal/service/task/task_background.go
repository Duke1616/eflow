package task

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
)

type ExecuteResultConsumer struct {
	consumer mq.Consumer
	svc      Task
	logger   *elog.Component
}

func NewExecuteResultConsumer(q mq.MQ, svc Task) (*ExecuteResultConsumer, error) {
	consumer, err := q.Consumer(event.ExecuteResultEventName, "task_receive_execute")
	if err != nil {
		return nil, err
	}
	return &ExecuteResultConsumer{consumer: consumer, svc: svc, logger: elog.DefaultLogger}, nil
}

func (c *ExecuteResultConsumer) Start(ctx context.Context) {
	go func() {
		for {
			if err := c.Consume(ctx); err != nil {
				c.logger.Error("同步修改任务执行状态失败", elog.FieldErr(err))
				time.Sleep(time.Second)
			}
		}
	}()
}

func (c *ExecuteResultConsumer) Consume(ctx context.Context) error {
	cm, err := c.consumer.Consume(ctx)
	if err != nil {
		return fmt.Errorf("获取消息失败: %w", err)
	}
	var evt event.ExecuteResultEvent
	if err = json.Unmarshal(cm.Value, &evt); err != nil {
		return fmt.Errorf("解析消息失败: %w", err)
	}
	triggerPosition := domain.TriggerPositionTaskExecutionSuccess
	if domain.TaskStatus(evt.Status) == domain.FAILED {
		triggerPosition = domain.TriggerPositionTaskExecutionFailed
	}
	_, err = c.svc.UpdateTaskResult(ctx, domain.TaskResult{
		Id:              evt.TaskId,
		Result:          evt.Result,
		WantResult:      evt.WantResult,
		TriggerPosition: triggerPosition.ToString(),
		Status:          domain.TaskStatus(evt.Status),
	})
	return err
}

type StartTaskJob struct {
	svc      Task
	limit    int64
	interval time.Duration
	logger   *elog.Component
}

func NewStartTaskJob(svc Task, limit int64, interval time.Duration) *StartTaskJob {
	return &StartTaskJob{svc: svc, limit: limit, interval: interval, logger: elog.DefaultLogger}
}

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
			if err := j.svc.StartTask(context.Background(), task.Id); err != nil {
				j.logger.Error("任务启动失败", elog.FieldErr(err), elog.Int64("taskId", task.Id))
			}
		}()
	}
	return nil
}

type TaskRecoveryJob struct {
	svc      Task
	limit    int64
	interval time.Duration
	logger   *elog.Component
}

func NewTaskRecoveryJob(svc Task, limit int64, interval time.Duration) *TaskRecoveryJob {
	return &TaskRecoveryJob{svc: svc, limit: limit, interval: interval, logger: elog.DefaultLogger}
}

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
