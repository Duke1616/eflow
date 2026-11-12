package memory

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seedTiedTasks 构造同创建时间的平局任务集。内存实现保留纳秒精度，而 MySQL 落库截断到毫秒，
// 故必须显式对齐 CreateTime 才能复现生产上的同毫秒平局
func seedTiedTasks(t *testing.T, s *Storage, count int, ctime time.Time) {
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}
	// 逆序插入，与 MySQL 侧测试保持同一构造口径：物理顺序不得与期望的平局定序碰巧一致
	tasks := lo.Times(count, func(i int) *model.Task {
		tk := model.NewTask(fmt.Sprintf("id_%02d", count-1-i), "inst", node, 1, "alice", model.TaskPending)
		tk.CreateTime = ctime
		return tk
	})
	require.NoError(t, s.SaveTasks(context.Background(), tasks))
}

func TestListPendingTasks_TieBreakStableAcrossRounds(t *testing.T) {
	testCases := []struct {
		name      string
		sortByAsc bool
		limit     int
	}{
		{name: "倒序_首页5条", sortByAsc: false, limit: 5},
		{name: "正序_首页5条", sortByAsc: true, limit: 5},
		{name: "倒序_首页1条", sortByAsc: false, limit: 1},
		{name: "正序_首页7条", sortByAsc: true, limit: 7},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStorage()
			seedTiedTasks(t, s, 20, time.Now())

			query := model.PendingTaskQuery{Assignee: "alice", SortByAsc: tc.sortByAsc, Limit: tc.limit}
			first, total, err := s.ListPendingTasks(context.Background(), query)
			require.NoError(t, err)
			require.Equal(t, int64(20), total)
			require.Len(t, first, tc.limit)

			// 平局恒以 id 升序定序，与 SortByAsc 无关，口径须与 MySQL 实现及跨引擎归并层逐字一致
			wantIDs := lo.Times(tc.limit, func(i int) string { return fmt.Sprintf("id_%02d", i) })
			assert.Equal(t, wantIDs, lo.Map(first, func(x *model.Task, _ int) string { return x.ID }))

			for round := 0; round < 200; round++ {
				again, _, err := s.ListPendingTasks(context.Background(), query)
				require.NoError(t, err)
				require.Equal(t, first, again, "第 %d 次重复查询结果漂移", round)
			}
		})
	}
}

func TestListPendingTasks_TieBreakPagingNoDupNoMiss(t *testing.T) {
	for _, sortByAsc := range []bool{false, true} {
		t.Run(fmt.Sprintf("sortByAsc=%v", sortByAsc), func(t *testing.T) {
			s := NewStorage()
			seedTiedTasks(t, s, 20, time.Now())

			seen := make([]string, 0, 20)
			for offset := 0; offset < 20; offset += 5 {
				page, _, err := s.ListPendingTasks(context.Background(), model.PendingTaskQuery{
					Assignee: "alice", SortByAsc: sortByAsc, Offset: offset, Limit: 5,
				})
				require.NoError(t, err)
				require.Len(t, page, 5)
				seen = append(seen, lo.Map(page, func(x *model.Task, _ int) string { return x.ID })...)
			}

			assert.Len(t, lo.Uniq(seen), 20, "逐页拼接必须不重不漏覆盖全部 20 条")
			assert.ElementsMatch(t, lo.Times(20, func(i int) string { return fmt.Sprintf("id_%02d", i) }), seen)
		})
	}
}

func TestListPendingTasks_PrimaryOrderStillCreateTime(t *testing.T) {
	s := NewStorage()
	base := time.Now()
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}

	var tasks []*model.Task
	for group := 0; group < 3; group++ {
		for i := 0; i < 3; i++ {
			tk := model.NewTask(fmt.Sprintf("g%d_%02d", group, i), "inst", node, 1, "alice", model.TaskPending)
			tk.CreateTime = base.Add(time.Duration(group) * time.Millisecond)
			tasks = append(tasks, tk)
		}
	}
	require.NoError(t, s.SaveTasks(context.Background(), tasks))

	page, _, err := s.ListPendingTasks(context.Background(), model.PendingTaskQuery{Assignee: "alice", Limit: 9})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"g2_00", "g2_01", "g2_02",
		"g1_00", "g1_01", "g1_02",
		"g0_00", "g0_01", "g0_02",
	}, lo.Map(page, func(x *model.Task, _ int) string { return x.ID }))
}
