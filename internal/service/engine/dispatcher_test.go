package engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	flowxDrvPkg "github.com/Duke1616/eflow/internal/service/engine/flowx"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEngineDispatcher_Deploy(t *testing.T) {
	testCases := []struct {
		name       string
		engineType domain.EngineType
		rawDef     any
		mockSetup  func(easyMock *enginemocks.MockIEngine)
		assertFunc func(t *testing.T, info domain.DeployInfo)
		wantErr    bool
		errSubStr  string
	}{
		{
			name:       "flowx引擎成功部署",
			engineType: domain.EngineTypeFlowx,
			rawDef: flowxModel.ProcessDefinition{
				Key:     "proc_101",
				Name:    "测试流程",
				Version: 2,
				Nodes: []flowxModel.NodeSpec{
					{ID: "start_1", Type: flowxModel.NodeStart},
					{ID: "end_1", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start_1", Target: "end_1"},
				},
			},
			assertFunc: func(t *testing.T, info domain.DeployInfo) {
				assert.Equal(t, 2, info.Version)
				assert.Equal(t, 0, info.ProcessID)
			},
		},
		{
			name:       "easyflow引擎成功部署",
			engineType: domain.EngineTypeEasyFlow,
			rawDef:     "fake_def",
			mockSetup: func(easyMock *enginemocks.MockIEngine) {
				easyMock.EXPECT().Deploy(gomock.Any(), domain.EngineTypeEasyFlow, "fake_def").
					Return(domain.DeployInfo{ProcessID: 555, Version: 1}, nil)
			},
			assertFunc: func(t *testing.T, info domain.DeployInfo) {
				assert.Equal(t, 555, info.ProcessID)
				assert.Equal(t, 1, info.Version)
			},
		},
		{
			name:       "未注册的引擎类型报错拦截",
			engineType: "unknown_engine",
			rawDef:     nil,
			wantErr:    true,
			errSubStr:  "未注册对应的流程引擎驱动",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			easyMock := enginemocks.NewMockIEngine(ctrl)
			easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
			if tc.mockSetup != nil {
				tc.mockSetup(easyMock)
			}

			flowxEng := flowx.NewEngine(memory.NewStorage())
			flowxDrv := flowxDrvPkg.NewEngine(flowxEng, nil)

			dispatcher := engine.NewEngineDispatcher(easyMock, flowxDrv)
			info, err := dispatcher.Deploy(context.Background(), tc.engineType, tc.rawDef)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubStr != "" {
					assert.Contains(t, err.Error(), tc.errSubStr)
				}
				return
			}

			require.NoError(t, err)
			tc.assertFunc(t, info)
		})
	}
}

func TestEngineDispatcher_Launch(t *testing.T) {
	testCases := []struct {
		name       string
		engineType domain.EngineType
		req        domain.ProcessLaunchReq
		mockSetup  func(easyMock *enginemocks.MockIEngine)
		assertFunc func(t *testing.T, res domain.ProcessLaunchResult)
		wantErr    bool
		errSubStr  string
	}{
		{
			name:       "flowx引擎成功启动流程实例",
			engineType: domain.EngineTypeFlowx,
			req: domain.ProcessLaunchReq{
				TicketId:     9999,
				WorkflowId:   1001,
				WorkflowName: "请假审批",
				Starter:      "tom",
				Variables:    map[string]any{"days": 3},
			},
			assertFunc: func(t *testing.T, res domain.ProcessLaunchResult) {
				assert.Equal(t, domain.EngineTypeFlowx, res.EngineType)
				assert.False(t, res.InstanceID.IsZero())
			},
		},
		{
			name:       "easyflow引擎成功启动流程实例",
			engineType: domain.EngineTypeEasyFlow,
			req: domain.ProcessLaunchReq{
				TicketId:     8888,
				WorkflowId:   2002,
				ProcessId:    777,
				WorkflowName: "采购申请",
				Starter:      "jack",
				Variables:    map[string]any{"amount": 100},
			},
			mockSetup: func(easyMock *enginemocks.MockIEngine) {
				easyMock.EXPECT().Start(gomock.Any(), gomock.Any()).
					Return(domain.ProcessLaunchResult{
						EngineType: domain.EngineTypeEasyFlow,
						InstanceID: domain.NewID(654),
					}, nil)
			},
			assertFunc: func(t *testing.T, res domain.ProcessLaunchResult) {
				assert.Equal(t, domain.EngineTypeEasyFlow, res.EngineType)
				assert.Equal(t, 654, res.InstanceID.Int())
			},
		},
		{
			name:       "入参校验防呆_工单ID缺失拦截",
			engineType: domain.EngineTypeFlowx,
			req: domain.ProcessLaunchReq{
				TicketId: 0,
				Starter:  "tom",
			},
			wantErr:   true,
			errSubStr: "工单 TicketId 非法",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			easyMock := enginemocks.NewMockIEngine(ctrl)
			easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
			if tc.mockSetup != nil {
				tc.mockSetup(easyMock)
			}

			flowxEng := flowx.NewEngine(memory.NewStorage())
			_, err := flowxEng.Deploy(context.Background(), flowxModel.ProcessDefinition{
				Key:  "wf_1001",
				Name: "请假审批",
				Nodes: []flowxModel.NodeSpec{
					{ID: "start_1", Type: flowxModel.NodeStart},
					{ID: "end_1", Type: flowxModel.NodeEnd},
				},
				Edges: []flowxModel.EdgeSpec{
					{Source: "start_1", Target: "end_1"},
				},
			})
			require.NoError(t, err)

			flowxDrv := flowxDrvPkg.NewEngine(flowxEng, nil)
			dispatcher := engine.NewEngineDispatcher(easyMock, flowxDrv)

			tc.req.EngineType = tc.engineType
			res, err := dispatcher.Start(context.Background(), tc.req)
			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubStr != "" {
					assert.Contains(t, err.Error(), tc.errSubStr)
				}
				return
			}

			require.NoError(t, err)
			if tc.assertFunc != nil {
				tc.assertFunc(t, res)
			}
		})
	}
}

