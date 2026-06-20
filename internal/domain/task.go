package domain

import "time"

// Kind 自动化作业任务派发执行渠道定义
type Kind string

const (
	// KAFKA 通过消息队列推送触发执行节点
	KAFKA Kind = "KAFKA"
	// GRPC 绑定分布式任务平台 gRPC 客户端执行节点
	GRPC Kind = "GRPC"
)

// ToString 转换为基础 string 属性
func (s Kind) ToString() string {
	return string(s)
}

// TaskStatus 自动化作业执行任务的状态定义
type TaskStatus uint8

// ToUint8 转换为基本 uint8 表达
func (s TaskStatus) ToUint8() uint8 {
	return uint8(s)
}

const (
	// SUCCESS 自动化执行成功办结
	SUCCESS TaskStatus = 1
	// FAILED 自动化执行失败
	FAILED TaskStatus = 2
	// RUNNING 正在运行派发中
	RUNNING TaskStatus = 3
	// WAITING 流程刚流转到该节点，初始化等待就绪
	WAITING TaskStatus = 4
	// BLOCKED 挂起或阻塞，找不到执行路由或参数异常，流程卡主等待干预
	BLOCKED TaskStatus = 5
	// SCHEDULED 任务已提交给调度系统派发，或定时器处于等待触发状态
	SCHEDULED TaskStatus = 6
)

// TaskArgs 自动化任务运行时透传的临时变量字典
type TaskArgs map[string]interface{}

// Task 自动化作业执行任务领域模型
// 纯净领域对象，去除了表现层所有的 JSON struct tags
type Task struct {
	Id              int64       // 自动化作业唯一 ID
	TicketID        int64       // 关联工单单据 ID
	ProcessInstId   int         // 关联工作流流程实例 ID
	CurrentNodeId   string      // 当前流程自动化节点 ID
	TriggerPosition string      // 最近一次状态变更或异常触发位置
	WorkflowId      int64       // 关联工作流定义 ID
	CodebookId      int64       // 关联的脚本库唯一 ID
	Code            string      // 待运行的自动化脚本源码快照
	Language        string      // 脚本编写语言 (python, shell 等)
	Args            TaskArgs    // 流程变量额外透传的临时入参属性字典
	Variables       []Variables // 作业绑定执行所输入的参数变量集合
	Status          TaskStatus  // 任务当前的执行状态
	Result          string      // 任务执行的日志或返回值结果
	WantResult      string      // 期待的正确返回值 (用于节点条件自动决策)
	ExternalId      string      // 分布式任务平台反馈给本系统的运行实例 ID
	StartTime       int64       // 作业任务实际开始运行的时间 (毫秒时间戳)
	EndTime         int64       // 作业任务完成运行的时间 (毫秒时间戳)
	RetryCount      int         // 自动重试的计数值
	IsTiming        bool        // 是否为定时任务
	ScheduledTime   int64       // 计划执行时间 (毫秒时间戳)
	Kind            Kind        // 执行派发渠道
	Target          string      // 执行目标 (Topic 或 ServiceName)
	Handler         string      // 执行方法
	Ctime           int64       // 创建时间 (毫秒级时间戳)
	Utime           int64       // 更新时间 (毫秒级时间戳)
}

// TaskResult 自动化作业执行的反馈结果对象模型
type TaskResult struct {
	Id              int64      // 任务 ID
	TriggerPosition string     // 异常触发阶段
	WantResult      string     // 期待的结果
	Result          string     // 实际的输出日志或结果
	Status          TaskStatus // 结束或重试的临时执行状态
	Time            time.Time  // 反馈记录的系统时间
	StartTime       int64      // 本次重试/运行开始的毫秒时间戳
	EndTime         int64      // 本次运行结束的毫秒时间戳
	RetryCount      int        // 每次重试自增计数值
}

// Variables 作业绑定或传递的参数变量键值对
type Variables struct {
	Key    string // 变量名
	Value  string // 变量值
	Secret bool   // 是否属于敏感加密参数
}

type TriggerPosition string

func (t TriggerPosition) ToString() string {
	return string(t)
}

const (
	TriggerPositionTaskWaiting          TriggerPosition = "任务等待"
	TriggerPositionReadyToStartNode     TriggerPosition = "准备启动节点"
	TriggerPositionDispatchDelivered    TriggerPosition = "分发已送达执行端，当前任务执行中"
	TriggerPositionTaskExecutionSuccess TriggerPosition = "任务执行成功"
	TriggerPositionTaskExecutionFailed  TriggerPosition = "任务执行失败"

	TriggerPositionManualRetry            TriggerPosition = "人工手动重试"
	TriggerPositionAutoRetry              TriggerPosition = "自动补发任务"
	TriggerPositionAutoRetryLimitExceeded TriggerPosition = "超过最大重试次数"
	TriggerPositionManualSuccess          TriggerPosition = "手动修改状态为成功"

	TriggerPositionErrorGetProcessInst        TriggerPosition = "获取流程实例失败"
	TriggerPositionErrorGetProcessInfo        TriggerPosition = "获取流程信息失败"
	TriggerPositionErrorExtractAutomationInfo TriggerPosition = "提取自动化信息失败"
	TriggerPositionErrorGetDispatcherNode     TriggerPosition = "获取调度节点失败"
	TriggerPositionErrorGetTaskTemplate       TriggerPosition = "获取任务模版失败"
)
