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