func TestEngineDispatcher_PassAndReject(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	easyMock := enginemocks.NewMockIEngine(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIEngine(ctrl)
	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

	// 1. 测试显式携带 EngineType 直接下发到 easyflow
	easyPassReq := domain.TaskPassReq{
		EngineType: domain.EngineTypeEasyFlow,
		TaskId:     domain.NewID(123),
		Operator:   "alice",
		Comment:    "同意",
	}
	easyMock.EXPECT().Pass(gomock.Any(), easyPassReq).Return(nil)
	err := dispatcher.Pass(context.Background(), easyPassReq)
	require.NoError(t, err)

	// 2. 测试显式携带 EngineType 直接下发到 flowx
	flowxPassReq := domain.TaskPassReq{
		EngineType: domain.EngineTypeFlowx,
		TaskId:     domain.NewID("task_nodeAudit_bob_b1_1"),
		Operator:   "bob",
		Comment:    "同意",
	}
	flowxMock.EXPECT().Pass(gomock.Any(), flowxPassReq).Return(nil)
	err = dispatcher.Pass(context.Background(), flowxPassReq)
	require.NoError(t, err)

	// 3. 测试 Reject 显式确定性路由
	rejectReq := domain.TaskRejectReq{
		EngineType: domain.EngineTypeEasyFlow,
		TaskId:     domain.NewID(123),
		Operator:   "alice",
		Comment:    "材料不合规驳回",
	}
	easyMock.EXPECT().Reject(gomock.Any(), rejectReq).Return(nil)
	err = dispatcher.Reject(context.Background(), rejectReq)
	require.NoError(t, err)

	// 4. 校验防呆_Reject缺少原因
	invalidReject := domain.TaskRejectReq{
		TaskId:   domain.NewID(123),
		Operator: "alice",
		Comment:  "",
	}
	err = dispatcher.Reject(context.Background(), invalidReject)
	require.Error(t, err)
	assert.ErrorIs(t, err, engine.ErrInvalidParameter)

	// 5. 校验防呆_TaskId为空
	emptyTaskReject := domain.TaskRejectReq{
		TaskId:   "",
		Operator: "alice",
		Comment:  "驳回",
	}
	err = dispatcher.Reject(context.Background(), emptyTaskReject)
	require.Error(t, err)
	assert.ErrorIs(t, err, engine.ErrInvalidParameter)

	// 6. 测试未显式提供 EngineType 时直接拦截报错（契约固定，杜绝隐式猜测）
	noEnginePassReq := domain.TaskPassReq{
		TaskId:   domain.NewID("377369916761903105"),
		Operator: "zhangsan",
		Comment:  "同意",
	}
	err = dispatcher.Pass(context.Background(), noEnginePassReq)
	require.Error(t, err)
	assert.ErrorIs(t, err, engine.ErrEngineNotFound)

	// 7. 测试显式携带 EngineTypeFlowx 时精准路由到 FlowX
	snowflakePassReq := domain.TaskPassReq{
		EngineType: domain.EngineTypeFlowx,
		TaskId:     domain.NewID("377369916761903105"),
		Operator:   "zhangsan",
		Comment:    "同意",
	}
	flowxMock.EXPECT().Pass(gomock.Any(), gomock.AssignableToTypeOf(domain.TaskPassReq{})).DoAndReturn(
		func(_ context.Context, req domain.TaskPassReq) error {
			assert.Equal(t, domain.EngineTypeFlowx, req.EngineType)
			assert.Equal(t, domain.NewID("377369916761903105"), req.TaskId)
			return nil
		},
	)
	err = dispatcher.Pass(context.Background(), snowflakePassReq)
	require.NoError(t, err)

	// 7. 测试显式携带 EngineTypeEasyFlow 时精准路由到 EasyFlow 防腐层
	legacyPassReq := domain.TaskPassReq{
		EngineType: domain.EngineTypeEasyFlow,
		TaskId:     domain.NewID("1002"),
		Operator:   "alice",
		Comment:    "同意",
	}
	easyMock.EXPECT().Pass(gomock.Any(), gomock.AssignableToTypeOf(domain.TaskPassReq{})).DoAndReturn(
		func(_ context.Context, req domain.TaskPassReq) error {
			assert.Equal(t, domain.EngineTypeEasyFlow, req.EngineType)
			assert.Equal(t, domain.NewID("1002"), req.TaskId)
			return nil
		},
	)
	err = dispatcher.Pass(context.Background(), legacyPassReq)
	require.NoError(t, err)
}

func TestEngineDispatcher_GetTaskContext_DirectDispatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	easyMock := enginemocks.NewMockIEngine(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIEngine(ctrl)
	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

	// 1. 指定 FlowX 引擎直接分发命中
	taskID1 := domain.NewID("flowx_task_1001")
	flowxMock.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeFlowx, taskID1).Return(domain.TaskContext{
		EngineType: domain.EngineTypeFlowx,
		TaskID:     taskID1,
		NodeID:     "node_approve",
	}, nil)

	ctx1, err := dispatcher.GetTaskContext(context.Background(), domain.EngineTypeFlowx, taskID1)
	require.NoError(t, err)
	assert.Equal(t, domain.EngineTypeFlowx, ctx1.EngineType)
	assert.Equal(t, "node_approve", ctx1.NodeID)

	// 2. 指定 EasyFlow 引擎直接分发命中
	taskID2 := domain.NewID("2002")
	easyMock.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, taskID2).Return(domain.TaskContext{
		EngineType: domain.EngineTypeEasyFlow,
		TaskID:     taskID2,
		NodeID:     "node_easy",
	}, nil)

	ctx2, err := dispatcher.GetTaskContext(context.Background(), domain.EngineTypeEasyFlow, taskID2)
	require.NoError(t, err)
	assert.Equal(t, domain.EngineTypeEasyFlow, ctx2.EngineType)
	assert.Equal(t, "node_easy", ctx2.NodeID)

	// 3. 未知引擎直接返回未注册错误，绝不盲目试探
	_, err = dispatcher.GetTaskContext(context.Background(), domain.EngineType("unknown"), taskID2)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "未注册对应的流程引擎驱动")
}

