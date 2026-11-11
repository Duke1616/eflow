package kernel

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// diamondPlan 构造菱形拓扑：start -> a -> gw -> [b, c] -> end
func diamondPlan() *ExecutionPlan {
	return NewExecutionPlan(model.ProcessDefinition{
		Key:     "walk_diamond",
		Name:    "遍历原语菱形拓扑",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart},
			{ID: "a", Type: model.NodeApproval},
			{ID: "gw", Type: model.NodeGatewayParallel},
			{ID: "b", Type: model.NodeApproval},
			{ID: "c", Type: model.NodeApproval},
			{ID: "end", Type: model.NodeEnd},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "a"},
			{Source: "a", Target: "gw"},
			{Source: "gw", Target: "b"},
			{Source: "gw", Target: "c"},
			{Source: "b", Target: "end"},
			{Source: "c", Target: "end"},
		},
	})
}

func TestExecutionPlan_Walk(t *testing.T) {
	plan := diamondPlan()
	all := func(model.NodeSpec) walkAction { return walkAction{Collect: true, Descend: true} }

	testCases := []struct {
		name    string
		roots   []string
		forward bool
		visit   func(model.NodeSpec) walkAction
		wantIDs []string
	}{
		{
			name:    "顺流广度优先_全收集全下探",
			roots:   []string{"start"},
			forward: true,
			visit:   all,
			wantIDs: []string{"start", "a", "gw", "b", "c", "end"},
		},
		{
			name:    "逆流广度优先_全收集全下探",
			roots:   []string{"end"},
			forward: false,
			visit:   all,
			wantIDs: []string{"end", "b", "c", "gw", "a", "start"},
		},
		{
			name:    "命中即停_Collect 与 Descend 相互独立",
			roots:   []string{"start"},
			forward: true,
			visit: func(n model.NodeSpec) walkAction {
				if n.IsActivity() {
					return walkAction{Collect: true}
				}
				return walkAction{Descend: true}
			},
			wantIDs: []string{"a"},
		},
		{
			name:    "命中后继续穿透_覆盖全部活动节点",
			roots:   []string{"start"},
			forward: true,
			visit: func(n model.NodeSpec) walkAction {
				if n.IsActivity() {
					return walkAction{Collect: true, Descend: true}
				}
				return walkAction{Descend: true}
			},
			wantIDs: []string{"a", "b", "c"},
		},
		{
			name:    "重复根节点与菱形交汇点均只访问一次",
			roots:   []string{"a", "a", "start"},
			forward: true,
			visit:   all,
			wantIDs: []string{"a", "start", "gw", "b", "c", "end"},
		},
		{
			name:    "不存在的根节点被忽略",
			roots:   []string{"ghost", "a"},
			forward: true,
			visit:   all,
			wantIDs: []string{"a", "gw", "b", "c", "end"},
		},
		{
			name:    "空根节点返回空结果",
			roots:   nil,
			forward: true,
			visit:   all,
			wantIDs: []string{},
		},
		{
			name:    "不下探时只访问根节点自身",
			roots:   []string{"start"},
			forward: true,
			visit:   func(model.NodeSpec) walkAction { return walkAction{Collect: true} },
			wantIDs: []string{"start"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := plan.walk(tc.roots, tc.forward, tc.visit)
			assert.Equal(t, tc.wantIDs, lo.Map(got, func(n model.NodeSpec, _ int) string { return n.ID }))
		})
	}
}

// cyclicGatewayPlan 构造网关环路：gw1 -> gw2 -> gw1。
// 静态校验只拦自环、不拦多节点环路，因此这种定义可以被正常部署，
// 遍历原语必须自带 visited，否则逆向递归会栈溢出并击穿整个进程。
func cyclicGatewayPlan(t *testing.T) *ExecutionPlan {
	t.Helper()
	def := model.ProcessDefinition{
		Key:     "walk_cycle",
		Name:    "网关环路",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart},
			{ID: "gw1", Type: model.NodeGatewayParallel},
			{ID: "gw2", Type: model.NodeGatewayParallel},
			{ID: "end", Type: model.NodeEnd},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "gw1"},
			{Source: "gw1", Target: "gw2"},
			{Source: "gw2", Target: "gw1"},
			{Source: "gw2", Target: "end"},
		},
	}
	require.NoError(t, def.Validate(), "环路定义当前可被部署，故遍历必须自身环安全")
	return NewExecutionPlan(def)
}

func TestExecutionPlan_GatewayCycleIsTraversalSafe(t *testing.T) {
	plan := cyclicGatewayPlan(t)

	t.Run("逆向溯源前置任务节点不再栈溢出", func(t *testing.T) {
		assert.Empty(t, plan.FindEffectivePrecedingTaskNodes("gw1"))
		assert.Empty(t, plan.FindEffectivePrecedingTaskNodes("gw2"))
	})

	t.Run("汇聚屏障裁决不再栈溢出", func(t *testing.T) {
		gw1 := plan.NodesMap["gw1"]
		assert.True(t, plan.IsJoinSatisfied(gw1, nil))
	})

	t.Run("驳回目标求解不再栈溢出", func(t *testing.T) {
		targets, err := plan.FindUpstreamRejectNodes("gw2")
		require.NoError(t, err)
		require.Len(t, targets, 1)
		assert.Equal(t, "start", targets[0].ID)
	})

	t.Run("高亮边投影不再栈溢出且出边严格阻断", func(t *testing.T) {
		edges := plan.CalculateTraversedEdges(nil, nil)
		assert.Equal(t, map[string][]string{"start": {"gw1"}}, edges)
	})

	t.Run("原语自身在环路上终止", func(t *testing.T) {
		got := plan.walk([]string{"gw1"}, true, func(model.NodeSpec) walkAction {
			return walkAction{Collect: true, Descend: true}
		})
		assert.ElementsMatch(t, []string{"gw1", "gw2", "end"}, lo.Map(got, func(n model.NodeSpec, _ int) string { return n.ID }))
	})
}
