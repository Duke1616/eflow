package flowx

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	flowxMem "github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	deployInfo, err := adapter.Deploy(ctx, flowxModel.ProcessDefinition{
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
	taskCtx, err := adapter.GetTaskContext(ctx, taskID)
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
	records, recordTotal, err := adapter.GetTaskRecords(ctx, launchRes.InstanceID, 0, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, recordTotal, int64(1))
	require.NotEmpty(t, records)
	assert.Equal(t, "主管审批", records[0].NodeName)
	assert.Equal(t, "leader_zhang", records[0].ApprovedBy)
	assert.Equal(t, "同意请假", records[0].Comment)

	// 8. 查询时间线事件 (GetTaskTimeline)
	timelineGroups, groupTotal, err := adapter.GetTaskTimeline(ctx, launchRes.InstanceID, 0, 10)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, groupTotal, int64(1))
	require.NotEmpty(t, timelineGroups)

	// 8.1 运行态点亮视图 (GetInstanceExecutionView)
	execView, err := adapter.GetInstanceExecutionView(ctx, launchRes.InstanceID)
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

	// 上游与到达待办分支正常点亮；跳过分支纳入流转轨迹以便前端渲染为跳过虚线
	assert.Contains(t, edges1["start"], "submitter")
	assert.Contains(t, edges1["submitter"], "split_gw")
	assert.Contains(t, edges1["split_gw"], "backend", "跳过分支的入边应被收集，供虚线渲染")
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

	// 跳过的后端入网关线被收集（供虚线渲染）；前端、算法流入汇聚网关正常点亮
	assert.Contains(t, edges2["backend"], "join_gw", "跳过分支汇聚连线应被收集，供虚线渲染")
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

	_, err := adapter.Deploy(ctx, flowxModel.ProcessDefinition{
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

// TestFlowxAdapter_RevokePrebindWhenEngineStartFails 校验引擎启动失败时适配层就地精确解绑预分配的实例标识
// 若残留预绑定，工单将永久滞留 PROCESS 且 IsBound 为真，消费者与重启 Job 的重试通道被彻底堵死
func TestFlowxAdapter_RevokePrebindWhenEngineStartFails(t *testing.T) {
	ctx := context.Background()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	ticketMock := repomocks.NewMockTicketRepository(ctrl)
	adapter := NewEngine(coreEngine, ticketMock)

	var boundInstanceID domain.ID
	ticketMock.EXPECT().RegisterProcess(gomock.Any(), int64(9999), domain.EngineTypeFlowx, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int64, _ domain.EngineType, instID domain.ID) error {
			boundInstanceID = instID
			return nil
		}).Times(1)

	ticketMock.EXPECT().RevokeProcessBinding(gomock.Any(), int64(9999), gomock.Any()).
		DoAndReturn(func(_ context.Context, ticketId int64, instID domain.ID) error {
			assert.Equal(t, int64(9999), ticketId)
			assert.Equal(t, boundInstanceID, instID, "必须精确回滚本次预绑定的那个实例标识")
			return nil
		}).Times(1)

	_, err := adapter.Start(ctx, domain.ProcessLaunchReq{
		EngineType: domain.EngineTypeFlowx,
		TicketId:   9999,
		WorkflowId: 777, // 未发布任何版本，必然触发引擎启动失败
		Starter:    "alice",
	})
	require.Error(t, err)
	assert.False(t, boundInstanceID.IsZero(), "预绑定应已真实发生")
}

func TestFlowxEngine_ListPendingSteps_BatchQuery(t *testing.T) {
	ctx := context.Background()
	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	adapter := NewEngine(coreEngine, nil)

	// 部署流程
	_, err := adapter.Deploy(ctx, flowxModel.ProcessDefinition{
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

// TestFlowxAdapter_ListTodoTasks_ProcessNameAndSort 端到端验证 flowx 侧待办检索：
// 流程名筛选必须真实生效（历史实现静默丢弃该条件），排序方向必须透传，total 必须是不受分页影响的真实计数
func TestFlowxAdapter_ListTodoTasks_ProcessNameAndSort(t *testing.T) {
	ctx := context.Background()
	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	adapter := NewEngine(coreEngine, nil)

	for _, def := range []flowxModel.ProcessDefinition{
		{Key: "wf_leave", Name: "请假审批工作流"},
		{Key: "wf_expense", Name: "报销审批工作流"},
	} {
		def.Nodes = []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "audit", Name: "主管审批", Type: flowxModel.NodeApproval, Assignees: []string{"leader_zhang"}},
			{ID: "end", Type: flowxModel.NodeEnd},
		}
		def.Edges = []flowxModel.EdgeSpec{
			{Source: "start", Target: "audit"},
			{Source: "audit", Target: "end"},
		}
		_, err := adapter.Deploy(ctx, def)
		require.NoError(t, err)
	}

	leaveInstIDs := make([]string, 0, 2)
	for i := 0; i < 2; i++ {
		id, err := coreEngine.Start(ctx, flowx.StartReq{Key: "wf_leave", Starter: "employee_wang"})
		require.NoError(t, err)
		leaveInstIDs = append(leaveInstIDs, id)
	}
	expenseInstID, err := coreEngine.Start(ctx, flowx.StartReq{Key: "wf_expense", Starter: "employee_wang"})
	require.NoError(t, err)

	instIDsOf := func(tasks []domain.TodoTask) []string {
		return lo.Map(tasks, func(item domain.TodoTask, _ int) string { return item.InstanceID.String() })
	}

	t.Run("流程名筛选只命中该流程的待办", func(t *testing.T) {
		tasks, total, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{
			Username:    "leader_zhang",
			ProcessName: "请假审批工作流",
			Limit:       10,
		})
		require.NoError(t, err)
		assert.Equal(t, int64(2), total)
		assert.ElementsMatch(t, leaveInstIDs, instIDsOf(tasks))
	})

	t.Run("不带流程名时返回全部待办", func(t *testing.T) {
		tasks, total, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{Username: "leader_zhang", Limit: 10})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.ElementsMatch(t, append(leaveInstIDs, expenseInstID), instIDsOf(tasks))
	})

	t.Run("流程名无匹配时返回空", func(t *testing.T) {
		tasks, total, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{
			Username:    "leader_zhang",
			ProcessName: "不存在的流程",
			Limit:       10,
		})
		require.NoError(t, err)
		assert.Zero(t, total)
		assert.Empty(t, tasks)
	})

	t.Run("分页切片不影响真实总数", func(t *testing.T) {
		tasks, total, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{Username: "leader_zhang", Offset: 2, Limit: 1})
		require.NoError(t, err)
		assert.Equal(t, int64(3), total)
		assert.Len(t, tasks, 1)
	})

	for _, asc := range []bool{false, true} {
		name := "默认按创建时间降序"
		if asc {
			name = "升序方向被透传到底层引擎"
		}
		t.Run(name, func(t *testing.T) {
			tasks, _, err := adapter.ListTodoTasks(ctx, domain.TodoQueryReq{
				Username:  "leader_zhang",
				SortByAsc: asc,
				Limit:     10,
			})
			require.NoError(t, err)
			require.Len(t, tasks, 3)
			times := lo.Map(tasks, func(item domain.TodoTask, _ int) time.Time { return item.CreateTime })
			assert.True(t, sort.SliceIsSorted(times, func(i, j int) bool {
				if asc {
					return times[i].Before(times[j])
				}
				return times[i].After(times[j])
			}))
		})
	}
}

