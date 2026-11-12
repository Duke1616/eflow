package kernel

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
)

// TestCalculateTraversedEdges_ParallelJoinWaiting 测试并行汇聚网关在等待单分支完成时的高亮表现
func TestCalculateTraversedEdges_ParallelJoinWaiting(t *testing.T) {
	// 拓扑:
	// start -> fork (gateway_parallel) -> [user1, user2] -> join (gateway_parallel) -> auto (node_task) -> end
	def := model.ProcessDefinition{
		Key:     "test_parallel_join",
		Name:    "并行汇聚流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "fork", Type: model.NodeGatewayParallel, Name: "并行分流"},
			{ID: "user1", Type: model.NodeApproval, Name: "产品审批"},
			{ID: "user2", Type: model.NodeApproval, Name: "领导审批"},
			{ID: "join", Type: model.NodeGatewayParallel, Name: "并行汇聚"},
			{ID: "auto", Type: model.NodeTask, Name: "自动发版"},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "fork"},
			{Source: "fork", Target: "user1"},
			{Source: "fork", Target: "user2"},
			{Source: "user1", Target: "join"},
			{Source: "user2", Target: "join"},
			{Source: "join", Target: "auto"},
			{Source: "auto", Target: "end"},
		},
	}

	plan := NewExecutionPlan(def)

	// 场景 1：流程刚启动，user1 和 user2 都在 pending，join 网关下游尚未生成
	tasksInitial := []*model.Task{
		{NodeID: "user1", Status: model.TaskPending},
		{NodeID: "user2", Status: model.TaskPending},
	}
	edges1 := plan.CalculateTraversedEdges(tasksInitial, nil)
	assert.Contains(t, edges1["start"], "fork")
	assert.Contains(t, edges1["fork"], "user1")
	assert.Contains(t, edges1["fork"], "user2")
	// join 网关前后的边均不应点亮
	assert.NotContains(t, edges1["user1"], "join")
	assert.NotContains(t, edges1["user2"], "join")
	assert.NotContains(t, edges1["join"], "auto")

	// 场景 2：user1 审批通过 (PASSED)，user2 仍在等待 (PENDING)
	tasksUser1Passed := []*model.Task{
		{NodeID: "user1", Status: model.TaskPassed},
		{NodeID: "user2", Status: model.TaskPending},
	}
	edges2 := plan.CalculateTraversedEdges(tasksUser1Passed, nil)
	// user1 到 join 网关的线条应该被点亮
	assert.Contains(t, edges2["user1"], "join", "用户1已通过，线条必须走到汇聚网关")
	// user2 到 join 网关的线条不应点亮
	assert.NotContains(t, edges2["user2"], "join", "用户2未通过，线条不应点亮")
	// join 网关处于等待态，下游线条严格不向后流出
	assert.NotContains(t, edges2["join"], "auto", "网关还在等待，下游线条绝对不能点亮")

	// 场景 3：user1 和 user2 全部通过，join 满足条件，auto 节点被激活
	tasksAllPassed := []*model.Task{
		{NodeID: "user1", Status: model.TaskPassed},
		{NodeID: "user2", Status: model.TaskPassed},
		{NodeID: "auto", Status: model.TaskPending},
	}
	edges3 := plan.CalculateTraversedEdges(tasksAllPassed, nil)
	assert.Contains(t, edges3["user1"], "join")
	assert.Contains(t, edges3["user2"], "join")
	assert.Contains(t, edges3["join"], "auto", "网关汇聚放行后，下游线条应当点亮")
}

