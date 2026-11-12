package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
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

// TestStorage_InstanceVariablesIsolatedFromCaller 内存实现必须与 MySQL 的 JSON 往返同口径：
// 实例变量不得与调用方共享底层 map，否则「未保存即生效」，事务回滚形同虚设
func TestStorage_InstanceVariablesIsolatedFromCaller(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name string
		load func(*Storage, string) (*model.Instance, error)
	}{
		{"GetInstance", func(s *Storage, id string) (*model.Instance, error) { return s.GetInstance(ctx, id) }},
		{"GetInstanceForUpdate", func(s *Storage, id string) (*model.Instance, error) { return s.GetInstanceForUpdate(ctx, id) }},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStorage()
			inst := &model.Instance{ID: "inst_1", Status: model.InstanceRunning, Variables: model.Variables{"amount": "100"}}
			require.NoError(t, s.SaveInstance(ctx, inst))

			inst.Variables["amount"] = "999"
			stored, err := tc.load(s, "inst_1")
			require.NoError(t, err)
			assert.Equal(t, "100", stored.Variables["amount"], "保存后改调用方不得污染已落库快照")

			stored.Variables["amount"] = "777"
			again, err := tc.load(s, "inst_1")
			require.NoError(t, err)
			assert.Equal(t, "100", again.Variables["amount"], "读出后改返回实体不得污染已落库快照")
		})
	}
}

// TestStorage_UpdateTask_OnlyPendingAllowed 与 MySQL 侧乐观状态防卫同口径：
// 已脱离待办态的任务不得被再次更新，否则并发重复审批在内存实现下会静默成功，单测再也测不出这类回归
func TestStorage_UpdateTask_OnlyPendingAllowed(t *testing.T) {
	ctx := context.Background()
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}

	testCases := []struct {
		name    string
		status  model.TaskStatus
		wantErr error
	}{
		{"待办态允许更新", model.TaskPending, nil},
		{"已通过拒绝更新", model.TaskPassed, storage.ErrTaskAlreadyHandled},
		{"已驳回拒绝更新", model.TaskRejected, storage.ErrTaskAlreadyHandled},
		{"系统通过拒绝更新", model.TaskSystemPassed, storage.ErrTaskAlreadyHandled},
		{"已跳过拒绝更新", model.TaskSkipped, storage.ErrTaskAlreadyHandled},
		{"已取消拒绝更新", model.TaskCancelled, storage.ErrTaskAlreadyHandled},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			s := NewStorage()
			require.NoError(t, s.SaveTasks(ctx, []*model.Task{model.NewTask("t1", "inst_1", node, 1, "alice", tc.status)}))

			err := s.UpdateTask(ctx, model.NewTask("t1", "inst_1", node, 1, "alice", model.TaskPassed))
			got, getErr := s.GetTask(ctx, "t1")
			require.NoError(t, getErr)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Equal(t, tc.status, got.Status, "被拒的更新不得改动已落库状态")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, model.TaskPassed, got.Status)
		})
	}
}

// seedTxRollbackFixture 预置一个运行中实例与一条待办任务，作为事务回滚的对照基线
func seedTxRollbackFixture(t *testing.T, s *Storage) {
	ctx := context.Background()
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}
	require.NoError(t, s.SaveInstance(ctx, &model.Instance{
		ID: "inst_1", Status: model.InstanceRunning, Variables: model.Variables{"amount": "100"},
	}))
	require.NoError(t, s.SaveTasks(ctx, []*model.Task{model.NewTask("t1", "inst_1", node, 1, "alice", model.TaskPending)}))
}

