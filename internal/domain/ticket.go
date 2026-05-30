package domain

import (
	"errors"
	"fmt"
)

// Channel 消息通知媒介/渠道定义
type Channel string

// String 转换为基础 string
func (c Channel) String() string {
	return string(c)
}

const (
	// ChannelLarkCard 飞书互动卡片通知渠道
	ChannelLarkCard Channel = "LARK_CARD"
	// ChannelEmail 通用邮件通知渠道
	ChannelEmail Channel = "EMAIL"
	// ChannelInApp 系统站内信通知渠道
	ChannelInApp Channel = "IN_APP"
)

// Status 工单流转实例的状态
type Status uint8

// ToUint8 转换为基本 uint8 表达
func (s Status) ToUint8() uint8 {
	return uint8(s)
}

// ToInt 转换为基本 int 表达
func (s Status) ToInt() int {
	return int(s)
}

const (
	// START 工单发起，等待审批节点激活/开始
	START Status = 1
	// PROCESS 流程节点运行中，审批人审批中
	PROCESS Status = 2
	// END 流程流转圆满结束，工单已办结通过
	END Status = 3
	// WITHDRAW 发起人主动撤回工单
	WITHDRAW Status = 4
)

// Provide 工单的发起/同步来源提供者
type Provide uint8

// ToUint8 转换为基本 uint8 表达
func (s Provide) ToUint8() uint8 {
	return uint8(s)
}

const (
	// SYSTEM 本工单平台系统直接创建
	SYSTEM Provide = 1
	// WECHAT 企业微信等外部渠道同步过来创建
	WECHAT Provide = 2
	// ALERT 监控告警系统自动触发生成的告警工单
	ALERT Provide = 3
)

// IsValid 检查来源是否合法
func (s Provide) IsValid() bool {
	return s == SYSTEM || s == WECHAT || s == ALERT
}

// IsAlert 检查是否属于告警自动转单来源
func (s Provide) IsAlert() bool {
	return s == ALERT
}

// TicketData 工单实时填写的动态表单数值集
type TicketData map[string]interface{}

// Ticket 具体的工单流转实例领域模型（核心业务对象，对应原 ecmdb 中的 Order 概念）
type Ticket struct {
	Id               int64            // 工单实例 ID
	BizID            int64            // 外部关联的业务实体 ID
	Key              string           // 工单的唯一业务 Key (用以幂等校验等)
	TemplateId       int64            // 所关联的页面渲染模板 ID
	WorkflowId       int64            // 所关联的工作流流程定义 ID
	Data             TicketData       // 用户在表单中填写的工单实时数据属性集合
	Status           Status           // 工单当前流转状态
	Provide          Provide          // 工单来源渠道提供商
	CreateBy         string           // 工单创建/发起人 ID 或是系统名
	Process          Process          // 绑定的流程引擎运行实例信息
	Ctime            int64            // 发起创建时间 (毫秒级时间戳)
	Wtime            int64            // 完成归档时间
	NotificationConf NotificationConf // 为支持告警自动转单等引入的外部通知媒介配置
}

// ErrInvalidParameter 统一的校验失败错误定义
var ErrInvalidParameter = errors.New("参数校验错误")

// Validate 校验工单创建/更新请求的合法性，防止脏数据入库
func (o *Ticket) Validate() error {
	if o.TemplateId <= 0 {
		return fmt.Errorf("%w: Template.ID = %d", ErrInvalidParameter, o.TemplateId)
	}

	if o.WorkflowId <= 0 {
		return fmt.Errorf("%w: WorkFlow.ID = %d", ErrInvalidParameter, o.WorkflowId)
	}

	if !o.Provide.IsValid() {
		return fmt.Errorf("%w: 不支持的来源提供商", ErrInvalidParameter)
	}

	if o.CreateBy == "" {
		return fmt.Errorf("%w: 工单创建人不能为空", ErrInvalidParameter)
	}

	return nil
}

// Process 承载流程引擎对于该工单底层流转的实例映射信息
type Process struct {
	InstanceId int // 绑定的流程执行引擎实例 ID (即 proc_inst 表中的 ID)
}

// NotificationParams 发送消息通知携带的特定业务参数字典
type NotificationParams map[string]interface{}

// NotificationConf 承载外部触发所携带的通知选项配置
type NotificationConf struct {
	TemplateID     int64              // 消息模板 ID
	TemplateParams NotificationParams // 发送通知所携带的参数字典
	Channel        Channel            // 通知的发送媒介渠道
}

// FormValue 审批节点中，某一个具体表单控件所输入或展示的值快照
type FormValue struct {
	Name  string      // 表单字段展示标签
	Key   string      // 表单字段 key 标识
	Type  string      // 字段展现渲染类型 (如 Input、Select)
	Value interface{} // 用户在审批该节点时所输入的表单值数据
}

// TaskForm 流程任务某一步骤下的静态数据和审批动作表单数据实体
type TaskForm struct {
	Id      int64       // 快照 ID
	OrderId int64       // 关联工单 ID
	TaskId  int         // 关联的底层任务节点 ID
	Name    string      // 步骤节点名称
	Key     string      // 节点 key 标识
	Type    string      // 快照表单类型
	Value   interface{} // 填写的表单具体值
	Ctime   int64       // 快照创建时间
}
