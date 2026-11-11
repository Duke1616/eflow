package memory

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorage_ListPendingTasksByInstances(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()

	// 准备测试数据
	tasks := []*model.Task{
		{
			ID:         "task_1",
			InstanceID: "inst_1",
			NodeID:     "node_a",
			Status:     model.TaskPending,
		},
		{
			ID:         "task_2",
			InstanceID: "inst_1",
			NodeID:     "node_b",
			Status:     model.TaskPassed,
		},
		{
			ID:         "task_3",
			InstanceID: "inst_2",
			NodeID:     "node_c",
			Status:     model.TaskPending,
		},
		{
			ID:         "task_4",
			InstanceID: "inst_3",
			NodeID:     "node_d",
			Status:     model.TaskPending,
		},
	}
	require.NoError(t, s.SaveTasks(ctx, tasks))

	testCases := []struct {
		name        string
		instIDs     []string
		wantCount   int
		wantTaskIDs []string
	}{
		{
			name:      "空实例列表返回空",
			instIDs:   []string{},
			wantCount: 0,
		},
		{
			name:        "命中单个实例且仅返回待办任务",
			instIDs:     []string{"inst_1"},
			wantCount:   1,
			wantTaskIDs: []string{"task_1"},
		},
		{
			name:        "跨多个实例批量拉取待办任务",
			instIDs:     []string{"inst_1", "inst_2"},
			wantCount:   2,
			wantTaskIDs: []string{"task_1", "task_3"},
		},
		{
			name:      "查询不存在的实例",
			instIDs:   []string{"inst_none"},
			wantCount: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := s.ListPendingTasksByInstances(ctx, tc.instIDs)
			require.NoError(t, err)
			assert.Len(t, got, tc.wantCount)
			if len(tc.wantTaskIDs) > 0 {
				gotIDs := make([]string, len(got))
				for i, item := range got {
					gotIDs[i] = item.ID
				}
				assert.ElementsMatch(t, tc.wantTaskIDs, gotIDs)
			}
		})
	}
}

// seedPendingTaskFixture 构造覆盖「多流程定义 / 多版本 / 多处理人 / 已办结 / 实例缺失」的待办数据集
// 任务创建时间严格递增：t1 < t2 < t3 < t4 < t6 < t7，便于断言排序方向
func seedPendingTaskFixture(t *testing.T, s *Storage) {
	t.Helper()
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	for _, def := range []model.ProcessDefinition{
		{Key: "leave", Version: 1, Name: "请假审批"},
		{Key: "leave", Version: 2, Name: "请假审批V2"},
		{Key: "expense", Version: 1, Name: "报销审批"},
	} {
		require.NoError(t, s.SaveDefinition(ctx, def))
	}

	for _, inst := range []model.Instance{
		{ID: "inst_leave", ProcKey: "leave", Version: 1},
		{ID: "inst_leave_v2", ProcKey: "leave", Version: 2},
		{ID: "inst_expense", ProcKey: "expense", Version: 1},
	} {
		require.NoError(t, s.SaveInstance(ctx, &inst))
	}

	require.NoError(t, s.SaveTasks(ctx, []*model.Task{
		{ID: "t1", InstanceID: "inst_leave", Assignee: "zhang", Status: model.TaskPending, CreateTime: base.Add(1 * time.Hour)},
		{ID: "t2", InstanceID: "inst_leave", Assignee: "zhang", Status: model.TaskPending, CreateTime: base.Add(2 * time.Hour)},
		{ID: "t3", InstanceID: "inst_expense", Assignee: "zhang", Status: model.TaskPending, CreateTime: base.Add(3 * time.Hour)},
		{ID: "t4", InstanceID: "inst_leave_v2", Assignee: "zhang", Status: model.TaskPending, CreateTime: base.Add(4 * time.Hour)},
		{ID: "t5", InstanceID: "inst_leave", Assignee: "zhang", Status: model.TaskPassed, CreateTime: base.Add(5 * time.Hour)},
		{ID: "t6", InstanceID: "inst_leave", Assignee: "li", Status: model.TaskPending, CreateTime: base.Add(6 * time.Hour)},
		{ID: "t7", InstanceID: "inst_missing", Assignee: "zhang", Status: model.TaskPending, CreateTime: base.Add(7 * time.Hour)},
	}))
}