// TestStorage_DoTx_RollbackOnError 事务内的实例改写、任务状态变更、新增任务与新增定义必须整体回滚，
// 与 MySQL 驱动语义一致；否则引擎 DoTx 静默降级为非事务，回滚断言全是空转
func TestStorage_DoTx_RollbackOnError(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()
	seedTxRollbackFixture(t, s)
	node := model.NodeSpec{ID: "n", Name: "审批节点", Type: model.NodeApproval}

	sentinel := errors.New("业务失败")
	err := s.DoTx(ctx, func(tx storage.IStorage) error {
		require.NoError(t, tx.SaveInstance(ctx, &model.Instance{
			ID: "inst_1", Status: model.InstanceFinished, Variables: model.Variables{"amount": "999"},
		}))
		require.NoError(t, tx.UpdateTasksStatus(ctx, []string{"t1"}, model.TaskPassed))
		require.NoError(t, tx.SaveTasks(ctx, []*model.Task{model.NewTask("t2", "inst_1", node, 1, "bob", model.TaskPending)}))
		require.NoError(t, tx.SaveDefinition(ctx, model.ProcessDefinition{Key: "k", Version: 1}))
		return sentinel
	})
	assert.ErrorIs(t, err, sentinel)

	inst, err := s.GetInstance(ctx, "inst_1")
	require.NoError(t, err)
	assert.Equal(t, model.InstanceRunning, inst.Status)
	assert.Equal(t, "100", inst.Variables["amount"])

	task, err := s.GetTask(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, model.TaskPending, task.Status)

	tasks, err := s.ListTasksByInstance(ctx, "inst_1")
	require.NoError(t, err)
	assert.Len(t, tasks, 1, "事务内新增的任务必须一并回滚")

	_, err = s.GetDefinition(ctx, "k", 1)
	assert.ErrorIs(t, err, storage.ErrDefinitionNotFound)
}

// TestStorage_DoTx_CommitOnSuccess 成功事务的写入必须全部留存，不能被快照机制误吞
func TestStorage_DoTx_CommitOnSuccess(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()
	seedTxRollbackFixture(t, s)

	require.NoError(t, s.DoTx(ctx, func(tx storage.IStorage) error {
		require.NoError(t, tx.UpdateTasksStatus(ctx, []string{"t1"}, model.TaskPassed))
		return tx.SaveInstance(ctx, &model.Instance{
			ID: "inst_1", Status: model.InstanceFinished, Variables: model.Variables{"amount": "999"},
		})
	}))

	inst, err := s.GetInstance(ctx, "inst_1")
	require.NoError(t, err)
	assert.Equal(t, model.InstanceFinished, inst.Status)
	assert.Equal(t, "999", inst.Variables["amount"])

	task, err := s.GetTask(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, model.TaskPassed, task.Status)
}

// TestStorage_DoTx_RollbackOnPanic panic 同样必须回滚，且 panic 继续向上冒泡不被吞掉
func TestStorage_DoTx_RollbackOnPanic(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()
	seedTxRollbackFixture(t, s)

	assert.Panics(t, func() {
		_ = s.DoTx(ctx, func(tx storage.IStorage) error {
			require.NoError(t, tx.UpdateTasksStatus(ctx, []string{"t1"}, model.TaskPassed))
			panic("boom")
		})
	})

	task, err := s.GetTask(ctx, "t1")
	require.NoError(t, err)
	assert.Equal(t, model.TaskPending, task.Status)
}

// TestStorage_DoTx_RollbackKeepsTaskIndexAlias 回滚必须复现 SaveTasks 建立的双表指针别名。
// 刻意不用「写后读一致」来断言：UpdateTask 与 UpdateTasksStatus 都会按 ID 重新同步 instTasks，
// 分叉会被下一次写入自愈，那种断言测不出快照是否 faithful——这里直接比对堆形状
func TestStorage_DoTx_RollbackKeepsTaskIndexAlias(t *testing.T) {
	ctx := context.Background()
	s := NewStorage()
	seedTxRollbackFixture(t, s)

	require.Same(t, s.tasks["t1"], s.instTasks["inst_1"][0], "事务前双表本就共享同一指针")

	err := s.DoTx(ctx, func(tx storage.IStorage) error {
		require.NoError(t, tx.UpdateTasksStatus(ctx, []string{"t1"}, model.TaskPassed))
		return errors.New("回滚")
	})
	require.Error(t, err)

	require.Same(t, s.tasks["t1"], s.instTasks["inst_1"][0], "回滚后双表必须仍指向同一副本")
	assert.Equal(t, model.TaskPending, s.tasks["t1"].Status)
}