func TestEngineDispatcher_ListTodoTasks_GlobalSortAndPage(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	easyMock := enginemocks.NewMockIEngine(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIEngine(ctrl)
	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

	now := time.Now()
	taskA := domain.TodoTask{
		EngineType: domain.EngineTypeFlowx,
		TaskID:     domain.NewID("task_flowx_1"),
		CreateTime: now.Add(-30 * time.Minute), // 最早
	}
	taskB := domain.TodoTask{
		EngineType: domain.EngineTypeEasyFlow,
		TaskID:     domain.NewID("task_easy_2"),
		CreateTime: now.Add(-15 * time.Minute), // 中间
	}
	taskC := domain.TodoTask{
		EngineType: domain.EngineTypeFlowx,
		TaskID:     domain.NewID("task_flowx_3"),
		CreateTime: now, // 最新
	}

	// 模拟两个引擎返回各自待办（无需引擎层做分页偏移，全量拉取）
	expectedReq := domain.TodoQueryReq{
		Username: "alice",
		Offset:   0,
		Limit:    500,
	}

	flowxMock.EXPECT().ListTodoTasks(gomock.Any(), expectedReq).Return([]domain.TodoTask{taskA, taskC}, int64(2), nil)
	easyMock.EXPECT().ListTodoTasks(gomock.Any(), expectedReq).Return([]domain.TodoTask{taskB}, int64(1), nil)

	// 1. 请求第一页，Limit = 2
	paged1, total, err := dispatcher.ListTodoTasks(context.Background(), domain.TodoQueryReq{
		Username: "alice",
		Offset:   0,
		Limit:    2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total)
	require.Len(t, paged1, 2)
	// 验证严格全局时间倒序：最新(C) -> 中间(B)
	assert.Equal(t, domain.NewID("task_flowx_3"), paged1[0].TaskID)
	assert.Equal(t, domain.NewID("task_easy_2"), paged1[1].TaskID)

	// 2. 模拟请求第二页（offset=2, limit=2）
	flowxMock.EXPECT().ListTodoTasks(gomock.Any(), expectedReq).Return([]domain.TodoTask{taskA, taskC}, int64(2), nil)
	easyMock.EXPECT().ListTodoTasks(gomock.Any(), expectedReq).Return([]domain.TodoTask{taskB}, int64(1), nil)

	paged2, total2, err := dispatcher.ListTodoTasks(context.Background(), domain.TodoQueryReq{
		Username: "alice",
		Offset:   2,
		Limit:    2,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(3), total2)
	require.Len(t, paged2, 1)
	// 验证第二页命中最早的任务(A)
	assert.Equal(t, domain.NewID("task_flowx_1"), paged2[0].TaskID)
}
