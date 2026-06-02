package dispatch

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/Duke1616/ecmdb/pkg/cryptox"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/mqx"
	"github.com/ecodeclub/ekit/slice"
	"github.com/ecodeclub/mq-api"
	"github.com/gotomicro/ego/core/elog"
	"github.com/robfig/cron/v3"
)

var topicRegexp = regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)

// isValidKafkaTopic 验证给定的字符串是否符合 Kafka 合法的 Topic 命名规范
func isValidKafkaTopic(topic string) bool {
	if len(topic) == 0 || len(topic) > 249 {
		return false
	}
	return topicRegexp.MatchString(topic)
}

// AgentExecuteEvent 任务在 Kafka 下发时传递给 Agent 的事件消息
type AgentExecuteEvent struct {
	TaskId    int64                  `json:"task_id"`
	Topic     string                 `json:"topic"`
	Handler   string                 `json:"handler"`
	Language  string                 `json:"language"`
	Code      string                 `json:"code"`
	Args      map[string]interface{} `json:"args"`
	Variables string                 `json:"variables"`
}

type kafkaService struct {
	mq       mq.MQ
	producer *mqx.MultipleProducer[AgentExecuteEvent]
	crypto   cryptox.Crypto
	cron     *cron.Cron
	logger   *elog.Component
}

// NewKafkaService 实例化基于 Kafka 渠道派发自动化任务的服务，补全 Crypto 依赖
func NewKafkaService(q mq.MQ, crypto cryptox.Crypto) TaskDispatcher {
	p := mqx.NewMultipleProducer[AgentExecuteEvent](q)
	k := &kafkaService{
		mq:       q,
		producer: p,
		crypto:   crypto,
		cron:     cron.New(cron.WithSeconds()),
		logger:   elog.DefaultLogger.With(elog.FieldComponentName("kafkaService")),
	}
	k.cron.Start()
	return k
}

// Dispatch 派发任务到 Kafka 消息管道
func (e *kafkaService) Dispatch(ctx context.Context, task domain.Task) error {
	if task.IsTiming && task.ScheduledTime > time.Now().UnixMilli() {
		return e.scheduleTimingTask(ctx, task)
	}

	return e.immediateDispatch(ctx, task)
}

func (e *kafkaService) immediateDispatch(ctx context.Context, task domain.Task) error {
	if !isValidKafkaTopic(task.Target) {
		return fmt.Errorf("任务下发的目标 (Target = %q) 并非合法的 Kafka Topic。合法的 Kafka Topic 长度必须在 1-249 之间，且只能由字母、数字、点(.)、下划线(_)和中划线(-)组成", task.Target)
	}

	// 确保基础 Topic 生产者缓存就绪
	if err := e.mq.CreateTopic(ctx, task.Target, 1); err != nil {
		e.logger.Debug("自动确保 Topic 状态", elog.FieldErr(err), elog.String("topic", task.Target))
	}

	if err := e.producer.AddProducer(task.Target); err != nil {
		return fmt.Errorf("初始化 Topic %s 生产者失败: %w", task.Target, err)
	}

	evt := AgentExecuteEvent{
		TaskId:    task.Id,
		Topic:     task.Target,
		Handler:   task.Handler,
		Code:      task.Code,
		Language:  task.Language,
		Args:      task.Args,
		Variables: e.decryptVariables(task.Variables),
	}

	if err := e.producer.Produce(ctx, task.Target, evt); err != nil {
		e.logger.Error("工作节点发送 Kafka 指令失败", elog.FieldErr(err), elog.Any("event", evt))
		return err
	}

	e.logger.Info("工作节点 Kafka 任务发送成功", elog.Int64("taskId", task.Id), elog.String("topic", task.Target))
	return nil
}

func (e *kafkaService) scheduleTimingTask(ctx context.Context, task domain.Task) error {
	jobTime := time.UnixMilli(task.ScheduledTime).Format("05 04 15 02 01 ?")
	_, err := e.cron.AddFunc(jobTime, func() {
		if err := e.immediateDispatch(ctx, task); err != nil {
			e.logger.Error("定时 Kafka 任务执行发送失败", elog.FieldErr(err), elog.Int64("taskId", task.Id))
			return
		}
		e.logger.Info("定时 Kafka 任务发送成功", elog.Int64("taskId", task.Id))
	})
	if err != nil {
		return err
	}

	e.logger.Info("定时 Kafka 任务开始调度", elog.Any("time", jobTime), elog.Int64("taskId", task.Id))
	return nil
}

// decryptVariables 处理变量，进行解密
func (e *kafkaService) decryptVariables(req []domain.Variables) string {
	variables := slice.Map(req, func(idx int, src domain.Variables) domain.Variables {
		if src.Secret {
			val, er := e.crypto.Decrypt(src.Value)
			if er != nil {
				return domain.Variables{}
			}

			return domain.Variables{
				Key:    src.Key,
				Value:  val,
				Secret: src.Secret,
			}
		}

		return domain.Variables{
			Key:    src.Key,
			Value:  src.Value,
			Secret: src.Secret,
		}
	})

	jsonVar, _ := json.Marshal(variables)
	return string(jsonVar)
}
