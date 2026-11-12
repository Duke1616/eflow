package mysql

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/glebarez/sqlite"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// newPaginationTestDB 用纯 Go SQLite 承载真实 SQL 执行，验证同 ctime 平局下的分页定序。
// ctime 落库为毫秒 (toTaskPO 走 UnixMilli)，同批派发的会签任务天然同值，平局是生产常态而非边界构造
func newPaginationTestDB(t *testing.T) *Storage {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&FlowDefinitionPO{}, &FlowInstancePO{}, &FlowTaskPO{}))
	return NewStorage(db)
}

func seedTiedTasks(t *testing.T, s *Storage, count int, ctime time.Time) {
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}
	// 逆序插入：让物理存储顺序与期望的 id 升序平局口径相反，
	// 否则「按插入序返回平局」的数据库会碰巧给出正确答案，测不出缺失的次级排序
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
			s := newPaginationTestDB(t)
			seedTiedTasks(t, s, 20, time.Now())

			query := model.PendingTaskQuery{Assignee: "alice", SortByAsc: tc.sortByAsc, Limit: tc.limit}
			first, total, err := s.ListPendingTasks(context.Background(), query)
			require.NoError(t, err)
			require.Equal(t, int64(20), total)
			require.Len(t, first, tc.limit)

			// 平局必须以 id 升序定序，且与 SortByAsc 无关：跨引擎归并层的 tiebreak 恒为 TaskID 升序，
			// 引擎层若随方向翻转，前 K 条截断口径就与归并口径不一致，K 边界上的任务会被挤掉
			wantIDs := lo.Times(tc.limit, func(i int) string { return fmt.Sprintf("id_%02d", i) })
			assert.Equal(t, wantIDs, lo.Map(first, func(x *model.Task, _ int) string { return x.ID }))

			for round := 0; round < 50; round++ {
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
			s := newPaginationTestDB(t)
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
	s := newPaginationTestDB(t)
	base := time.Now()
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}

	// 三组不同毫秒，每组内部平局；组间必须仍按 ctime 定序，次级排序不得喧宾夺主
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
