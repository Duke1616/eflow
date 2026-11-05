package engine

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/Duke1616/eflow/internal/domain"
)

// ID 引擎实体（任务、实例等）强类型自适应标识
// 支持 int 与 string 的无缝安全转换，消除底层代码对 any 的防御性反射断言
type ID string

// NewID 从任意整型或字符串创建强类型 ID（泛型工厂方法）
func NewID[T ~int | ~int64 | ~int32 | ~string](val T) ID {
	return ID(fmt.Sprint(val))
}

// Int 安全解析为整型，失败返回 0
func (id ID) Int() int {
	v, _ := strconv.Atoi(string(id))
	return v
}

// Int64 安全解析为 64 位整型，失败返回 0
func (id ID) Int64() int64 {
	v, _ := strconv.ParseInt(string(id), 10, 64)
	return v
}

// String 返回字符串形态
func (id ID) String() string {
	return string(id)
}

// IsZero 判断是否为零值
func (id ID) IsZero() bool {
	return id == "" || id == "0"
}

// DeployInfo 流程引擎部署完成返回的元信息
type DeployInfo struct {
	ProcessID int // 底层引擎的标识（若引擎无自增ID则为 0）
	Version   int // 部署发布的版本号
}

// ProcessLaunchReq 工单发起流程实例的强类型入参结构（单一事实来源 SSOT）
type ProcessLaunchReq struct {
	EngineType   domain.EngineType // 流程底层引擎类型（可选，未指定时由调度器自动判定或默认兜底）
	TicketId     int64             // 业务工单唯一主键 ID（作为流程实例对齐的基石）
	WorkflowId   int64             // 工作流定义主键 ID
	ProcessId    int               // 兼容底层引擎的定义 ID（easy-workflow 为自增ID，flowx 为 wf.Id）
	WorkflowName string            // 流程名称
	Starter      string            // 流程发起人用户名/工号
	Variables    map[string]any    // 表单初始变量快照字典（权威唯一结构）
}

// Validate 编译期与运行时防呆自卫校验（Fail-Fast）
func (r ProcessLaunchReq) Validate() error {
	if r.TicketId <= 0 {
		return errors.New("工单 TicketId 非法")
	}
	if r.WorkflowId <= 0 {
		return errors.New("流程定义 WorkflowId 非法")
	}
	if r.Starter == "" {
		return errors.New("流程发起人 Starter 不能为空")
	}
	return nil
}

// ProcessLaunchResult 流程实例启动成功的返回结果
type ProcessLaunchResult struct {
	InstanceId  int    // 整数型实例 ID（用于兼容回写 ticket.process_instance_id）
	InstanceKey string // 字符串型全局实例唯一标识（如 flowx 的实例原生 ID）
}

// TaskPassReq 审批通过请求结构
type TaskPassReq struct {
	TaskId    ID             // 强类型任务唯一标识
	Operator  string         // 审批操作人
	Comment   string         // 审批意见
	Variables map[string]any // 增量变量合并
}

// Validate 审批通过入参自卫校验
func (r TaskPassReq) Validate() error {
	if r.TaskId.IsZero() {
		return errors.New("任务 ID 不能为空")
	}
	if r.Operator == "" {
		return errors.New("审批操作人 Operator 不能为空")
	}
	return nil
}

// TaskRejectReq 审批驳回请求结构
type TaskRejectReq struct {
	TaskId       ID     // 强类型任务唯一标识
	Operator     string // 审批操作人
	Comment      string // 驳回原因（必填）
	TargetNodeId string // 回退目标节点 ID（可选）
}

// Validate 审批驳回入参自卫校验
func (r TaskRejectReq) Validate() error {
	if r.TaskId.IsZero() {
		return errors.New("任务 ID 不能为空")
	}
	if r.Operator == "" {
		return errors.New("审批操作人 Operator 不能为空")
	}
	if r.Comment == "" {
		return errors.New("驳回原因 Comment 不能为空")
	}
	return nil
}

// TimelineNode 跨引擎统一流转时间线节点视图
type TimelineNode struct {
	NodeID       string         // 流程节点全局唯一标识
	NodeName     string         // 流程节点展示名称
	NodeType     string         // 节点类型（如 START, APPROVAL, AUTOMATION, END）
	Status       string         // 节点流转状态（PENDING, RUNNING, COMPLETED, REJECTED, TERMINATED, SKIPPED）
	Batch        int            // 执行批次号（用于回退场景区分多次流转）
	Operator     string         // 处理人账号/工号
	OperatorName string         // 处理人中文真名
	Comment      string         // 审批意见或流转批注
	StartTime    string         // 节点激活/到达时间
	EndTime      string         // 节点完结/流出时间
	Extra        map[string]any // 扩展信息（如增量变量、执行器详情）
}

// TaskContext 跨引擎统一的任务流转运行时上下文快照
// 供工单业务层执行审批鉴权、节点表单查找、状态判断使用
type TaskContext struct {
	EngineType        domain.EngineType // 所属工作流底层引擎类型
	TaskID            ID                // 任务强类型 ID
	ProcessInstanceID ID                // 流程实例强类型 ID
	ProcessID         int               // 流程定义 ID
	ProcessVersion    int               // 流程定义版本号
	NodeID            string            // 节点物理标识
	NodeName          string            // 节点名称
	Assignee          string            // 当前指派处理人账号
	IsFinished        bool              // 是否已完结
	Comment           string            // 审批批注
}
