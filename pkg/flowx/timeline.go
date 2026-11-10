package flowx

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// buildTimelineFromTasks 根据物理任务列表按批次号（BatchCode）与节点聚合展示时间线
func buildTimelineFromTasks(allTasks []*model.Task) []model.TimelineEvent {
	// 按 "NodeID:BatchCode" 维度分组，确保同一节点的多轮驳回重审完全分离
	groups := lo.GroupBy(allTasks, func(t *model.Task) string {
		return fmt.Sprintf("%s:%d", t.NodeID, t.BatchCode)
	})

	events := lo.FilterMap(lo.Values(groups), func(tasks []*model.Task, _ int) (model.TimelineEvent, bool) {
		if len(tasks) == 0 {
			return model.TimelineEvent{}, false
		}
		first := tasks[0]

		// 利用 lo.Reduce 函数式规约提取该节点批次下的最晚发生时间
		occurredAt := lo.Reduce(tasks, func(acc time.Time, t *model.Task, _ int) time.Time {
			if !t.FinishTime.IsZero() && t.FinishTime.After(acc) {
				return t.FinishTime
			}
			return acc
		}, first.CreateTime)

		return model.TimelineEvent{
			NodeID:     first.NodeID,
			NodeName:   first.NodeName,
			BatchCode:  first.BatchCode,
			OccurredAt: occurredAt,
			Tasks:      tasks,
			IsFinished: lo.EveryBy(tasks, func(t *model.Task) bool {
				return t.Status != model.TaskPending
			}),
		}, true
	})

	// 严格按流转批次升序呈现时间线；同批次内按发生时间升序，次级按节点 ID 排序，保证绝对稳定的渲染顺序
	slices.SortFunc(events, func(a, b model.TimelineEvent) int {
		if n := cmp.Compare(a.BatchCode, b.BatchCode); n != 0 {
			return n
		}
		if !a.OccurredAt.Equal(b.OccurredAt) {
			if a.OccurredAt.Before(b.OccurredAt) {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.NodeID, b.NodeID)
	})

	return events
}
