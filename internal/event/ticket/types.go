package ticket

import (
	"fmt"
	"strconv"
)

const (
	// WechatTicketEventName 接收企业微信 OA 审批的回调事件名 (变量名规范为 Ticket，底层 Kafka 保持 order 以兼容生产)
	WechatTicketEventName = "wechat_order_events"
	// LarkCallbackEventName 接收飞书审批消息卡片回调的事件名
	LarkCallbackEventName = "lark_callback_events"
)

// Action 飞书卡片交互行为
type Action string

const (
	Pass     Action = "pass"
	Reject   Action = "reject"
	Progress Action = "progress"
	Revoke   Action = "revoke"
)

// LarkCallback 飞书卡片交互触发的 JSON 回调结构体
type LarkCallback struct {
	Action    Action                 `json:"action"`
	MessageId string                 `json:"message_id"`
	UserId    string                 `json:"user_id"`
	OpenId    string                 `json:"open_id"`
	FormValue map[string]interface{} `json:"form_value"`
	Value     map[string]interface{} `json:"value"`
}

func (l *LarkCallback) GetTenantId() int64 {
	if l.Value == nil {
		return 0
	}
	if v, ok := l.Value["tenant_id"].(string); ok {
		tid, _ := strconv.ParseInt(v, 10, 64)
		return tid
	}
	if v, ok := l.Value["tenant_id"].(float64); ok {
		return int64(v)
	}
	if v, ok := l.Value["tenant_id"].(int64); ok {
		return v
	}
	return 0
}

func (l *LarkCallback) GetMessageId() string {
	return l.MessageId
}

func (l *LarkCallback) GetTicketId() string {
	if l.Value == nil {
		return ""
	}
	if v, ok := l.Value["ticket_id"].(string); ok {
		return v
	}
	return ""
}

func (l *LarkCallback) GetTicketIdInt() (int64, error) {
	val := l.GetTicketId()
	if val == "" {
		return 0, fmt.Errorf("ticket_id is empty")
	}
	return strconv.ParseInt(val, 10, 64)
}

func (l *LarkCallback) GetTaskId() string {
	if v, ok := l.Value["task_id"].(string); ok {
		return v
	}
	return ""
}

func (l *LarkCallback) GetFormValue() map[string]interface{} {
	delete(l.FormValue, "comment")
	return l.FormValue
}

func (l *LarkCallback) GetTaskIdInt() (int, error) {
	val := l.GetTaskId()
	if val == "" {
		return 0, fmt.Errorf("task_id is empty")
	}
	return strconv.Atoi(val)
}

func (l *LarkCallback) GetAction() Action {
	if l.Action != "" {
		return l.Action
	}

	if v, ok := l.Value["action"].(string); ok {
		return Action(v)
	}

	return ""
}

func (l *LarkCallback) GetComment() string {
	if v, ok := l.FormValue["comment"].(string); ok {
		if v == "" {
			return "无"
		}
		return v
	}
	return "无"
}

func (l *LarkCallback) GetUserId() string {
	return l.UserId
}

func (l *LarkCallback) GetOpenId() string {
	return l.OpenId
}
