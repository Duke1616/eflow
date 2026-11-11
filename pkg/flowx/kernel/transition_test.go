package kernel

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionPlan_FindUpstreamRejectNodes_BackToStart(t *testing.T) {
	// 拓扑: start -> cc (抄送知会) -> gw (选择网关) -> backend (后端审批)
	def := model.ProcessDefinition{
		Key:     "test_reject_to_start",
		Name:    "驳回至开始节点测试",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "cc", Type: model.NodeCarbonCopy, Name: "抄送提交人"},
			{ID: "gw", Type: model.NodeGatewayExclusive, Name: "选择网关"},
			{ID: "backend", Type: model.NodeApproval, Name: "后端审批"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "cc"},
			{Source: "cc", Target: "gw"},
			{Source: "gw", Target: "backend"},
		},
	}

	plan := NewExecutionPlan(def)

	// 后端审批人点击驳回：穿透网关与抄送节点，上游无前置审批人，应正确回溯至开始节点 (NodeStart)
	targets, err := plan.FindUpstreamRejectNodes("backend")
	require.NoError(t, err)
	require.Len(t, targets, 1)
	assert.Equal(t, "start", targets[0].ID)
	assert.Equal(t, model.NodeStart, targets[0].Type)
}

func TestExecutionPlan_FindUpstreamRejectNodes_FilterSkippedBranches(t *testing.T) {
	// 拓扑:
	// start -> fork -> [backend, frontend, algo] -> join -> leader -> end
	def := model.ProcessDefinition{
		Key:     "test_reject_filter_skipped",
		Name:    "驳回排除跳过分支测试",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "fork", Type: model.NodeGatewayInclusive, Name: "条件包容分流"},
			{ID: "backend", Type: model.NodeApproval, Name: "后端"},
			{ID: "frontend", Type: model.NodeApproval, Name: "前端"},
			{ID: "algo", Type: model.NodeApproval, Name: "算法"},
			{ID: "join", Type: model.NodeGatewayParallel, Name: "并行汇聚"},
			{ID: "leader", Type: model.NodeApproval, Name: "领导"},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "fork"},
			{Source: "fork", Target: "backend"},
			{Source: "fork", Target: "frontend"},
			{Source: "fork", Target: "algo"},
			{Source: "backend", Target: "join"},
			{Source: "frontend", Target: "join"},
			{Source: "algo", Target: "join"},
			{Source: "join", Target: "leader"},
			{Source: "leader", Target: "end"},
		},
	}

	plan := NewExecutionPlan(def)

	// 历史任务：后端通过、前端通过、算法因为条件未命中被跳过 (TaskSkipped)
	allTasks := []*model.Task{
		{ID: "t1", NodeID: "backend", BatchCode: 1, Status: model.TaskPassed},
		{ID: "t2", NodeID: "frontend", BatchCode: 1, Status: model.TaskPassed},
		{ID: "t3", NodeID: "algo", BatchCode: 1, Status: model.TaskSkipped},
		{ID: "t4", NodeID: "leader", BatchCode: 1, Status: model.TaskPending},
	}

	// 领导驳回：应精准定位到上一阶段真正执行过审批的节点 [backend, frontend]，自动过滤被跳过的 algo 分支
	targets, err := plan.FindUpstreamRejectNodes("leader", allTasks)
	require.NoError(t, err)
	require.Len(t, targets, 2)

	targetIDs := []string{targets[0].ID, targets[1].ID}
	assert.Contains(t, targetIDs, "backend")
	assert.Contains(t, targetIDs, "frontend")
	assert.NotContains(t, targetIDs, "algo", "被系统跳过的分支绝不能作为驳回目标拉出待办")
}

