package kernel

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// ArbitrationResult 节点流转仲裁裁决产物（纯内存计算，保持与持久化解耦）
type ArbitrationResult struct {
	IsSatisfied    bool     // 当前节点是否满足流向下一阶段的条件
	SkippedTaskIDs []string // 或签竞争通过时，需被联动跳过的其他待办任务 ID 列表
}

// ISignArbitrator 审批流转仲裁策略接口（操作与判定针对领域实体指针）
type ISignArbitrator interface {
	// IsSatisfied 判定本批次审批任务是否满足流转推进条件
	IsSatisfied(batchTasks []*model.Task) bool
	// CalculateSkipped 当任务通过时，计算需联动跳过的其他待办任务（如或签）
	CalculateSkipped(currentTask *model.Task, batchTasks []*model.Task) []string
}

// oneVoteArbitrator 或签仲裁器 (一人同意即可通过，联动跳过其他待办)
type oneVoteArbitrator struct{}

func (a *oneVoteArbitrator) IsSatisfied(batchTasks []*model.Task) bool {
	return lo.SomeBy(batchTasks, func(t *model.Task) bool {
		return t.Status == model.TaskPassed
	})
}

func (a *oneVoteArbitrator) CalculateSkipped(currentTask *model.Task, batchTasks []*model.Task) []string {
	if !currentTask.IsPassed() {
		return nil
	}
	return lo.FilterMap(batchTasks, func(t *model.Task, _ int) (string, bool) {
		return t.ID, t.ID != currentTask.ID && t.IsPending()
	})
}

// allPassArbitrator 会签仲裁器 (全员同意才可通过，无联动跳过)
type allPassArbitrator struct{}

func (a *allPassArbitrator) IsSatisfied(batchTasks []*model.Task) bool {
	return lo.EveryBy(batchTasks, func(t *model.Task) bool {
		return t.Status == model.TaskPassed
	})
}

func (a *allPassArbitrator) CalculateSkipped(currentTask *model.Task, batchTasks []*model.Task) []string {
	return nil
}

// defaultArbitrators 预置仲裁策略注册表
var defaultArbitrators = map[model.SignRule]ISignArbitrator{
	model.SignOneVote: &oneVoteArbitrator{},
	model.SignAllPass: &allPassArbitrator{},
}

// getArbitrator 获取指定审批规则对应的仲裁器 (默认降级为会签)
func getArbitrator(rule model.SignRule) ISignArbitrator {
	if arb, ok := defaultArbitrators[rule]; ok {
		return arb
	}
	return defaultArbitrators[model.SignAllPass]
}

// IsNodeSatisfiedToAdvance 裁决当前节点是否已满足流转推进条件
func IsNodeSatisfiedToAdvance(node model.NodeSpec, currentBatchTasks []*model.Task) bool {
	if len(currentBatchTasks) == 0 {
		return false
	}
	return getArbitrator(node.SignRule).IsSatisfied(currentBatchTasks)
}

// ArbitrateNode 纯内存裁决当前节点是否满足推进条件，并计算需联动跳过的任务列表
func ArbitrateNode(node model.NodeSpec, currentTask *model.Task, allTasks []*model.Task) ArbitrationResult {
	batchTasks := lo.Filter(allTasks, func(t *model.Task, _ int) bool {
		return t.NodeID == currentTask.NodeID && t.BatchCode == currentTask.BatchCode
	})

	if len(batchTasks) == 0 {
		return ArbitrationResult{IsSatisfied: false}
	}

	arb := getArbitrator(node.SignRule)
	return ArbitrationResult{
		IsSatisfied:    arb.IsSatisfied(batchTasks),
		SkippedTaskIDs: arb.CalculateSkipped(currentTask, batchTasks),
	}
}
