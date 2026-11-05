package domain

// ChannelType 标识工单外部通知与交互渠道来源类型
type ChannelType string

const (
	// ChannelFeishu 飞书渠道
	ChannelFeishu ChannelType = "feishu"
	// ChannelWechat 企业微信渠道
	ChannelWechat ChannelType = "wechat"
	// ChannelDingTalk 钉钉渠道
	ChannelDingTalk ChannelType = "dingtalk"
	// ChannelMail 邮件渠道
	ChannelMail ChannelType = "mail"
)

// ChannelActionType 跨渠道统一的审批流转用户行为动作
type ChannelActionType string

const (
	// ChannelActionPass 审批通过
	ChannelActionPass ChannelActionType = "pass"
	// ChannelActionReject 审批驳回
	ChannelActionReject ChannelActionType = "reject"
	// ChannelActionRevoke 申请撤销
	ChannelActionRevoke ChannelActionType = "revoke"
	// ChannelActionProgress 进度查看
	ChannelActionProgress ChannelActionType = "progress"
)

// ChannelActionEvent 跨渠道统一的交互事件数据契约，抹平飞书、企微、钉钉等特定平台 Payload 差异
type ChannelActionEvent struct {
	// Channel 渠道类型标识
	Channel ChannelType `json:"channel"`
	// ChannelUserID 第三方平台用户唯一标识（如飞书 user_id/open_id、企业微信 userid）
	ChannelUserID string `json:"channel_user_id"`
	// Action 用户触发的审批动作
	Action ChannelActionType `json:"action"`
	// TenantID 多租户隔离标识
	TenantID int64 `json:"tenant_id"`
	// TicketID 关联业务工单 ID
	TicketID int64 `json:"ticket_id"`
	// TaskID 关联工作流任务节点 ID
	TaskID int `json:"task_id"`
	// MessageID 外部消息/卡片唯一标识（用于回执就地更新/Patch）
	MessageID string `json:"message_id"`
	// Comment 用户提交的审批意见或撤销原因
	Comment string `json:"comment"`
	// FormValues 任务节点用户填写的动态表单数据
	FormValues map[string]interface{} `json:"form_value"`
	// Extra 预留平台扩展上下文参数
	Extra map[string]interface{} `json:"extra"`
}

// ChannelActionResult 跨渠道外部动作执行结果领域实体，驱动渠道卡片状态流转与失败阻断
type ChannelActionResult struct {
	// Success 业务操作是否最终成功（底层 Pass/Reject 是否成功）
	Success bool `json:"success"`
	// Action 实际触发的动作类型
	Action ChannelActionType `json:"action"`
	// Operator 经身份解析器确认为可信系统用户的登录用户名
	Operator string `json:"operator"`
	// Remark 渲染给第三方渠道卡片或消息通知的文本反馈内容
	Remark string `json:"remark"`
	// Error 底层业务失败的原始错误（若执行成功则为 nil）
	Error error `json:"-"`
}

