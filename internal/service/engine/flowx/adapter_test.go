package flowx

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	flowxMem "github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/samber/lo"
	"go.uber.org/mock/gomock"
)

func TestFlowxAdapter_EndToEndLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. 初始化基于内存仓储的 FlowX 内核
	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	adapter := NewEngine(coreEngine, nil)

	require.Equal(t, domain.EngineTypeFlowx, adapter.EngineType())

	// 2. 部署流程定义
	deployInfo, err := adapter.Deploy(ctx, domain.EngineTypeFlowx, flowxModel.ProcessDefinition{
		Key:  "wf_888",
		Name: "请假审批工作流",
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "manager_audit", Name: "主管审批", Type: flowxModel.NodeApproval, Assignees: []string{"leader_zhang"}},
			{ID: "hr_audit", Name: "HR审批", Type: flowxModel.NodeApproval, Assignees: []string{"hr_li"}},
			{ID: "end", Type: flowxModel.NodeEnd},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "manager_audit"},
			{Source: "manager_audit", Target: "hr_audit"},
			{Source: "hr_audit", Target: "end"},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, deployInfo.Version)

	// 3. 启动流程实例 (ProcessLaunchReq)
	launchRes, err := adapter.Start(ctx, domain.ProcessLaunchReq{
		EngineType:   domain.EngineTypeFlowx,
		TicketId:     1001,
		WorkflowId:   888,
		Version:      1,
		WorkflowName: "请假审批工作流",
		Starter:      "employee_wang",
		Variables: map[string]any{
			"days": 3,
		},
	})
	require.NoError(t, err)
	assert.Equal(t, domain.EngineTypeFlowx, launchRes.EngineType)
	assert.NotEmpty(t, launchRes.InstanceID.String())

	// 4. 查询待办任务 (ListTodoTasks)
	todoTasks, total, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{
		Username: "leader_zhang",
		Offset:   0,
		Limit:    10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, todoTasks, 1)

	// 4.1 管理员查询全量待办任务（不限定审批人）
	allTodoTasks, allTotal, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{
		IsAdmin: true,
		Offset:  0,
		Limit:   10,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), allTotal)
	require.Len(t, allTodoTasks, 1)

	taskID := todoTasks[0].TaskID
	assert.Equal(t, "主管审批", todoTasks[0].NodeName)
	assert.Equal(t, "leader_zhang", todoTasks[0].ApprovedBy)

	// 5. 获取任务上下文 (GetTaskContext)
	taskCtx, err := adapter.GetTaskContext(ctx, domain.EngineTypeFlowx, taskID)
	require.NoError(t, err)
	assert.Equal(t, domain.EngineTypeFlowx, taskCtx.EngineType)
	assert.Equal(t, 888, taskCtx.ProcessID)
	assert.Equal(t, "manager_audit", taskCtx.NodeID)
	assert.Equal(t, "主管审批", taskCtx.NodeName)
	assert.Equal(t, "leader_zhang", taskCtx.Assignee)
	assert.False(t, taskCtx.IsFinished)

	// 6. 审批通过第一节点 (Pass)
	err = adapter.Pass(ctx, domain.TaskPassReq{
		EngineType: domain.EngineTypeFlowx,
		TaskId:     taskID,
		Operator:   "leader_zhang",
		Comment:    "同意请假",
		Variables:  map[string]any{"leader_comment": "同意"},
	})
	require.NoError(t, err)

	// 7. 查询流转记录 (GetTaskRecords)
	records, recordTotal, err := adapter.GetTaskRecords(ctx, domain.EngineTypeFlowx, launchRes.InstanceID, 0, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, recordTotal, int64(1))
	require.NotEmpty(t, records)
	assert.Equal(t, "主管审批", records[0].NodeName)
	assert.Equal(t, "leader_zhang", records[0].ApprovedBy)
	assert.Equal(t, "同意请假", records[0].Comment)

	// 8. 查询时间线事件 (GetTaskTimeline)
	timelineGroups, groupTotal, err := adapter.GetTaskTimeline(ctx, domain.EngineTypeFlowx, launchRes.InstanceID, 0, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, groupTotal, int64(1))
	require.NotEmpty(t, timelineGroups)

	// 8.1 运行态点亮视图 (GetInstanceExecutionView)
	execView, err := adapter.GetInstanceExecutionView(ctx, domain.EngineTypeFlowx, launchRes.InstanceID, 1)
	require.NoError(t, err)
	assert.Equal(t, 888, execView.ProcessID)
	assert.Equal(t, 1, execView.ProcessVersion)
	assert.Contains(t, execView.NodeStatusMap, "manager_audit")
	assert.Equal(t, 1, execView.NodeStatusMap["manager_audit"]) // 主管审批通过
	assert.Contains(t, execView.NodeStatusMap, "hr_audit")
	assert.Equal(t, 0, execView.NodeStatusMap["hr_audit"]) // HR 待办中
	assert.Contains(t, execView.PendingUsers, "hr_li")

	// 验证连线点亮：start -> manager 与 manager -> hr 应当高亮贯通，hr -> end 绝不能提前点亮
	assert.Contains(t, execView.TraversedEdges["start"], "manager_audit")
	assert.Contains(t, execView.TraversedEdges["manager_audit"], "hr_audit")
	assert.NotContains(t, execView.TraversedEdges, "hr_audit")

	// 9. 提单人撤销/撤单流程 (Revoke)
	err = adapter.Revoke(ctx, domain.RevokeReq{
		EngineType: domain.EngineTypeFlowx,
		InstanceID: launchRes.InstanceID,
		Force:      true,
		Operator:   "employee_wang",
	})
	require.NoError(t, err)

	// 验证撤销后流程状态
	inst, err := coreEngine.GetInstance(ctx, launchRes.InstanceID.String())
	require.NoError(t, err)
	assert.True(t, inst.IsCancelled())
}

