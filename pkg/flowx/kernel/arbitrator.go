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

// isBatchSatisfied 按会签规则裁决本批次任务是否满足推进条件：
// 或签一人同意即可，其余规则（含未识别规则的兜底）一律按会签要求全员同意
func isBatchSatisfied(rule model.SignRule, batchTasks []*model.Task) bool {
	passed := func(t *model.Task) bool { return t.Status == model.TaskPassed }
	if rule == model.SignOneVote {
		return lo.SomeBy(batchTasks, passed)
	}
	return lo.EveryBy(batchTasks, passed)
}

// skippedSiblings 或签竞争通过时联动跳过其余待办，会签无联动。
// 准入判定用 IsPassed()（含系统通过），与 isBatchSatisfied 的 == TaskPassed 口径不同，此处原样保留既有行为
func skippedSiblings(rule model.SignRule, currentTask *model.Task, batchTasks []*model.Task) []string {
	if rule != model.SignOneVote || !currentTask.IsPassed() {
		return nil
	}
	return lo.FilterMap(batchTasks, func(t *model.Task, _ int) (string, bool) {
		return t.ID, t.ID != currentTask.ID && t.IsPending()
	})
}

// IsNodeSatisfiedToAdvance 裁决当前节点是否已满足流转推进条件
func IsNodeSatisfiedToAdvance(node model.NodeSpec, currentBatchTasks []*model.Task) bool {
	if len(currentBatchTasks) == 0 {
		return false
	}
	return isBatchSatisfied(node.SignRule, currentBatchTasks)
}

// ArbitrateNode 纯内存裁决当前节点是否满足推进条件，并计算需联动跳过的任务列表
func ArbitrateNode(node model.NodeSpec, currentTask *model.Task, allTasks []*model.Task) ArbitrationResult {
	batchTasks := lo.Filter(allTasks, func(t *model.Task, _ int) bool {
		return t.NodeID == currentTask.NodeID && t.BatchCode == currentTask.BatchCode
	})

	if len(batchTasks) == 0 {
		return ArbitrationResult{IsSatisfied: false}
	}

	return ArbitrationResult{
		IsSatisfied:    isBatchSatisfied(node.SignRule, batchTasks),
		SkippedTaskIDs: skippedSiblings(node.SignRule, currentTask, batchTasks),
	}
}