func TestExecutionPlan_FindAllUpstreamRejectTargets(t *testing.T) {
	// 拓扑: start -> leader (部门主管) -> finance (财务初审) -> cfo (CFO审批)
	def := model.ProcessDefinition{
		Key:     "test_reject_targets",
		Name:    "全量可驳回节点查找测试",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "发起人"},
			{ID: "leader", Type: model.NodeApproval, Name: "部门主管"},
			{ID: "finance", Type: model.NodeApproval, Name: "财务初审"},
			{ID: "cfo", Type: model.NodeApproval, Name: "CFO终审"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "leader"},
			{Source: "leader", Target: "finance"},
			{Source: "finance", Target: "cfo"},
		},
	}

	plan := NewExecutionPlan(def)

	allTasks := []*model.Task{
		{ID: "t1", NodeID: "leader", Status: model.TaskPassed},
		{ID: "t2", NodeID: "finance", Status: model.TaskPassed},
		{ID: "t3", NodeID: "cfo", Status: model.TaskPending},
	}

	// CFO 进行驳回：候选节点应包括起点与已通过的部门主管、财务初审
	targets, err := plan.FindAllUpstreamRejectTargets("cfo", allTasks)
	require.NoError(t, err)
	require.Len(t, targets, 3)

	assert.Equal(t, "start", targets[0].ID)
	assert.Equal(t, model.NodeStart, targets[0].Type)

	assert.Equal(t, "leader", targets[1].ID)
	assert.Equal(t, "finance", targets[2].ID)
}

func TestExecutionPlan_FindAllUpstreamRejectTargets_XORBranch(t *testing.T) {
	// 拓扑: start -> submit -> xor_gw -> [backend, algorithm, frontend] -> join_gw -> leader
	def := model.ProcessDefinition{
		Key:     "test_xor_reject_targets",
		Name:    "XOR网关分支跳过可驳回节点过滤测试",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "发起人"},
			{ID: "submit", Type: model.NodeApproval, Name: "提交确认"},
			{ID: "xor_gw", Type: model.NodeGatewayExclusive, Name: "条件分支"},
			{ID: "backend", Type: model.NodeApproval, Name: "后端审批"},
			{ID: "algorithm", Type: model.NodeApproval, Name: "算法审批"},
			{ID: "frontend", Type: model.NodeApproval, Name: "前端审批"},
			{ID: "join_gw", Type: model.NodeGatewayParallel, Name: "汇聚网关"},
			{ID: "leader", Type: model.NodeApproval, Name: "领导终审"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "submit"},
			{Source: "submit", Target: "xor_gw"},
			{Source: "xor_gw", Target: "backend"},
			{Source: "xor_gw", Target: "algorithm"},
			{Source: "xor_gw", Target: "frontend"},
			{Source: "backend", Target: "join_gw"},
			{Source: "algorithm", Target: "join_gw"},
			{Source: "frontend", Target: "join_gw"},
			{Source: "join_gw", Target: "leader"},
		},
	}

	plan := NewExecutionPlan(def)

	// 实际流转任务快照：
	// 1. submit 已通过
	// 2. backend 命中并审批通过 (TaskPassed)
	// 3. algorithm 条件未满足被系统跳过 (TaskSkipped)
	// 4. frontend 默认不开，压根没有生成任何任务
	// 5. leader 当前处于待办处理中 (TaskPending)
	allTasks := []*model.Task{
		{ID: "t1", NodeID: "submit", Status: model.TaskPassed, BatchCode: 1},
		{ID: "t2", NodeID: "backend", Status: model.TaskPassed, BatchCode: 1},
		{ID: "t3", NodeID: "algorithm", Status: model.TaskSkipped, BatchCode: 1},
		{ID: "t4", NodeID: "leader", Status: model.TaskPending, BatchCode: 1},
	}

	// 1. 测试自由驳回目标 (FindAllUpstreamRejectTargets)
	allTargets, err := plan.FindAllUpstreamRejectTargets("leader", allTasks)
	require.NoError(t, err)
	// 只能包含 start, submit, backend；绝对不能包含 algorithm (被跳过) 与 frontend (无任务未走)
	require.Len(t, allTargets, 3)
	assert.Equal(t, "start", allTargets[0].ID)
	assert.Equal(t, "submit", allTargets[1].ID)
	assert.Equal(t, "backend", allTargets[2].ID)

	// 2. 测试顺序驳回目标 (FindUpstreamRejectNodes)
	upstreamNodes, err := plan.FindUpstreamRejectNodes("leader", allTasks)
	require.NoError(t, err)
	require.Len(t, upstreamNodes, 1)
	assert.Equal(t, "backend", upstreamNodes[0].ID)
}
