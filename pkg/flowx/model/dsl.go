package model

import (
	"fmt"
	"time"

	"github.com/samber/lo"
)

// NodeType 流程节点类型
type NodeType string

const (
	NodeStart            NodeType = "START"             // 开始节点
	NodeEnd              NodeType = "END"               // 结束节点
	NodeApproval         NodeType = "APPROVAL"          // 人工审批节点
	NodeCarbonCopy       NodeType = "CC"                // 抄送节点 (只读知会相关人，瞬时穿透推进，不阻塞流程)
	NodeChatGroup        NodeType = "CHAT_GROUP"        // 群聊通知节点 (IM 群聊广播通知，瞬时穿透，不阻塞流程)
	NodeTask             NodeType = "TASK"              // 自动化任务节点
	NodeGatewayExclusive NodeType = "GATEWAY_EXCLUSIVE" // 排他网关 (XOR / 或网关)：顺次求值，首个命中立即短路，仅走单条分支；汇聚任一到达即放行
	NodeGatewayParallel  NodeType = "GATEWAY_PARALLEL"  // 并行网关 (AND / 并网关)：无条件全部分支并发激活；汇聚等待全部前置入边完成
	NodeGatewayInclusive NodeType = "GATEWAY_INCLUSIVE" // 包容网关 (OR / 条件并行)：激活所有满足条件的分支；未命中自动 Skip；汇聚等待有效分支
)

// SignRule 审批仲裁规则
type SignRule string

const (
	SignOneVote SignRule = "ONE_VOTE" // 或签 (一人同意即可通过)
	SignAllPass SignRule = "ALL_PASS" // 会签 (需所有人同意才可通过)
)

// ProcessDefinition 流程定义（标准 DSL 模型）
type ProcessDefinition struct {
	Key     string     `json:"key"`     // 流程唯一标识
	Name    string     `json:"name"`    // 流程名称
	Version int        `json:"version"` // 流程版本
	Nodes   []NodeSpec `json:"nodes"`   // 拓扑节点集合
	Edges   []EdgeSpec `json:"edges"`   // 有向边集合
}

// Validate 静态校验流程定义拓扑完整性、连通性与端点度数（Fail-Fast 实体领域自卫）
func (def ProcessDefinition) Validate() error {
	if def.Key == "" {
		return fmt.Errorf("流程唯一标识 (Key) 不能为空")
	}
	if len(def.Nodes) == 0 {
		return fmt.Errorf("流程节点列表不能为空")
	}

	if lo.SomeBy(def.Nodes, func(n NodeSpec) bool { return n.ID == "" }) {
		return fmt.Errorf("节点 ID 不能为空")
	}

	nodeIDs := lo.SliceToMap(def.Nodes, func(n NodeSpec) (string, struct{}) {
		return n.ID, struct{}{}
	})
	if len(nodeIDs) != len(def.Nodes) {
		return fmt.Errorf("存在重复的节点 ID")
	}

	startCount := lo.CountBy(def.Nodes, func(n NodeSpec) bool { return n.Type == NodeStart })
	if startCount != 1 {
		return fmt.Errorf("流程必须有且仅有 1 个开始节点 (NodeStart)，当前为 %d 个", startCount)
	}

	endCount := lo.CountBy(def.Nodes, func(n NodeSpec) bool { return n.Type == NodeEnd })
	if endCount < 1 {
		return fmt.Errorf("流程必须至少包含 1 个结束节点 (NodeEnd)")
	}

	inDegree := make(map[string]int, len(def.Nodes))
	outDegree := make(map[string]int, len(def.Nodes))

	for idx, e := range def.Edges {
		if _, ok := nodeIDs[e.Source]; !ok {
			return fmt.Errorf("边 #%d 的源节点不存在: %s", idx, e.Source)
		}
		if _, ok := nodeIDs[e.Target]; !ok {
			return fmt.Errorf("边 #%d 的目标节点不存在: %s", idx, e.Target)
		}
		if e.Source == e.Target {
			return fmt.Errorf("边 #%d 存在无效的自环连接: %s -> %s", idx, e.Source, e.Target)
		}
		outDegree[e.Source]++
		inDegree[e.Target]++
	}

	// 拓扑连通性与端点度数 Fail-Fast 静态防卫
	for _, n := range def.Nodes {
		switch n.Type {
		case NodeStart:
			if inDegree[n.ID] > 0 {
				return fmt.Errorf("开始节点 [%s] 不能拥有入边", n.ID)
			}
			if outDegree[n.ID] == 0 {
				return fmt.Errorf("开始节点 [%s] 必须至少包含 1 条出边", n.ID)
			}
		case NodeEnd:
			if outDegree[n.ID] > 0 {
				return fmt.Errorf("结束节点 [%s] 不能拥有出边", n.ID)
			}
			if inDegree[n.ID] == 0 {
				return fmt.Errorf("结束节点 [%s] 必须至少包含 1 条入边", n.ID)
			}
		default:
			if n.IsGateway() {
				if inDegree[n.ID] == 0 {
					return fmt.Errorf("网关节点 [%s] 悬空异常，必须至少包含 1 条入边", n.ID)
				}
				if outDegree[n.ID] == 0 {
					return fmt.Errorf("网关节点 [%s] 悬空异常，必须至少包含 1 条出边", n.ID)
				}
			}
		}
	}

	return nil
}

