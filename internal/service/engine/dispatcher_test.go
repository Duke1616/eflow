package engine_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	flowxDrvPkg "github.com/Duke1616/eflow/internal/service/engine/flowx"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestEngineDispatcher_Deploy(t *testing.T) {
	testCases := []struct {
		name       string
		engineType domain.EngineType
		rawDef     any
		mockSetup  func(easyMock *enginemocks.MockIDriver)
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
			mockSetup: func(easyMock *enginemocks.MockIDriver) {
				easyMock.EXPECT().Deploy(gomock.Any(), "fake_def").
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

			easyMock := enginemocks.NewMockIDriver(ctrl)
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
		mockSetup  func(easyMock *enginemocks.MockIDriver)
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
			mockSetup: func(easyMock *enginemocks.MockIDriver) {
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

			easyMock := enginemocks.NewMockIDriver(ctrl)
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

	easyMock := enginemocks.NewMockIDriver(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIDriver(ctrl)
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

	easyMock := enginemocks.NewMockIDriver(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIDriver(ctrl)
	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

	// 1. 指定 FlowX 引擎直接分发命中
	taskID1 := domain.NewID("flowx_task_1001")
	flowxMock.EXPECT().GetTaskContext(gomock.Any(), taskID1).Return(domain.TaskContext{
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
	easyMock.EXPECT().GetTaskContext(gomock.Any(), taskID2).Return(domain.TaskContext{
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

	easyMock := enginemocks.NewMockIDriver(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIDriver(ctrl)
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

	// 各引擎只需返回各自的前 K 条（K = offset + limit），全局前 K 条必然包含于其并集
	page1Req := domain.TodoQueryReq{
		Username: "alice",
		Offset:   0,
		Limit:    2, // K = 0 + 2
	}

	flowxMock.EXPECT().ListTodoTasks(gomock.Any(), page1Req).Return([]domain.TodoTask{taskC, taskA}, int64(2), nil)
	easyMock.EXPECT().ListTodoTasks(gomock.Any(), page1Req).Return([]domain.TodoTask{taskB}, int64(1), nil)

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

	// 2. 模拟请求第二页（offset=2, limit=2），此时 K 增大到 4
	page2Req := domain.TodoQueryReq{
		Username: "alice",
		Offset:   0,
		Limit:    4, // K = 2 + 2
	}
	flowxMock.EXPECT().ListTodoTasks(gomock.Any(), page2Req).Return([]domain.TodoTask{taskC, taskA}, int64(2), nil)
	easyMock.EXPECT().ListTodoTasks(gomock.Any(), page2Req).Return([]domain.TodoTask{taskB}, int64(1), nil)

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

func TestEngineDispatcher_ListTodoTasks_FetchLimitAndTrueTotal(t *testing.T) {
	now := time.Now()

	testCases := []struct {
		name          string
		req           domain.TodoQueryReq
		wantFetchLim  int
		engineTotal   int64
		engineRows    int
		wantTotal     int64
		wantSortedAsc bool
	}{
		{
			name:         "首页只取前limit条_不再固定拉500",
			req:          domain.TodoQueryReq{Username: "alice", Offset: 0, Limit: 20},
			wantFetchLim: 20,
			engineTotal:  3000,
			engineRows:   20,
			wantTotal:    6000, // 两个引擎各 3000，total 必须是真实计数之和而非返回行数
		},
		{
			name:         "深翻页按页深取前K条_total仍为真实计数",
			req:          domain.TodoQueryReq{Username: "alice", Offset: 1000, Limit: 20},
			wantFetchLim: 1020,
			engineTotal:  3000,
			engineRows:   1020,
			wantTotal:    6000,
		},
		{
			name:         "limit为0语义不限条数_下推负一规避GORM_LIMIT_0陷阱",
			req:          domain.TodoQueryReq{Username: "alice", Offset: 0, Limit: 0},
			wantFetchLim: -1,
			engineTotal:  3000,
			engineRows:   3000,
			wantTotal:    6000,
		},
		{
			name:          "升序请求必须透传给引擎_否则归并取到错误的一端",
			req:           domain.TodoQueryReq{Username: "alice", Offset: 0, Limit: 10, SortByAsc: true},
			wantFetchLim:  10,
			engineTotal:   5,
			engineRows:    5,
			wantTotal:     10,
			wantSortedAsc: true,
		},
		{
			name:         "负offset不得污染下推的取数条数",
			req:          domain.TodoQueryReq{Username: "alice", Offset: -5, Limit: 10},
			wantFetchLim: 10,
			engineTotal:  5,
			engineRows:   5,
			wantTotal:    10,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			easyMock := enginemocks.NewMockIDriver(ctrl)
			easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
			flowxMock := enginemocks.NewMockIDriver(ctrl)
			flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

			dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

			wantReq := domain.TodoQueryReq{
				Username:  tc.req.Username,
				Offset:    0,
				Limit:     tc.wantFetchLim,
				SortByAsc: tc.req.SortByAsc,
			}

			// 构造引擎侧有序返回：倒序时最新在前，升序时最早在前
			rows := make([]domain.TodoTask, 0, tc.engineRows)
			for i := 0; i < tc.engineRows; i++ {
				offset := time.Duration(i) * time.Minute
				if !tc.req.SortByAsc {
					offset = -offset
				}
				rows = append(rows, domain.TodoTask{
					TaskID:     domain.NewID(fmt.Sprintf("task_%d", i)),
					CreateTime: now.Add(offset),
				})
			}

			flowxMock.EXPECT().ListTodoTasks(gomock.Any(), wantReq).Return(rows, tc.engineTotal, nil)
			easyMock.EXPECT().ListTodoTasks(gomock.Any(), wantReq).Return(rows, tc.engineTotal, nil)

			got, total, err := dispatcher.ListTodoTasks(context.Background(), tc.req)
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total, "total 必须来自引擎真实计数，绝不能被返回行数顶替")

			if tc.req.Offset < 0 {
				assert.Empty(t, got)
				return
			}
			if tc.req.Limit > 0 {
				assert.LessOrEqual(t, len(got), tc.req.Limit)
			}
			// 校验归并结果的全局有序性
			for i := 1; i < len(got); i++ {
				if tc.wantSortedAsc {
					assert.False(t, got[i].CreateTime.Before(got[i-1].CreateTime), "升序归并结果乱序")
				} else {
					assert.False(t, got[i].CreateTime.After(got[i-1].CreateTime), "倒序归并结果乱序")
				}
			}
		})
	}
}

func TestEngineDispatcher_ListTodoTasks_SameTimestampDeterministicOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	easyMock := enginemocks.NewMockIDriver(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
	flowxMock := enginemocks.NewMockIDriver(ctrl)
	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

	// 两个引擎返回完全相同时间戳的任务：缺少确定性 tiebreak 时翻页会漂移
	sameTime := time.Now()
	flowxRows := []domain.TodoTask{
		{TaskID: domain.NewID("f_b"), CreateTime: sameTime},
		{TaskID: domain.NewID("f_a"), CreateTime: sameTime},
	}
	easyRows := []domain.TodoTask{
		{TaskID: domain.NewID("e_b"), CreateTime: sameTime},
		{TaskID: domain.NewID("e_a"), CreateTime: sameTime},
	}

	req := domain.TodoQueryReq{Username: "alice", Offset: 0, Limit: 4}
	var firstPage []domain.ID
	for round := 0; round < 5; round++ {
		flowxMock.EXPECT().ListTodoTasks(gomock.Any(), req).Return(flowxRows, int64(2), nil)
		easyMock.EXPECT().ListTodoTasks(gomock.Any(), req).Return(easyRows, int64(2), nil)

		got, total, err := dispatcher.ListTodoTasks(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, int64(4), total)
		require.Len(t, got, 4)

		ids := lo.Map(got, func(task domain.TodoTask, _ int) domain.ID { return task.TaskID })
		if round == 0 {
			firstPage = ids
			// 同时间戳按 TaskID 升序定序
			assert.Equal(t, []domain.ID{"e_a", "e_b", "f_a", "f_b"}, ids)
			continue
		}
		assert.Equal(t, firstPage, ids, "同时间戳任务在重复请求间排序必须稳定，否则翻页重复或遗漏")
	}
}

func TestEngineDispatcher_GetInstanceExecutionView_CrossEngineFallback(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	flowxMock := enginemocks.NewMockIDriver(ctrl)
	easyMock := enginemocks.NewMockIDriver(ctrl)

	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(flowxMock, easyMock)

	instID := domain.NewID("flowx_history_inst_123")

	// 模拟指定 EasyFlow 查询历史 FlowX 实例时 EasyFlow 报未找到
	easyMock.EXPECT().GetInstanceExecutionView(gomock.Any(), instID).
		Return(domain.InstanceExecutionView{}, errors.New("easyflow 获取实例详情失败: record not found"))

	// 期望调度器自动降级尝试 FlowX 并成功返回
	flowxMock.EXPECT().GetInstanceExecutionView(gomock.Any(), instID).
		Return(domain.InstanceExecutionView{
			ProcessKey: "wf_key_45",
			ProcessID:  45,
		}, nil)

	view, err := dispatcher.GetInstanceExecutionView(context.Background(), domain.EngineTypeEasyFlow, instID)
	require.NoError(t, err)
	assert.Equal(t, "wf_key_45", view.ProcessKey)
	assert.Equal(t, 45, view.ProcessID)
}
