package kernel

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// BranchState 描述汇聚网关上游分支在当前流转轮次的快照定态
type BranchState struct {
	NodeID    string // 前置有效节点 ID
	NodeName  string // 前置节点名称
	HasTask   bool   // 该分支是否已到达并产生物理任务
	IsPassed  bool   // 该分支是否已满足流转推进条件
	IsSkipped bool   // 该分支是否已被系统标记为跳过 (TaskSkipped)
}

// IJoinStrategy 汇聚网关开闸裁决策略接口
// 将不同网关类型的分支汇聚语义通过策略模式解耦，消除臃肿的面向过程分支判断
type IJoinStrategy interface {
	// IsSatisfied 裁决给定的前置分支定态集合是否满足开闸放行条件
	IsSatisfied(branches []BranchState) bool
}

// parallelJoinStrategy 并行汇聚网关策略 (AND)
// 语义：全部分支必须全部到达且定态（通过或跳过），且至少有一条有效分支真实通过
type parallelJoinStrategy struct{}

func (s *parallelJoinStrategy) IsSatisfied(branches []BranchState) bool {
	if len(branches) == 0 {
		return true
	}

	var passedCount int
	for _, b := range branches {
		// 并行网关要求所有分支必须到达
		if !b.HasTask {
			return false
		}
		// 分支必须是已决状态（通过 或 被跳过）
		if !b.IsPassed && !b.IsSkipped {
			return false
		}
		if b.IsPassed {
			passedCount++
		}
	}

	// 安全防卫：若所有分支全部被跳过（0条分支真实通过），则不盲目放行
	return passedCount > 0
}

// inclusiveJoinStrategy 包容汇聚网关策略 (OR)
// 语义：凡是已到达并产生任务的分支必须全部定态，且至少有一条分支真实通过
type inclusiveJoinStrategy struct{}

func (s *inclusiveJoinStrategy) IsSatisfied(branches []BranchState) bool {
	if len(branches) == 0 {
		return true
	}

	var passedCount int
	for _, b := range branches {
		if !b.HasTask {
			// 未到达的分支在包容网关下若尚未生成任务，则阻断等待
			return false
		}
		if !b.IsPassed && !b.IsSkipped {
			return false
		}
		if b.IsPassed {
			passedCount++
		}
	}

	// 至少有一条真实通过的分支
	return passedCount > 0
}

// passThroughJoinStrategy 排他网关/普通流转直通策略 (XOR)
// 语义：首到即走，不执行屏障阻断
type passThroughJoinStrategy struct{}

func (s *passThroughJoinStrategy) IsSatisfied(_ []BranchState) bool {
	return true
}

// GetJoinStrategy 根据网关节点类型获取对应的汇聚策略实例
func GetJoinStrategy(nodeType model.NodeType) IJoinStrategy {
	switch nodeType {
	case model.NodeGatewayParallel:
		return &parallelJoinStrategy{}
	case model.NodeGatewayInclusive:
		return &inclusiveJoinStrategy{}
	default:
		return &passThroughJoinStrategy{}
	}
}
