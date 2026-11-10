package memory

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
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
