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
