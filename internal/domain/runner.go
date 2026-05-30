package domain

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/errs"
)

// Action 执行器运行节点的活跃/注册动作状态
type Action uint8

// ToUint8 转换为基本 uint8 表达
func (s Action) ToUint8() uint8 {
	return uint8(s)
}

const (
	// REGISTER 执行器成功在平台上注册并处于活跃待命状态
	REGISTER Action = 1
	// UNREGISTER 执行器注销下线
	UNREGISTER Action = 2
)

// Runner 执行自动化作业的外部运行节点领域模型
// 纯净领域对象。其中 Variables 直接复用 task.go 中定义好的通用结构体，不重复声明。
type Runner struct {
	Id             int64
	TenantID       string // 租户隔离标识
	Name           string
	CodebookUid    string
	CodebookSecret string
	Kind           Kind     // 派发通信协议类型 (KAFKA / GRPC)
	Target         string   // 派发物理目标 (如 Kafka Topic 或者是 gRPC 绑定的 ServiceName)
	Handler        string   // 执行器需要调用的具体业务方法
	Tags           []string // 执行器标签绑定集，自动化任务通过匹配这些标签来挑选路由执行器
	Action         Action   // 当前执行器的活跃状态
	Desc           string
	Variables      []Variables
	Ctime          int64 // 创建时间
	Utime          int64 // 更新时间
}

// Validate 创建/更新时的领域模型自身校验
func (r *Runner) Validate() error {
	if r.Name == "" {
		return fmt.Errorf("%w: Name = %q", errs.ErrInvalidParameter, r.Name)
	}
	if r.CodebookUid == "" {
		return fmt.Errorf("%w: CodebookUid = %q", errs.ErrInvalidParameter, r.CodebookUid)
	}
	if r.Kind == "" {
		return fmt.Errorf("%w: Kind = %q", errs.ErrInvalidParameter, r.Kind)
	}
	if r.Target == "" {
		return fmt.Errorf("%w: Target = %q", errs.ErrInvalidParameter, r.Target)
	}
	if r.Handler == "" {
		return fmt.Errorf("%w: Handler = %q", errs.ErrInvalidParameter, r.Handler)
	}
	return nil
}

// IsKindKafka 判定此执行器是否是基于消息队列进行作业分发的模式
func (r Runner) IsKindKafka() bool {
	return r.Kind == KAFKA
}

// TagDetail 描述标签的派发路由映射细节
type TagDetail struct {
	Kind    Kind
	Target  string
	Handler string
}

// RunnerTags 单个独立脚本与可用运行器标记映射之间的关系组合模型
type RunnerTags struct {
	CodebookUid string
	TagsMapping map[string]TagDetail // Tag 到对应执行器派发规则的映射字典
}
