package flowx

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	flowxMem "github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowxAdapter_EndToEndLifecycle(t *testing.T) {
	ctx := context.Background()

	// 1. 初始化基于内存仓储的 FlowX 内核
	store := flowxMem.NewStorage()
	coreEngine := flowx.NewEngine(store)
	adapter := NewEngine(coreEngine)

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