func TestStorage_ListPendingTasks(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()
	seedPendingTaskFixture(t, s)

	testCases := []struct {
		name      string
		query     model.PendingTaskQuery
		wantIDs   []string
		wantTotal int64
	}{
		{
			name:      "默认按创建时间降序且过滤已办结任务",
			query:     model.PendingTaskQuery{},
			wantIDs:   []string{"t7", "t6", "t4", "t3", "t2", "t1"},
			wantTotal: 6,
		},
		{
			name:      "升序方向被严格遵守",
			query:     model.PendingTaskQuery{SortByAsc: true},
			wantIDs:   []string{"t1", "t2", "t3", "t4", "t6", "t7"},
			wantTotal: 6,
		},
		{
			name:      "指定处理人只命中本人待办",
			query:     model.PendingTaskQuery{Assignee: "zhang"},
			wantIDs:   []string{"t7", "t4", "t3", "t2", "t1"},
			wantTotal: 5,
		},
		{
			name:      "流程名经实例回溯定义版本精确匹配",
			query:     model.PendingTaskQuery{ProcessName: "请假审批"},
			wantIDs:   []string{"t6", "t2", "t1"},
			wantTotal: 3,
		},
		{
			name:      "同名流程的不同版本按各自定义名区分",
			query:     model.PendingTaskQuery{ProcessName: "请假审批V2"},
			wantIDs:   []string{"t4"},
			wantTotal: 1,
		},
		{
			name:      "流程名与处理人条件叠加",
			query:     model.PendingTaskQuery{Assignee: "zhang", ProcessName: "请假审批"},
			wantIDs:   []string{"t2", "t1"},
			wantTotal: 2,
		},
		{
			name:      "分页切片但总数仍为真实计数",
			query:     model.PendingTaskQuery{Offset: 2, Limit: 2},
			wantIDs:   []string{"t4", "t3"},
			wantTotal: 6,
		},
		{
			name:      "末页不足一页时截断到实际长度",
			query:     model.PendingTaskQuery{Offset: 5, Limit: 10},
			wantIDs:   []string{"t1"},
			wantTotal: 6,
		},
		{
			name:      "offset 越界返回空但保留真实总数",
			query:     model.PendingTaskQuery{Offset: 100, Limit: 10},
			wantIDs:   []string{},
			wantTotal: 6,
		},
		{
			name:      "limit 为负数语义为不限条数",
			query:     model.PendingTaskQuery{Limit: -1},
			wantIDs:   []string{"t7", "t6", "t4", "t3", "t2", "t1"},
			wantTotal: 6,
		},
		{
			name:      "limit 为零语义为不限条数",
			query:     model.PendingTaskQuery{Limit: 0},
			wantIDs:   []string{"t7", "t6", "t4", "t3", "t2", "t1"},
			wantTotal: 6,
		},
		{
			name:      "无匹配流程名返回空",
			query:     model.PendingTaskQuery{ProcessName: "不存在的流程"},
			wantIDs:   []string{},
			wantTotal: 0,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, total, err := s.ListPendingTasks(ctx, tc.query)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total)
			assert.Equal(t, tc.wantIDs, lo.Map(got, func(item *model.Task, _ int) string { return item.ID }))
		})
	}
}

// TestStorage_ListPendingTasks_OrderStableAcrossCalls 底层 map 遍历顺序随机，
// 多轮重复查询必须得到完全一致的排序，否则上层跨引擎归并分页会重复或遗漏
func TestStorage_ListPendingTasks_OrderStableAcrossCalls(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()
	seedPendingTaskFixture(t, s)

	want := []string{"t7", "t6", "t4", "t3", "t2", "t1"}
	for i := 0; i < 20; i++ {
		got, _, err := s.ListPendingTasks(ctx, model.PendingTaskQuery{})
		require.NoError(t, err)
		assert.Equal(t, want, lo.Map(got, func(item *model.Task, _ int) string { return item.ID }))
	}
}