func TestFlowxAdapter_GetActiveReject_Lifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. 初始化 FlowX 内存引擎
	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	adapter := NewEngine(coreEngine, nil)

	// 2. 部署流程：开始 -> 提单人录入 -> 主管审批 -> 结束
	_, err := adapter.Deploy(ctx, flowxModel.ProcessDefinition{
		Key:  "wf_reject_test",
		Name: "驳回闭环测试流程",
		Nodes: []flowxModel.NodeSpec{
			{ID: "start", Type: flowxModel.NodeStart},
			{ID: "submitter", Name: "提单人录入", Type: flowxModel.NodeApproval, Assignees: []string{"alice"}},
			{ID: "auditor", Name: "主管审批", Type: flowxModel.NodeApproval, Assignees: []string{"bob"}},
			{ID: "end", Type: flowxModel.NodeEnd},
		},
		Edges: []flowxModel.EdgeSpec{
			{Source: "start", Target: "submitter"},
			{Source: "submitter", Target: "auditor"},
			{Source: "auditor", Target: "end"},
		},
	})
	require.NoError(t, err)

	// 3. 启动流程
	instID, err := coreEngine.Start(ctx, flowx.StartReq{
		Key:     "wf_reject_test",
		Starter: "alice",
	})
	require.NoError(t, err)
	instDomainID := domain.NewID(instID)

	// 此时未发生驳回，GetActiveReject 必须为 nil
	rejectSnap, err := adapter.GetActiveReject(ctx, instDomainID)
	require.NoError(t, err)
	assert.Nil(t, rejectSnap)

	// 4. 提单人 alice 提交通过
	allTasks, err := coreEngine.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	submitTask, found := lo.Find(allTasks, func(t *flowxModel.Task) bool {
		return t.NodeID == "submitter" && t.Status == flowxModel.TaskPending
	})
	require.True(t, found)

	err = adapter.Pass(ctx, domain.TaskPassReq{
		EngineType: domain.EngineTypeFlowx,
		TaskId:     domain.NewID(submitTask.ID),
		Operator:   "alice",
		Comment:    "提交申请",
	})
	require.NoError(t, err)

	// 5. 主管 bob 驳回回到提单人环节
	allTasks, err = coreEngine.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	auditTask, found := lo.Find(allTasks, func(t *flowxModel.Task) bool {
		return t.NodeID == "auditor" && t.Status == flowxModel.TaskPending
	})
	require.True(t, found)

	err = adapter.Reject(ctx, domain.TaskRejectReq{
		EngineType:   domain.EngineTypeFlowx,
		TaskId:       domain.NewID(auditTask.ID),
		Operator:     "bob",
		Comment:      "资料不全，请重新填写",
		TargetNodeId: "submitter",
	})
	require.NoError(t, err)

	// 6. 验证处于驳回重审待办中，GetActiveReject 正确返回快照
	rejectSnap, err = adapter.GetActiveReject(ctx, instDomainID)
	require.NoError(t, err)
	require.NotNil(t, rejectSnap)
	assert.Equal(t, "auditor", rejectSnap.NodeID)
	assert.Equal(t, "主管审批", rejectSnap.NodeName)
	assert.Equal(t, "bob", rejectSnap.Operator)
	assert.Equal(t, "资料不全，请重新填写", rejectSnap.Comment)
	assert.NotEmpty(t, rejectSnap.FinishTime)

	// 7. 提单人 alice 重新修改后再次提交通过（走回来了）
	allTasks, err = coreEngine.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	newSubmitTask, found := lo.Find(allTasks, func(t *flowxModel.Task) bool {
		return t.NodeID == "submitter" && t.Status == flowxModel.TaskPending
	})
	require.True(t, found)

	err = adapter.Pass(ctx, domain.TaskPassReq{
		EngineType: domain.EngineTypeFlowx,
		TaskId:     domain.NewID(newSubmitTask.ID),
		Operator:   "alice",
		Comment:    "已补全材料重新提交",
	})
	require.NoError(t, err)

	// 8. 验证走回来后，驳回快照自动消隐（返回 nil）
	rejectSnapAfterAdvance, err := adapter.GetActiveReject(ctx, instDomainID)
	require.NoError(t, err)
	assert.Nil(t, rejectSnapAfterAdvance, "流程已被提单人响应并重新向前推进，旧驳回横幅必须自动消隐")
}