func TestCalculateFlowxTraversedEdges_ComplexTopology(t *testing.T) {
	// 拓扑结构：
	// start -> submitter -> split_gw -> [backend(skip), frontend(passed), algo(pending)]
	// backend, frontend, algo -> join_gw -> split_gw2 -> [product(pending), leader(pending)]
	def := flowxModel.ProcessDefinition{
		Key:     "complex_proc",
		Version: 1,
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "submitter", Type: flowxModel.NodeApproval},
			{ID: "split_gw", Type: flowxModel.NodeGatewayInclusive},
			{ID: "backend", Type: flowxModel.NodeApproval},
			{ID: "frontend", Type: flowxModel.NodeApproval},
			{ID: "algo", Type: flowxModel.NodeApproval},
			{ID: "join_gw", Type: flowxModel.NodeGatewayParallel},
			{ID: "split_gw2", Type: flowxModel.NodeGatewayParallel},
			{ID: "product", Type: flowxModel.NodeApproval},
			{ID: "leader", Type: flowxModel.NodeApproval},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "submitter"},
			{Source: "submitter", Target: "split_gw"},
			{Source: "split_gw", Target: "backend"},
			{Source: "split_gw", Target: "frontend"},
			{Source: "split_gw", Target: "algo"},
			{Source: "backend", Target: "join_gw"},
			{Source: "frontend", Target: "join_gw"},
			{Source: "algo", Target: "join_gw"},
			{Source: "join_gw", Target: "split_gw2"},
			{Source: "split_gw2", Target: "product"},
			{Source: "split_gw2", Target: "leader"},
		},
	}

	// 场景 1：提交人通过，后端跳过，前端通过，算法仍在待办，汇聚网关下游尚未激活
	tasksCase1 := []*flowxModel.Task{
		{NodeID: "submitter", Status: flowxModel.TaskPassed},
		{NodeID: "backend", Status: flowxModel.TaskSkipped},
		{NodeID: "frontend", Status: flowxModel.TaskPassed},
		{NodeID: "algo", Status: flowxModel.TaskPending},
	}

	plan := kernel.NewExecutionPlan(def)
	edges1 := plan.CalculateTraversedEdges(tasksCase1, nil)

	// 上游与到达待办分支正常点亮
	assert.Contains(t, edges1["start"], "submitter")
	assert.Contains(t, edges1["submitter"], "split_gw")
	assert.Contains(t, edges1["split_gw"], "backend")
	assert.Contains(t, edges1["split_gw"], "frontend")
	assert.Contains(t, edges1["split_gw"], "algo")

	// 算法仍在待办，其出边 algo -> join_gw 绝不能点亮
	assert.NotContains(t, edges1, "algo")
	// 汇聚网关下游产品与领导绝不能被提前点亮
	assert.NotContains(t, edges1, "join_gw")
	assert.NotContains(t, edges1, "split_gw2")

	// 场景 2：算法也通过，流程推进到产品和领导待办，双网关连接线正常点亮
	tasksCase2 := []*flowxModel.Task{
		{NodeID: "submitter", Status: flowxModel.TaskPassed},
		{NodeID: "backend", Status: flowxModel.TaskSkipped},
		{NodeID: "frontend", Status: flowxModel.TaskPassed},
		{NodeID: "algo", Status: flowxModel.TaskPassed},
		{NodeID: "product", Status: flowxModel.TaskPending},
		{NodeID: "leader", Status: flowxModel.TaskPending},
	}

	edges2 := plan.CalculateTraversedEdges(tasksCase2, nil)

	// 后端、前端、算法流入汇聚网关全部点亮
	assert.Contains(t, edges2["backend"], "join_gw")
	assert.Contains(t, edges2["frontend"], "join_gw")
	assert.Contains(t, edges2["algo"], "join_gw")

	// 核心断言：连续双网关 join_gw -> split_gw2 正确点亮贯通！
	assert.Contains(t, edges2["join_gw"], "split_gw2")
	assert.Contains(t, edges2["split_gw2"], "product")
	assert.Contains(t, edges2["split_gw2"], "leader")
}