// TestCalculateTraversedEdges_SkippedBranch 验证包容网关跳过分支的有向边能被正确纳入轨迹以便前端渲染为虚线
func TestCalculateTraversedEdges_SkippedBranch(t *testing.T) {
	// 拓扑: start -> fork (inclusive) -> [backend, frontend(skipped)] -> join (inclusive) -> end
	def := model.ProcessDefinition{
		Key:     "test_inclusive_skip",
		Name:    "包容网关分支跳过流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "fork", Type: model.NodeGatewayInclusive, Name: "包容网关分流"},
			{ID: "backend", Type: model.NodeApproval, Name: "后端审批"},
			{ID: "frontend", Type: model.NodeApproval, Name: "前端审批"},
			{ID: "join", Type: model.NodeGatewayInclusive, Name: "包容网关汇聚"},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "fork"},
			{Source: "fork", Target: "backend"},
			{Source: "fork", Target: "frontend"},
			{Source: "backend", Target: "join"},
			{Source: "frontend", Target: "join"},
			{Source: "join", Target: "end"},
		},
	}

	plan := NewExecutionPlan(def)

	// 场景：backend 在审批中，frontend 条件未命中被系统跳过 (TaskSkipped)
	tasks := []*model.Task{
		{NodeID: "backend", Status: model.TaskPending},
		{NodeID: "frontend", Status: model.TaskSkipped},
	}

	edges := plan.CalculateTraversedEdges(tasks, nil)

	// 1. 开始到分叉网关，分叉网关到两个分支均已流经
	assert.Contains(t, edges["start"], "fork")
	assert.Contains(t, edges["fork"], "backend")
	assert.Contains(t, edges["fork"], "frontend", "跳过分支的入边必须被收集，供虚线渲染")

	// 2. 跳过分支的自身出边流向汇聚网关
	assert.Contains(t, edges["frontend"], "join", "跳过分支已办结，指向汇聚网关的线条必须被收集")

	// 3. backend 尚未通过，join 网关未放行，下游 end 不得点亮
	assert.NotContains(t, edges["join"], "end")
}

// TestCalculateTraversedEdges_RejectBackToStart 验证工单被驳回至起点修改重提时，下游已作废的历史连线全部熄灭
func TestCalculateTraversedEdges_RejectBackToStart(t *testing.T) {
	def := model.ProcessDefinition{
		Key:     "test_reject_back_to_start",
		Name:    "驳回回退起点流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "cc", Type: model.NodeCarbonCopy, Name: "抄送提交人"},
			{ID: "fork", Type: model.NodeGatewayInclusive, Name: "分流网关"},
			{ID: "backend", Type: model.NodeApproval, Name: "后端"},
			{ID: "algo", Type: model.NodeApproval, Name: "算法"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "cc"},
			{Source: "cc", Target: "fork"},
			{Source: "fork", Target: "backend"},
			{Source: "fork", Target: "algo"},
		},
	}
	plan := NewExecutionPlan(def)

	// 历史批次 0 曾流转至后端，并在后端驳回至起点（批次 1，start 生成 Pending 待办任务）
	tasks := []*model.Task{
		{NodeID: "cc", Status: model.TaskPassed, BatchCode: 0},
		{NodeID: "backend", Status: model.TaskRejected, BatchCode: 0},
		{NodeID: "algo", Status: model.TaskCancelled, BatchCode: 0},
		{NodeID: "start", Status: model.TaskPending, BatchCode: 1}, // 当前处于提单人重报待办中
	}

	edges := plan.CalculateTraversedEdges(tasks, nil)

	// 开始节点未提交，任何向下流转的边均不得点亮
	assert.Empty(t, edges["start"], "退回至起点待办时，开始节点出边不得点亮")
	assert.Empty(t, edges["cc"], "历史抄送人出边不得点亮")
	assert.Empty(t, edges["fork"], "历史分流网关出边绝对不得点亮至后端或算法")
}

// TestCalculateTraversedEdges_MultilevelReject 验证跨级驳回场景下，流转精确停留在回退目标节点，后续作废边严格阻断
func TestCalculateTraversedEdges_MultilevelReject(t *testing.T) {
	def := model.ProcessDefinition{
		Key:     "test_multilevel_reject",
		Name:    "多级审批驳回流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "audit_a", Type: model.NodeApproval, Name: "节点A"},
			{ID: "audit_b", Type: model.NodeApproval, Name: "节点B"},
			{ID: "audit_c", Type: model.NodeApproval, Name: "节点C"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "audit_a"},
			{Source: "audit_a", Target: "audit_b"},
			{Source: "audit_b", Target: "audit_c"},
		},
	}
	plan := NewExecutionPlan(def)

	// 批次 0 中 A 与 B 通过，在 C 处驳回至 A（批次 1 中 A 生成新的 Pending 任务）
	tasks := []*model.Task{
		{NodeID: "audit_a", Status: model.TaskPassed, BatchCode: 0},
		{NodeID: "audit_b", Status: model.TaskPassed, BatchCode: 0},
		{NodeID: "audit_c", Status: model.TaskRejected, BatchCode: 0},
		{NodeID: "audit_a", Status: model.TaskPending, BatchCode: 1}, // A 回退后重新待办
	}

	edges := plan.CalculateTraversedEdges(tasks, nil)

	assert.Contains(t, edges["start"], "audit_a", "Start 到 A 节点的连线正常流经点亮")
	assert.NotContains(t, edges["audit_a"], "audit_b", "A 节点等待重审，出边必须严格阻断")
	assert.NotContains(t, edges["audit_b"], "audit_c", "历史作废的 B 到 C 连线绝对不得点亮")
}