// TestFlowxAdapter_GetActiveReject_RetreatScenarios 覆盖各种回退形态下驳回横幅的展示与消隐时机。
// 核心不变量：意见必须一直挂到流程重新走回驳回节点，不能被"某个重审人先提交"提前抹掉——
// 自由驳回会同时打开多个上游节点，那条意见可能正是写给尚未响应的那个人的。
func TestFlowxAdapter_GetActiveReject_RetreatScenarios(t *testing.T) {
	type step struct {
		action   string // pass / reject
		node     string
		operator string
		target   string         // reject 的定向目标，空即自由驳回
		vars     map[string]any // pass 时合并的增量变量
		wantShow bool
		wantNode string
	}

	approval := func(id, name, assignee string) flowxModel.NodeSpec {
		return flowxModel.NodeSpec{ID: id, Name: name, Type: flowxModel.NodeApproval, Assignees: []string{assignee}}
	}

	testCases := []struct {
		name      string
		def       flowxModel.ProcessDefinition
		startVars map[string]any
		steps     []step
	}{
		{
			name: "并行双节点自由驳回_一人提交不得抹掉另一人的意见",
			def: flowxModel.ProcessDefinition{
				Key: "wf_banner_parallel", Name: "并行双节点",
				Nodes: []flowxModel.NodeSpec{
					{ID: "start", Type: flowxModel.NodeStart},
					{ID: "fork", Type: flowxModel.NodeGatewayParallel},
					approval("backend", "后端审批", "bob"),
					approval("algo", "算法审批", "dave"),
					{ID: "join", Type: flowxModel.NodeGatewayParallel},
					approval("product", "产品审批", "carol"),
					{ID: "end", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start", Target: "fork"}, {Source: "fork", Target: "backend"}, {Source: "fork", Target: "algo"},
					{Source: "backend", Target: "join"}, {Source: "algo", Target: "join"},
					{Source: "join", Target: "product"}, {Source: "product", Target: "end"},
				},
			},
			steps: []step{
				{action: "pass", node: "backend", operator: "bob"},
				{action: "pass", node: "algo", operator: "dave"},
				{action: "reject", node: "product", operator: "carol", wantShow: true, wantNode: "product"},
				{action: "pass", node: "backend", operator: "bob", wantShow: true, wantNode: "product"},
				{action: "pass", node: "algo", operator: "dave"},
			},
		},
		{
			name: "单目标定向驳回_提单人重报即闭环",
			def: flowxModel.ProcessDefinition{
				Key: "wf_banner_linear", Name: "串行单目标",
				Nodes: []flowxModel.NodeSpec{
					{ID: "start", Type: flowxModel.NodeStart},
					approval("submitter", "提单人录入", "alice"),
					approval("auditor", "主管审批", "bob"),
					{ID: "end", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start", Target: "submitter"}, {Source: "submitter", Target: "auditor"}, {Source: "auditor", Target: "end"},
				},
			},
			steps: []step{
				{action: "pass", node: "submitter", operator: "alice"},
				{action: "reject", node: "auditor", operator: "bob", target: "submitter", wantShow: true, wantNode: "auditor"},
				{action: "pass", node: "submitter", operator: "alice"},
			},
		},
		{
			name: "定向驳回跨多跳_中途停留期间意见持续可见",
			def: flowxModel.ProcessDefinition{
				Key: "wf_banner_multihop", Name: "多跳回退",
				Nodes: []flowxModel.NodeSpec{
					{ID: "start", Type: flowxModel.NodeStart},
					approval("submitter", "提单人录入", "alice"),
					approval("leader", "主管审批", "bob"),
					approval("product", "产品审批", "carol"),
					{ID: "end", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start", Target: "submitter"}, {Source: "submitter", Target: "leader"},
					{Source: "leader", Target: "product"}, {Source: "product", Target: "end"},
				},
			},
			steps: []step{
				{action: "pass", node: "submitter", operator: "alice"},
				{action: "pass", node: "leader", operator: "bob"},
				{action: "reject", node: "product", operator: "carol", target: "submitter", wantShow: true, wantNode: "product"},
				{action: "pass", node: "submitter", operator: "alice", wantShow: true, wantNode: "product"},
				{action: "pass", node: "leader", operator: "bob"},
			},
		},
		{
			name: "多轮驳回_每轮各自独立闭环",
			def: flowxModel.ProcessDefinition{
				Key: "wf_banner_rounds", Name: "多轮驳回",
				Nodes: []flowxModel.NodeSpec{
					{ID: "start", Type: flowxModel.NodeStart},
					approval("submitter", "提单人录入", "alice"),
					approval("leader", "主管审批", "bob"),
					approval("product", "产品审批", "carol"),
					approval("final", "终审", "eve"),
					{ID: "end", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start", Target: "submitter"}, {Source: "submitter", Target: "leader"},
					{Source: "leader", Target: "product"}, {Source: "product", Target: "final"}, {Source: "final", Target: "end"},
				},
			},
			steps: []step{
				{action: "pass", node: "submitter", operator: "alice"},
				{action: "pass", node: "leader", operator: "bob"},
				{action: "pass", node: "product", operator: "carol"},
				{action: "reject", node: "final", operator: "eve", target: "leader", wantShow: true, wantNode: "final"},
				{action: "pass", node: "leader", operator: "bob", wantShow: true, wantNode: "final"},
				{action: "pass", node: "product", operator: "carol"},
				{action: "reject", node: "final", operator: "eve", target: "leader", wantShow: true, wantNode: "final"},
				{action: "pass", node: "leader", operator: "bob", wantShow: true, wantNode: "final"},
				{action: "pass", node: "product", operator: "carol"},
			},
		},
		{
			name: "重报改道_驳回节点被跳过后横幅不得滞留",
			def: flowxModel.ProcessDefinition{
				Key: "wf_banner_reroute", Name: "重报改道",
				Nodes: []flowxModel.NodeSpec{
					{ID: "start", Type: flowxModel.NodeStart},
					approval("submitter", "提单人录入", "alice"),
					{ID: "xor", Type: flowxModel.NodeGatewayExclusive},
					approval("leader", "主管审批", "bob"),
					approval("product", "产品审批", "carol"),
					approval("other", "另一条分支审批", "dave"),
					{ID: "xorJoin", Type: flowxModel.NodeGatewayExclusive},
					{ID: "end", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start", Target: "submitter"}, {Source: "submitter", Target: "xor"},
					{Source: "xor", Target: "leader", Condition: "route == 'a'", Standard: true},
					{Source: "xor", Target: "other", Condition: "route == 'b'", Standard: true},
					{Source: "leader", Target: "product"}, {Source: "product", Target: "xorJoin"},
					{Source: "other", Target: "xorJoin"}, {Source: "xorJoin", Target: "end"},
				},
			},
			startVars: map[string]any{"route": "a"},
			steps: []step{
				{action: "pass", node: "submitter", operator: "alice"},
				{action: "pass", node: "leader", operator: "bob"},
				{action: "reject", node: "product", operator: "carol", target: "submitter", wantShow: true, wantNode: "product"},
				{action: "pass", node: "submitter", operator: "alice", vars: map[string]any{"route": "b"}},
				{action: "pass", node: "other", operator: "dave"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			core := flowx.NewEngine(flowxMem.NewStorage())
			adapter := NewEngine(core, nil)

			_, err := adapter.Deploy(ctx, tc.def)
			require.NoError(t, err)
			instID, err := core.Start(ctx, flowx.StartReq{Key: tc.def.Key, Starter: "alice", Variables: tc.startVars})
			require.NoError(t, err)

			// 每步都重新取待办：回退会重建任务，缓存旧 ID 会打到已办结的历史任务上
			pending := func(node string) *flowxModel.Task {
				all, listErr := core.ListTasksByInstance(ctx, instID)
				require.NoError(t, listErr)
				task, found := lo.Find(all, func(x *flowxModel.Task) bool {
					return x.NodeID == node && x.Status == flowxModel.TaskPending
				})
				require.True(t, found, "节点 %s 无待办任务", node)
				return task
			}

			for i, st := range tc.steps {
				task := pending(st.node)
				switch st.action {
				case "pass":
					require.NoError(t, adapter.Pass(ctx, domain.TaskPassReq{
						EngineType: domain.EngineTypeFlowx, TaskId: domain.NewID(task.ID),
						Operator: st.operator, Comment: "同意", Variables: st.vars,
					}))
				case "reject":
					require.NoError(t, adapter.Reject(ctx, domain.TaskRejectReq{
						EngineType: domain.EngineTypeFlowx, TaskId: domain.NewID(task.ID),
						Operator: st.operator, Comment: "算法侧口径不对，请重算", TargetNodeId: st.target,
					}))
				default:
					t.Fatalf("未知动作: %s", st.action)
				}

				snap, snapErr := adapter.GetActiveReject(ctx, domain.NewID(instID))
				require.NoError(t, snapErr)
				if !st.wantShow {
					assert.Nil(t, snap, "第%d步(%s %s)后横幅应消隐", i+1, st.action, st.node)
					continue
				}
				require.NotNil(t, snap, "第%d步(%s %s)后横幅应仍在展示", i+1, st.action, st.node)
				assert.Equal(t, st.wantNode, snap.NodeID, "第%d步后横幅应指向驳回节点", i+1)
				assert.Equal(t, "算法侧口径不对，请重算", snap.Comment, "第%d步后驳回意见不得丢失", i+1)
			}
		})
	}
}

// TestFlowxAdapter_GetPriorComments 验证前序意见的过滤、倒序与截断全部在驱动内完成，
// 且 OccurredAt 已是标准时间字符串，服务层不必再感知引擎时间形态
func TestFlowxAdapter_GetPriorComments(t *testing.T) {
	ctx := context.Background()
	base := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)

	seed := func(id, node, assignee string, status flowxModel.TaskStatus, comment string, finish time.Time) *flowxModel.Task {
		return &flowxModel.Task{
			ID: id, InstanceID: "inst_prior_comments",
			NodeID: node, NodeName: node + "环节",
			Assignee: assignee, Status: status, Comment: comment,
			CreateTime: base, FinishTime: finish,
		}
	}

	testCases := []struct {
		name         string
		nodes        []flowxModel.NodeSpec // 缺省时把任务涉及的全部节点视为人工审批环节
		tasks        []*flowxModel.Task
		limit        int
		wantTotal    int64
		wantComments []string
		wantTimes    []string
		wantFirst    *domain.PriorComment
	}{
		{
			name: "只保留人工裁决且意见非空白_待办与系统裁决全部剔除",
			tasks: []*flowxModel.Task{
				seed("t1", "backend", "alice", flowxModel.TaskPassed, "构建通过", base.Add(time.Hour)),
				seed("t2", "product", "bob", flowxModel.TaskRejected, "口径不对请重算", base.Add(3*time.Hour)),
				seed("t3", "algo", "carol", flowxModel.TaskPending, "", time.Time{}),
				seed("t4", "ops", "dave", flowxModel.TaskSystemPassed, "系统自动放行", base.Add(2*time.Hour)),
				seed("t5", "qa", "eve", flowxModel.TaskSkipped, "条件未命中跳过", base.Add(2*time.Hour)),
				seed("t6", "ui", "frank", flowxModel.TaskPassed, "   ", base.Add(4*time.Hour)),
				seed("t7", "db", "grace", flowxModel.TaskPassed, "", base.Add(5*time.Hour)),
				seed("t8", "sec", "heidi", flowxModel.TaskSystemRejected, "会签联动取消", base.Add(2*time.Hour)),
			},
			limit:        5,
			wantTotal:    2,
			wantComments: []string{"口径不对请重算", "构建通过"},
			wantTimes:    []string{"2026-11-10 12:00:00", "2026-11-10 10:00:00"},
			wantFirst: &domain.PriorComment{
				NodeID: "product", NodeName: "product环节",
				ApprovedBy: "bob", Status: 2,
				Comment: "口径不对请重算", OccurredAt: "2026-11-10 12:00:00",
			},
		},
		{
			name: "超过展开上限按办结时间倒序截断_总数仍为全量意见数",
			tasks: lo.Map(lo.Range(7), func(i int, _ int) *flowxModel.Task {
				return seed(fmt.Sprintf("t%d", i), "node", "alice", flowxModel.TaskPassed,
					fmt.Sprintf("意见%d", i+1), base.Add(time.Duration(i+1)*time.Hour))
			}),
			limit:        5,
			wantTotal:    7,
			wantComments: []string{"意见7", "意见6", "意见5", "意见4", "意见3"},
		},
		{
			name: "同一时刻办结按任务ID倒序兜底_顺序不随存储返回序漂移",
			tasks: []*flowxModel.Task{
				seed("a_task", "node", "alice", flowxModel.TaskPassed, "先入库", base.Add(time.Hour)),
				seed("b_task", "node", "bob", flowxModel.TaskPassed, "后入库", base.Add(time.Hour)),
			},
			limit:        5,
			wantTotal:    2,
			wantComments: []string{"后入库", "先入库"},
		},
		{
			name: "limit非正数时不截断",
			tasks: []*flowxModel.Task{
				seed("t1", "node", "alice", flowxModel.TaskPassed, "意见1", base.Add(time.Hour)),
				seed("t2", "node", "bob", flowxModel.TaskPassed, "意见2", base.Add(2*time.Hour)),
				seed("t3", "node", "carol", flowxModel.TaskPassed, "意见3", base.Add(3*time.Hour)),
			},
			limit:        0,
			wantTotal:    3,
			wantComments: []string{"意见3", "意见2", "意见1"},
		},
		{
			name: "链路无任何可读意见时返回空摘要",
			tasks: []*flowxModel.Task{
				seed("t1", "algo", "carol", flowxModel.TaskPending, "", time.Time{}),
				seed("t2", "ui", "frank", flowxModel.TaskPassed, "  ", base.Add(time.Hour)),
			},
			limit:        5,
			wantTotal:    0,
			wantComments: []string{},
		},
		{
			name: "抄送群聊与自动化节点的系统文案全部剔除_只留审批与提单环节",
			nodes: []flowxModel.NodeSpec{
				{ID: "start", Name: "提单", Type: flowxModel.NodeStart},
				{ID: "leader", Name: "主管审批", Type: flowxModel.NodeApproval},
				{ID: "cc", Name: "抄送知会", Type: flowxModel.NodeCarbonCopy},
				{ID: "chat", Name: "群聊通知", Type: flowxModel.NodeChatGroup},
				{ID: "auto", Name: "自动化作业", Type: flowxModel.NodeTask},
				{ID: "end", Name: "结束", Type: flowxModel.NodeEnd},
			},
			tasks: []*flowxModel.Task{
				seed("t1", "start", "alice", flowxModel.TaskPassed, "补充了预算明细", base.Add(time.Hour)),
				seed("t2", "leader", "bob", flowxModel.TaskPassed, "同意，注意成本控制", base.Add(2*time.Hour)),
				// 抄送解析到真实处理人时同样写 TaskPassed，按处理人哨兵判定人机会漏掉这一条
				seed("t3", "cc", "carol", flowxModel.TaskPassed, "系统抄送知会", base.Add(3*time.Hour)),
				seed("t4", "chat", "chat_group", flowxModel.TaskPassed, "群聊通知自动完成", base.Add(4*time.Hour)),
				seed("t5", "auto", "system", flowxModel.TaskPassed, "系统自动执行成功", base.Add(5*time.Hour)),
				seed("t6", "leader", "dave", flowxModel.TaskRejected, "预算超标，请重填", base.Add(6*time.Hour)),
			},
			limit:        5,
			wantTotal:    3,
			wantComments: []string{"预算超标，请重填", "同意，注意成本控制", "补充了预算明细"},
		},
		{
			name: "全链路只有机器节点时不得回落到未过滤口径",
			nodes: []flowxModel.NodeSpec{
				{ID: "cc", Name: "抄送知会", Type: flowxModel.NodeCarbonCopy},
				{ID: "chat", Name: "群聊通知", Type: flowxModel.NodeChatGroup},
				{ID: "auto", Name: "自动化作业", Type: flowxModel.NodeTask},
			},
			tasks: []*flowxModel.Task{
				seed("t1", "cc", "carol", flowxModel.TaskPassed, "系统抄送知会", base.Add(time.Hour)),
				seed("t2", "chat", "chat_group", flowxModel.TaskPassed, "群聊通知自动完成", base.Add(2*time.Hour)),
				seed("t3", "auto", "system", flowxModel.TaskPassed, "系统异步作业执行完成", base.Add(3*time.Hour)),
			},
			limit:        5,
			wantTotal:    0,
			wantComments: []string{},
		},
		{
			name: "机器节点不得挤占展开名额_截断必须发生在节点类型过滤之后",
			nodes: []flowxModel.NodeSpec{
				{ID: "leader", Name: "主管审批", Type: flowxModel.NodeApproval},
				{ID: "cc", Name: "抄送知会", Type: flowxModel.NodeCarbonCopy},
			},
			// 机器条目全部晚于人工意见：若先截断再过滤，整页会被机器条目占满而人工意见一条不剩
			tasks: append(
				lo.Map(lo.Range(3), func(i int, _ int) *flowxModel.Task {
					return seed(fmt.Sprintf("h%d", i), "leader", "alice", flowxModel.TaskPassed,
						fmt.Sprintf("人工意见%d", i+1), base.Add(time.Duration(i+1)*time.Hour))
				}),
				lo.Map(lo.Range(7), func(i int, _ int) *flowxModel.Task {
					return seed(fmt.Sprintf("m%d", i), "cc", "system", flowxModel.TaskPassed,
						"系统抄送知会", base.Add(time.Duration(i+4)*time.Hour))
				})...,
			),
			limit:        5,
			wantTotal:    3,
			wantComments: []string{"人工意见3", "人工意见2", "人工意见1"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			store := flowxMem.NewStorage()
			require.NoError(t, store.SaveTasks(ctx, tc.tasks))

			// 节点类型是判定人机的唯一依据，必须落定义与实例，否则驱动无法回源
			nodes := tc.nodes
			if nodes == nil {
				nodes = lo.Map(lo.UniqBy(tc.tasks, func(t *flowxModel.Task) string { return t.NodeID }),
					func(t *flowxModel.Task, _ int) flowxModel.NodeSpec {
						return flowxModel.NodeSpec{ID: t.NodeID, Name: t.NodeName, Type: flowxModel.NodeApproval}
					})
			}
			def := flowxModel.ProcessDefinition{Key: "wf_prior_comments", Version: 1, Nodes: nodes}
			require.NoError(t, store.SaveDefinition(ctx, def))
			require.NoError(t, store.SaveInstance(ctx, &flowxModel.Instance{
				ID: "inst_prior_comments", ProcKey: def.Key, Version: def.Version,
				Status: flowxModel.InstanceRunning, Variables: flowxModel.Variables{},
			}))

			adapter := NewEngine(flowx.NewEngine(store), nil)

			comments, total, err := adapter.GetPriorComments(ctx, domain.NewID("inst_prior_comments"), tc.limit)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total)
			assert.Equal(t, tc.wantComments, lo.Map(comments, func(c domain.PriorComment, _ int) string { return c.Comment }))
			if tc.wantTimes != nil {
				assert.Equal(t, tc.wantTimes, lo.Map(comments, func(c domain.PriorComment, _ int) string { return c.OccurredAt }))
			}
			if tc.wantFirst != nil {
				require.NotEmpty(t, comments)
				assert.Equal(t, *tc.wantFirst, comments[0], "驱动必须完成字段映射与时间格式化，真名留空由服务层填充")
			}
		})
	}
}
