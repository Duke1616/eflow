package kernel

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
)

// diamondPrecedingPlan 构造菱形多层网关拓扑：a2 同时归属 gwA 与 gwB 两条分支，用于验证穿透去重
// start -> [a1, a2, a3]；a1,a2 -> gwA；a2,a3 -> gwB；gwA,gwB -> join
func diamondPrecedingPlan(extraGatewayLayer bool) model.ProcessDefinition {
	nodes := []model.NodeSpec{
		{ID: "start", Type: model.NodeStart, Name: "开始"},
		{ID: "a1", Type: model.NodeApproval, Name: "审批A1"},
		{ID: "a2", Type: model.NodeApproval, Name: "审批A2"},
		{ID: "a3", Type: model.NodeApproval, Name: "审批A3"},
		{ID: "gwA", Type: model.NodeGatewayParallel, Name: "网关A"},
		{ID: "gwB", Type: model.NodeGatewayParallel, Name: "网关B"},
		{ID: "join", Type: model.NodeGatewayParallel, Name: "汇聚"},
	}
	edges := []model.EdgeSpec{
		{Source: "start", Target: "a1"},
		{Source: "start", Target: "a2"},
		{Source: "start", Target: "a3"},
		{Source: "a1", Target: "gwA"},
		{Source: "a2", Target: "gwA"},
		{Source: "a2", Target: "gwB"},
		{Source: "a3", Target: "gwB"},
	}

	if extraGatewayLayer {
		nodes = append(nodes, model.NodeSpec{ID: "gwOuter", Type: model.NodeGatewayExclusive, Name: "外层网关"})
		edges = append(edges,
			model.EdgeSpec{Source: "gwA", Target: "gwOuter"},
			model.EdgeSpec{Source: "gwB", Target: "gwOuter"},
			model.EdgeSpec{Source: "gwOuter", Target: "join"},
		)
	} else {
		edges = append(edges,
			model.EdgeSpec{Source: "gwA", Target: "join"},
			model.EdgeSpec{Source: "gwB", Target: "join"},
		)
	}

	return model.ProcessDefinition{Key: "test_diamond_preceding", Name: "菱形前置溯源测试", Version: 1, Nodes: nodes, Edges: edges}
}

func TestExecutionPlan_FindEffectivePrecedingTaskNodes(t *testing.T) {
	testCases := []struct {
		name              string
		extraGatewayLayer bool
		target            string
		wantIDs           []string
	}{
		{
			name:    "单层网关穿透_菱形交汇节点只收集一次",
			target:  "join",
			wantIDs: []string{"a1", "a2", "a3"},
		},
		{
			name:              "多层级联网关穿透_菱形交汇节点只收集一次",
			extraGatewayLayer: true,
			target:            "join",
			wantIDs:           []string{"a1", "a2", "a3"},
		},
		{
			name:    "自网关起溯_只取该网关自身分支",
			target:  "gwA",
			wantIDs: []string{"a1", "a2"},
		},
		{
			name:    "活动节点自身无网关前置_返回空",
			target:  "a1",
			wantIDs: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plan := NewExecutionPlan(diamondPrecedingPlan(tc.extraGatewayLayer))
			got := plan.FindEffectivePrecedingTaskNodes(tc.target)
			gotIDs := lo.Map(got, func(n model.NodeSpec, _ int) string { return n.ID })
			assert.ElementsMatch(t, tc.wantIDs, gotIDs)
			assert.Len(t, gotIDs, len(lo.Uniq(gotIDs)), "同一物理节点被多条分支共享时严禁重复收集，否则汇聚段位会被重复计数")
		})
	}
}