// TestCalculateTraversedEdges_RejectRetreatBlocksJoinOutflow 驳回回退后汇聚网关出边必须熄灭。
// 汇聚屏障一旦不再满足，"上游有已完成分支 + 下游有已激活节点"这两个存在性判定仍会被历史状态喂通，
// 曾导致流程已退回上游、画布上却仍把下游整段点亮
func TestCalculateTraversedEdges_RejectRetreatBlocksJoinOutflow(t *testing.T) {
	// 拓扑: start -> fork(并行) -> [a, b] -> join(并行) -> c -> end
	def := model.ProcessDefinition{
		Key:     "test_retreat_parallel",
		Name:    "并行汇聚驳回回退流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "fork", Type: model.NodeGatewayParallel, Name: "并行分流"},
			{ID: "a", Type: model.NodeApproval, Name: "A"},
			{ID: "b", Type: model.NodeApproval, Name: "B"},
			{ID: "join", Type: model.NodeGatewayParallel, Name: "并行汇聚"},
			{ID: "c", Type: model.NodeApproval, Name: "C"},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "fork"},
			{Source: "fork", Target: "a"},
			{Source: "fork", Target: "b"},
			{Source: "a", Target: "join"},
			{Source: "b", Target: "join"},
			{Source: "join", Target: "c"},
			{Source: "c", Target: "end"},
		},
	}
	plan := NewExecutionPlan(def)

	testCases := []struct {
		name        string
		tasks       []*model.Task
		wantJoinOut bool
		wantBToJoin bool
	}{
		{
			name: "驳回前双分支通过，汇聚已开闸",
			tasks: []*model.Task{
				{NodeID: "a", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "b", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "c", Status: model.TaskPending, BatchCode: 0},
			},
			wantJoinOut: true,
			wantBToJoin: true,
		},
		{
			name: "C 定向驳回仅回 A：B 停留旧批次通过，不得据此喂通汇聚",
			tasks: []*model.Task{
				{NodeID: "a", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "b", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "c", Status: model.TaskRejected, BatchCode: 0},
				{NodeID: "a", Status: model.TaskPending, BatchCode: 1},
			},
			wantJoinOut: false,
			wantBToJoin: true, // B 已通过，入边仍须亮到网关入口（等待态）
		},
		{
			name: "驳回回 C 自身：汇聚上游未回退，出边必须保持点亮",
			tasks: []*model.Task{
				{NodeID: "a", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "b", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "c", Status: model.TaskRejected, BatchCode: 0},
				{NodeID: "c", Status: model.TaskPending, BatchCode: 1},
			},
			wantJoinOut: true,
			wantBToJoin: true,
		},
		{
			name: "A 重审通过后再次汇聚放行",
			tasks: []*model.Task{
				{NodeID: "a", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "b", Status: model.TaskPassed, BatchCode: 0},
				{NodeID: "c", Status: model.TaskRejected, BatchCode: 0},
				{NodeID: "a", Status: model.TaskPassed, BatchCode: 1},
				{NodeID: "c", Status: model.TaskPending, BatchCode: 1},
			},
			wantJoinOut: true,
			wantBToJoin: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			edges := plan.CalculateTraversedEdges(tc.tasks, nil)
			if tc.wantJoinOut {
				assert.Contains(t, edges["join"], "c", "汇聚真实开闸，出边应点亮")
			} else {
				assert.NotContains(t, edges["join"], "c", "流程已退回上游，汇聚出边绝对不得点亮")
			}
			if tc.wantBToJoin {
				assert.Contains(t, edges["b"], "join")
			} else {
				assert.NotContains(t, edges["b"], "join")
			}
			assert.Contains(t, edges["start"], "fork")
			assert.Contains(t, edges["fork"], "a")
			assert.Contains(t, edges["fork"], "b")
		})
	}
}