func TestFlowxAdapter_PrebindInstanceBeforeEngineEvents(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	ticketMock := repomocks.NewMockTicketRepository(ctrl)

	adapter := NewEngine(coreEngine, ticketMock)

	_, err := adapter.Deploy(ctx, domain.EngineTypeFlowx, flowxModel.ProcessDefinition{
		Key:  "wf_88",
		Name: "前置绑定测试流程",
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "audit", Name: "审批节点", Type: flowxModel.NodeApproval, Assignees: []string{"bob"}},
			{ID: "end", Type: flowxModel.NodeEnd},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "audit"},
			{Source: "audit", Target: "end"},
		},
	})
	require.NoError(t, err)

	var boundInstanceID domain.ID
	ticketMock.EXPECT().RegisterProcess(gomock.Any(), int64(8888), domain.EngineTypeFlowx, gomock.Any()).
		DoAndReturn(func(_ context.Context, ticketId int64, engineType domain.EngineType, instID domain.ID) error {
			assert.Equal(t, int64(8888), ticketId)
			assert.Equal(t, domain.EngineTypeFlowx, engineType)
			assert.False(t, instID.IsZero())
			boundInstanceID = instID
			return nil
		}).Times(1)

	launchRes, err := adapter.Start(ctx, domain.ProcessLaunchReq{
		EngineType: domain.EngineTypeFlowx,
		TicketId:   8888,
		WorkflowId: 88,
		Starter:    "alice",
	})
	require.NoError(t, err)
	assert.Equal(t, boundInstanceID, launchRes.InstanceID)
}

func TestFlowxEngine_ListPendingSteps_BatchQuery(t *testing.T) {
	ctx := context.Background()
	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	adapter := NewEngine(coreEngine, nil)

	// 部署流程
	_, err := adapter.Deploy(ctx, domain.EngineTypeFlowx, flowxModel.ProcessDefinition{
		Key:  "wf_99",
		Name: "批量拉取测试流程",
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "audit", Name: "财务审核", Type: flowxModel.NodeApproval, Assignees: []string{"cfo"}},
			{ID: "end", Type: flowxModel.NodeEnd},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "audit"},
			{Source: "audit", Target: "end"},
		},
	})
	require.NoError(t, err)

	// 启动两个实例
	instID1, err := coreEngine.Start(ctx, flowx.StartReq{
		Key:     "wf_99",
		Starter: "alice",
	})
	require.NoError(t, err)

	instID2, err := coreEngine.Start(ctx, flowx.StartReq{
		Key:     "wf_99",
		Starter: "bob",
	})
	require.NoError(t, err)

	// 调用 ListPendingSteps 批量拉取
	processes := []domain.Process{
		{
			EngineType: domain.EngineTypeFlowx,
			InstanceID: domain.NewID(instID1),
		},
		{
			EngineType: domain.EngineTypeFlowx,
			InstanceID: domain.NewID(instID2),
		},
	}

	todos, err := adapter.ListPendingSteps(ctx, processes)
	require.NoError(t, err)
	assert.Len(t, todos, 2)

	gotInstIDs := lo.Map(todos, func(item domain.TodoTask, _ int) string {
		return item.InstanceID.String()
	})
	assert.ElementsMatch(t, []string{instID1, instID2}, gotInstIDs)
	for _, todo := range todos {
		assert.Equal(t, domain.EngineTypeFlowx, todo.EngineType)
		assert.Equal(t, "财务审核", todo.NodeName)
		assert.Equal(t, "cfo", todo.ApprovedBy)
	}
}
