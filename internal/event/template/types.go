package template

const (
	// WechatTicketEventName 企业微信 OA 创建工单队列事件名 (变量名重构规范为 Ticket，底层 Kafka 主题保持 order 以兼容生产)
	WechatTicketEventName = "wechat_order_events"

	// WechatCallbackEventName 企业微信接收 OA 回调事件名
	WechatCallbackEventName = "wechat_callback_events"
)