func TestExecutionPlan_IsJoinSatisfied_ThroughNestedGateways(t *testing.T) {
	// join 的两条入边均来自网关而非物理节点，屏障判定必须先穿透网关溯源到 a1/a2/a3
	plan := NewExecutionPlan(diamondPrecedingPlan(false))
	join := plan.NodesMap["join"]

	testCases := []struct {
		name     string
		statuses map[string]model.TaskStatus
		want     bool
	}{
		{
			name:     "全部前置通过_放行",
			statuses: map[string]model.TaskStatus{"a1": model.TaskPassed, "a2": model.TaskPassed, "a3": model.TaskPassed},
			want:     true,
		},
		{
			name:     "共享分支a2未通过_两条网关分支同时阻断",
			statuses: map[string]model.TaskStatus{"a1": model.TaskPassed, "a2": model.TaskPending, "a3": model.TaskPassed},
			want:     false,
		},
		{
			name:     "仅gwB侧a3未通过_阻断",
			statuses: map[string]model.TaskStatus{"a1": model.TaskPassed, "a2": model.TaskPassed, "a3": model.TaskPending},
			want:     false,
		},
		{
			name:     "a3条件未命中被跳过_其余通过即放行",
			statuses: map[string]model.TaskStatus{"a1": model.TaskPassed, "a2": model.TaskPassed, "a3": model.TaskSkipped},
			want:     true,
		},
		{
			name:     "a1从未生成任务_视为未到达分支阻断",
			statuses: map[string]model.TaskStatus{"a2": model.TaskPassed, "a3": model.TaskPassed},
			want:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tasks := lo.MapToSlice(tc.statuses, func(nodeID string, status model.TaskStatus) *model.Task {
				return &model.Task{ID: "task_" + nodeID, NodeID: nodeID, Status: status, BatchCode: 1}
			})
			assert.Equal(t, tc.want, plan.IsJoinSatisfied(join, tasks))
		})
	}
}

func TestIsJoinSatisfied(t *testing.T) {
	testCases := []struct {
		name      string
		branches  []BranchState
		wantAllow bool
	}{
		{
			name:      "空分支集合_放行",
			branches:  nil,
			wantAllow: true,
		},
		{
			name: "全部分支通过_放行",
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsPassed: true},
			},
			wantAllow: true,
		},
		{
			name: "存在未到达分支_阻断",
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: false},
			},
			wantAllow: false,
		},
		{
			name: "存在待办未通过分支_阻断",
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsPassed: false, IsSkipped: false},
			},
			wantAllow: false,
		},
		{
			name: "分支通过且另一分支被跳过_放行",
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsSkipped: true},
			},
			wantAllow: true,
		},
		{
			name: "全部分支被跳过_防卫阻断",
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsSkipped: true},
				{NodeID: "b", HasTask: true, IsSkipped: true},
			},
			wantAllow: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantAllow, isJoinSatisfied(tc.branches))
		})
	}
}

// TestExecutionPlan_IsJoinSatisfied_GatewayTypeGuard 只有并行/包容网关才设汇聚屏障，排他网关首到即走。
// 这道守卫原先由 GetJoinStrategy 的 default 分支间接覆盖，策略收敛后它落在 IsJoinSatisfied 自身，
// 必须在此保住覆盖面——用同一拓扑只换网关类型做差分，避免断言空转
func TestExecutionPlan_IsJoinSatisfied_GatewayTypeGuard(t *testing.T) {
	testCases := []struct {
		name string
		typ  model.NodeType
		want bool
	}{
		{name: "排他网关_分支未齐也放行", typ: model.NodeGatewayExclusive, want: true},
		{name: "并行网关_分支未齐必须阻断", typ: model.NodeGatewayParallel, want: false},
		{name: "包容网关_分支未齐必须阻断", typ: model.NodeGatewayInclusive, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			plan := NewExecutionPlan(model.ProcessDefinition{
				Key: "gw_guard", Name: "网关类型守卫", Version: 1,
				Nodes: []model.NodeSpec{
					{ID: "start", Type: model.NodeStart, Name: "开始"},
					{ID: "a1", Type: model.NodeApproval, Name: "审批1"},
					{ID: "a2", Type: model.NodeApproval, Name: "审批2"},
					{ID: "gw", Type: tc.typ, Name: "网关"},
				},
				Edges: []model.EdgeSpec{
					{Source: "start", Target: "a1"},
					{Source: "start", Target: "a2"},
					{Source: "a1", Target: "gw"},
					{Source: "a2", Target: "gw"},
				},
			})
			tasks := []*model.Task{
				{ID: "t1", NodeID: "a1", Status: model.TaskPassed, BatchCode: 1},
				{ID: "t2", NodeID: "a2", Status: model.TaskPending, BatchCode: 1},
			}
			assert.Equal(t, tc.want, plan.IsJoinSatisfied(plan.NodesMap["gw"], tasks))
		})
	}
}
