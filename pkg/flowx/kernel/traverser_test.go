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