// RetryPolicy 自动化任务失败重试退避策略
type RetryPolicy struct {
	MaxAttempts     int           `json:"max_attempts"`               // 最大重试次数 (如 3 表示失败后最多重试 3 次)
	InitialInterval time.Duration `json:"initial_interval,omitempty"` // 初始重试间隔 (如 100ms)
	BackoffFactor   float64       `json:"backoff_factor,omitempty"`   // 退避指数倍数 (如 2.0，若 <= 1.0 则为固定间隔)
	MaxInterval     time.Duration `json:"max_interval,omitempty"`     // 封顶重试间隔 (防止指数退避无限膨胀)
}

// NodeSpec 节点规格定义
type NodeSpec struct {
	ID          string         `json:"id"`                     // 节点物理ID
	Name        string         `json:"name"`                   // 节点展示名称
	Type        NodeType       `json:"type"`                   // 节点类型
	WorkerType  string         `json:"worker_type,omitempty"`  // 自动化任务执行器类型 (当 Type 为 TASK 时生效)
	RetryPolicy *RetryPolicy   `json:"retry_policy,omitempty"` // 自动化任务失败重试退避策略
	SignRule    SignRule       `json:"sign_rule,omitempty"`    // 审批规则: ONE_VOTE, ALL_PASS
	Assignees   []string       `json:"assignees,omitempty"`    // 指派审批人账号列表
	Hooks       []string       `json:"hooks,omitempty"`        // 同步守卫与动态算人 Hook 声明
	Props       map[string]any `json:"props,omitempty"`        // 扩展配置属性
}

// IsActivity 是否属于活动/任务节点 (审批节点、自动化节点、抄送节点等具有业务执行或任务特征的实体)
func (n NodeSpec) IsActivity() bool {
	return n.Type == NodeApproval || n.Type == NodeTask || n.Type == NodeCarbonCopy || n.Type == NodeChatGroup
}

// IsGateway 是否属于分支或汇聚网关节点
func (n NodeSpec) IsGateway() bool {
	return n.Type == NodeGatewayExclusive || n.Type == NodeGatewayParallel || n.Type == NodeGatewayInclusive
}

// IsWaitState 是否属于等待态节点 (到达时会产生未决状态或人工待办，决定本轮流转是否不能立即全单办结)
func (n NodeSpec) IsWaitState() bool {
	return n.Type == NodeApproval
}

// CanRejectTo 是否允许作为驳回目标节点 (仅审批节点与开始节点具备回退资格)
func (n NodeSpec) CanRejectTo() bool {
	return n.Type == NodeApproval || n.Type == NodeStart
}

// EdgeSpec 边定义
type EdgeSpec struct {
	Source    string          `json:"source"`               // 源节点 ID
	Target    string          `json:"target"`               // 目标节点 ID
	Condition string          `json:"condition,omitempty"`  // 兼容简易表达式文本 (如 "days > 3")
	RuleGroup *ConditionGroup `json:"rule_group,omitempty"` // 强类型规则引擎条件组 (推荐优先使用)
}
