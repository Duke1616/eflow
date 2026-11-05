package engine_test

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	flowxAdapter "github.com/Duke1616/eflow/internal/service/engine/flowx"
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
		mockSetup  func(easyMock *enginemocks.MockIEngineAdapter)
		assertFunc func(t *testing.T, info engine.DeployInfo)
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
			assertFunc: func(t *testing.T, info engine.DeployInfo) {
				assert.Equal(t, 2, info.Version)
				assert.Equal(t, 0, info.ProcessID)
			},
		},
		{
			name:       "easyflow引擎成功部署",
			engineType: domain.EngineTypeEasyFlow,
			rawDef:     "fake_def",
			mockSetup: func(easyMock *enginemocks.MockIEngineAdapter) {
				easyMock.EXPECT().Deploy(gomock.Any(), "fake_def").
					Return(engine.DeployInfo{ProcessID: 555, Version: 1}, nil)
			},
			assertFunc: func(t *testing.T, info engine.DeployInfo) {
				assert.Equal(t, 555, info.ProcessID)
				assert.Equal(t, 1, info.Version)
			},
		},
		{
			name:       "未注册的引擎类型报错拦截",
			engineType: "unknown_engine",
			rawDef:     nil,
			wantErr:    true,
			errSubStr:  "未注册对应的流程引擎适配器",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			easyMock := enginemocks.NewMockIEngineAdapter(ctrl)
			easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
			if tc.mockSetup != nil {
				tc.mockSetup(easyMock)
			}

			flowxEng := flowx.NewEngine(memory.NewStorage())
			flowxDrv := flowxAdapter.NewAdapter(flowxEng)

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
		req        engine.ProcessLaunchReq
		mockSetup  func(easyMock *enginemocks.MockIEngineAdapter)
		assertFunc func(t *testing.T, res engine.ProcessLaunchResult)
		wantErr    bool
		errSubStr  string
	}{
		{
			name:       "flowx引擎成功启动流程实例",
			engineType: domain.EngineTypeFlowx,
			req: engine.ProcessLaunchReq{
				TicketId:     9999,
				WorkflowId:   1001,
				WorkflowName: "请假审批",
				Starter:      "tom",
				Variables:    map[string]any{"days": 3},
			},
			assertFunc: func(t *testing.T, res engine.ProcessLaunchResult) {
				assert.Equal(t, 9999, res.InstanceId)
				assert.NotEmpty(t, res.InstanceKey)
			},
		},
		{
			name:       "easyflow引擎成功启动流程实例",
			engineType: domain.EngineTypeEasyFlow,
			req: engine.ProcessLaunchReq{
				TicketId:     8888,
				WorkflowId:   2002,
				ProcessId:    777,
				WorkflowName: "采购申请",
				Starter:      "jack",
			}.WithRawVariables(`{"amount":100}`),
			mockSetup: func(easyMock *enginemocks.MockIEngineAdapter) {
				easyMock.EXPECT().Start(gomock.Any(), gomock.Any()).
					Return(engine.ProcessLaunchResult{InstanceId: 654, InstanceKey: "654"}, nil)
			},
			assertFunc: func(t *testing.T, res engine.ProcessLaunchResult) {
				assert.Equal(t, 654, res.InstanceId)
				assert.Equal(t, "654", res.InstanceKey)
			},
		},
		{
			name:       "入参校验防呆_工单ID缺失拦截",
			engineType: domain.EngineTypeFlowx,
			req: engine.ProcessLaunchReq{
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

			easyMock := enginemocks.NewMockIEngineAdapter(ctrl)
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

			flowxDrv := flowxAdapter.NewAdapter(flowxEng)
			dispatcher := engine.NewEngineDispatcher(easyMock, flowxDrv)

			tc.req.EngineType = tc.engineType
			res, err := dispatcher.Launch(context.Background(), tc.req)
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

	easyMock := enginemocks.NewMockIEngineAdapter(ctrl)
	easyMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	flowxMock := enginemocks.NewMockIEngineAdapter(ctrl)
	flowxMock.EXPECT().EngineType().Return(domain.EngineTypeFlowx).AnyTimes()

	dispatcher := engine.NewEngineDispatcher(easyMock, flowxMock)

	// 1. 测试确定性路由到 easyflow (纯整型数字任务 ID)
	easyPassReq := engine.TaskPassReq{
		TaskId:   engine.NewID(123),
		Operator: "alice",
		Comment:  "同意",
	}
	easyMock.EXPECT().Pass(gomock.Any(), easyPassReq).Return(nil)
	err := dispatcher.Pass(context.Background(), easyPassReq)
	require.NoError(t, err)

	// 2. 测试确定性路由到 flowx (带 task_ 前缀特征)
	flowxPassReq := engine.TaskPassReq{
		TaskId:   engine.NewID("task_nodeAudit_bob_b1_1"),
		Operator: "bob",
		Comment:  "同意",
	}
	flowxMock.EXPECT().Pass(gomock.Any(), flowxPassReq).Return(nil)
	err = dispatcher.Pass(context.Background(), flowxPassReq)
	require.NoError(t, err)

	// 3. 测试 Reject 确定性路由
	rejectReq := engine.TaskRejectReq{
		TaskId:   engine.NewID(123),
		Operator: "alice",
		Comment:  "材料不合规驳回",
	}
	easyMock.EXPECT().Reject(gomock.Any(), rejectReq).Return(nil)
	err = dispatcher.Reject(context.Background(), rejectReq)
	require.NoError(t, err)

	// 4. 校验防呆_Reject缺少原因
	invalidReject := engine.TaskRejectReq{
		TaskId:   engine.NewID(123),
		Operator: "alice",
		Comment:  "",
	}
	err = dispatcher.Reject(context.Background(), invalidReject)
	require.Error(t, err)
	assert.ErrorIs(t, err, engine.ErrInvalidParameter)

	// 5. 校验防呆_TaskId为空
	emptyTaskReject := engine.TaskRejectReq{
		TaskId:   "",
		Operator: "alice",
		Comment:  "驳回",
	}
	err = dispatcher.Reject(context.Background(), emptyTaskReject)
	require.Error(t, err)
	assert.ErrorIs(t, err, engine.ErrInvalidParameter)
}