// TestCalculateTraversedEdges_SkippedBranchCannotFeedJoin 跳过分支只承载虚线投影，不得成为穿透汇聚网关的通路。
// 复现画布缺陷：后端/算法已被驳回重审（入边黑线），汇聚网关却因前端 Skip 分支被判定为已穿透，
// 继续把下游并行分流与领导/产品整段点亮，形成"出边亮、入边黑"的不可能拓扑
func TestCalculateTraversedEdges_SkippedBranchCannotFeedJoin(t *testing.T) {
	// 拓扑: start -> cc -> fork(包容) -> [backend, algo, frontend(Skip)] -> join(包容)
	//       -> fork2(并行) -> [leader, product] -> join2(并行) -> end
	def := model.ProcessDefinition{
		Key:     "test_skip_cannot_feed_join",
		Name:    "跳过分支驳回回退流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "cc", Type: model.NodeCarbonCopy, Name: "抄送提交人"},
			{ID: "fork", Type: model.NodeGatewayInclusive, Name: "包容分流"},
			{ID: "backend", Type: model.NodeApproval, Name: "后端"},
			{ID: "algo", Type: model.NodeApproval, Name: "算法"},
			{ID: "frontend", Type: model.NodeApproval, Name: "前端"},
			{ID: "join", Type: model.NodeGatewayInclusive, Name: "包容汇聚"},
			{ID: "fork2", Type: model.NodeGatewayParallel, Name: "并行分流"},
			{ID: "leader", Type: model.NodeApproval, Name: "领导"},
			{ID: "product", Type: model.NodeApproval, Name: "产品"},
			{ID: "join2", Type: model.NodeGatewayParallel, Name: "并行汇聚"},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "cc"},
			{Source: "cc", Target: "fork"},
			{Source: "fork", Target: "backend"},
			{Source: "fork", Target: "algo"},
			{Source: "fork", Target: "frontend"},
			{Source: "backend", Target: "join"},
			{Source: "algo", Target: "join"},
			{Source: "frontend", Target: "join"},
			{Source: "join", Target: "fork2"},
			{Source: "fork2", Target: "leader"},
			{Source: "fork2", Target: "product"},
			{Source: "leader", Target: "join2"},
			{Source: "product", Target: "join2"},
			{Source: "join2", Target: "end"},
		},
	}
	plan := NewExecutionPlan(def)

	// 批次 0：后端/算法通过、前端条件未命中落 Skip，流转至领导/产品；领导驳回后二者在批次 1 重新待办
	history := []*model.Task{
		{NodeID: "cc", Status: model.TaskPassed, BatchCode: 0},
		{NodeID: "backend", Status: model.TaskPassed, BatchCode: 0},
		{NodeID: "algo", Status: model.TaskPassed, BatchCode: 0},
		{NodeID: "frontend", Status: model.TaskSkipped, BatchCode: 0},
		{NodeID: "leader", Status: model.TaskRejected, BatchCode: 0},
		{NodeID: "product", Status: model.TaskCancelled, BatchCode: 0},
	}
	retreated := append(append([]*model.Task{}, history...),
		&model.Task{NodeID: "backend", Status: model.TaskPending, BatchCode: 1},
		&model.Task{NodeID: "algo", Status: model.TaskPending, BatchCode: 1},
	)

	testCases := []struct {
		name           string
		tasks          []*model.Task
		wantDownstream bool
	}{
		{name: "驳回前：领导/产品待办，下游正常点亮", tasks: history, wantDownstream: true},
		{name: "领导驳回回后端+算法：下游整段必须熄灭", tasks: retreated, wantDownstream: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			edges := plan.CalculateTraversedEdges(tc.tasks, nil)

			// 起点到分流段与跳过分支虚线在两种状态下都必须保留
			assert.Contains(t, edges["start"], "cc")
			assert.Contains(t, edges["cc"], "fork")
			assert.Contains(t, edges["fork"], "backend")
			assert.Contains(t, edges["fork"], "algo")
			assert.Contains(t, edges["fork"], "frontend")
			assert.Contains(t, edges["frontend"], "join", "跳过分支虚线投影不得丢失")

			if tc.wantDownstream {
				assert.Contains(t, edges["join"], "fork2")
				assert.Contains(t, edges["fork2"], "leader")
				assert.Contains(t, edges["fork2"], "product")
				return
			}
			assert.NotContains(t, edges["join"], "fork2", "汇聚未开闸，出边不得点亮")
			assert.NotContains(t, edges["fork2"], "leader", "已退出的下游分支不得点亮")
			assert.NotContains(t, edges["fork2"], "product", "已退出的下游分支不得点亮")
			assert.NotContains(t, edges["backend"], "join", "后端等待重审，入边保持阻断")
			assert.NotContains(t, edges["algo"], "join", "算法等待重审，入边保持阻断")
		})
	}
}
