package ioc

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Duke1616/eflow/internal/event"
	templateEvent "github.com/Duke1616/eflow/internal/event/template"
	ticketEvent "github.com/Duke1616/eflow/internal/event/ticket"
	"github.com/ecodeclub/ekit/retry"
	"github.com/ecodeclub/mq-api"
	"github.com/ecodeclub/mq-api/kafka"
	"github.com/gotomicro/ego/core/elog"
	"github.com/spf13/viper"
)

var (
	q          mq.MQ
	mqInitOnce sync.Once
)

// TopicSpec 消息队列主题规格定义
type TopicSpec struct {
	Name       string
	Partitions int
}

// RequiredTopics 声明 eflow 系统运行所需的核心 Topic 拓扑（统一使用 1 分区保证全局严格时序与轻量部署）
var RequiredTopics = []TopicSpec{
	{Name: event.CreateProcessEventName, Partitions: 1},          // 流程创建启动 (create_process_events)
	{Name: event.ExecuteResultEventName, Partitions: 1},          // 自动化任务执行完成回传 (complete_topic)
	{Name: event.OrderStatusModifyEventName, Partitions: 1},      // 工单状态变更 (order_status_modify_events)
	{Name: templateEvent.WechatTicketEventName, Partitions: 1},   // 企业微信工单事件 (wechat_order_events)
	{Name: templateEvent.WechatCallbackEventName, Partitions: 1}, // 企业微信 OA 回调 (wechat_callback_events)
	{Name: ticketEvent.LarkCallbackEventName, Partitions: 1},     // 飞书卡片回调 (lark_callback_events)
}

func InitMQ() mq.MQ {
	mqInitOnce.Do(func() {
		const maxInterval = 10 * time.Second
		const maxRetries = 10
		strategy, err := retry.NewExponentialBackoffRetryStrategy(time.Second, maxInterval, maxRetries)
		if err != nil {
			panic(err)
		}
		for {
			q, err = initMQ()
			if err == nil {
				break
			}
			next, ok := strategy.Next()
			if !ok {
				panic("InitMQ 重试失败......")
			}
			time.Sleep(next)
		}
	})
	return q
}

func initMQ() (mq.MQ, error) {
	type Config struct {
		Network   string   `yaml:"network"`
		Addresses []string `yaml:"addresses"`
	}

	var cfg Config
	if err := viper.UnmarshalKey("kafka", &cfg); err != nil {
		panic(fmt.Errorf("unable to decode into struct: %v", err))
	}

	qq, err := kafka.NewMQ(cfg.Network, cfg.Addresses)
	if err != nil {
		return nil, err
	}

	// 幂等自愈创建系统所需的全部核心 Topic，消除新用户首次部署时主题缺失导致的消费报错
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, spec := range RequiredTopics {
		if createErr := qq.CreateTopic(ctx, spec.Name, spec.Partitions); createErr != nil {
			elog.Warn("自动创建 Topic 跳过或失败（若 Topic 已预建可忽略）",
				elog.String("topic", spec.Name),
				elog.Int("partitions", spec.Partitions),
				elog.FieldErr(createErr))
		}
	}

	return qq, nil
}
