package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/compiler"
	"github.com/Duke1616/eflow/internal/repository"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/internal/service/engine"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
	flowxAdapter "github.com/Duke1616/eflow/internal/service/engine/flowx"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestService_Find(t *testing.T) {
	testCases := []struct {
		name       string
		workflowID int64
		mock       func(ctrl *gomock.Controller) repository.IWorkflowRepository
		wantResult domain.Workflow
		wantErr    error
	}{
		{
			name:       "查询工作流成功",
			workflowID: 10,
			mock: func(ctrl *gomock.Controller) repository.IWorkflowRepository {
				repo := repomocks.NewMockIWorkflowRepository(ctrl)
				repo.EXPECT().Find(gomock.Any(), int64(10)).
					Return(domain.Workflow{
						Id:   10,
						Name: "资产审批流程",
					}, nil)
				return repo
			},
			wantResult: domain.Workflow{
				Id:   10,
				Name: "资产审批流程",
			},
			wantErr: nil,
		},
		{
			name:       "工作流不存在",
			workflowID: 99,
			mock: func(ctrl *gomock.Controller) repository.IWorkflowRepository {
				repo := repomocks.NewMockIWorkflowRepository(ctrl)
				repo.EXPECT().Find(gomock.Any(), int64(99)).
					Return(domain.Workflow{}, errors.New("not found"))
				return repo
			},
			wantResult: domain.Workflow{},
			wantErr:    errors.New("not found"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := tc.mock(ctrl)
			svc := NewWorkflowService(repo, nil, nil)

			res, err := svc.Find(context.Background(), tc.workflowID)
			if tc.wantErr != nil {
				assert.EqualError(t, err, tc.wantErr.Error())
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantResult, res)
			}
		})
	}
}

func TestService_Deploy(t *testing.T) {
	// 标准的审批流 LogicFlow 画布测试数据
	validFlowData := domain.CanvasData{
		"nodes": []any{
			map[string]any{
				"id":   "start_1",
				"type": "start",
				"properties": map[string]interface{}{
					"name": "发起流程",
				},
			},
			map[string]any{
				"id":   "approval_1",
				"type": "user",
				"properties": map[string]interface{}{
					"name":      "主管审批",
					"sign_rule": "all",
					"rule_type": "appoint",
					"users":     []interface{}{"user_admin"},
				},
			},
			map[string]any{
				"id":   "end_1",
				"type": "end",
				"properties": map[string]interface{}{
					"name": "流程结束",
				},
			},
		},
		"edges": []any{
			map[string]any{
				"id":           "edge_1",
				"sourceNodeId": "start_1",
				"targetNodeId": "approval_1",
			},
			map[string]any{
				"id":           "edge_2",
				"sourceNodeId": "approval_1",
				"targetNodeId": "end_1",
			},
		},
	}

	testCases := []struct {
		name        string
		workflow    domain.Workflow
		mockRepo    func(ctrl *gomock.Controller) repository.IWorkflowRepository
		mockAdapter func(ctrl *gomock.Controller) engine.IEngine
		wantErr     string
	}{
		{
			name: "flowx 引擎流程部署成功",
			workflow: domain.Workflow{
				Id:         1001,
				Name:       "Flowx请假审批",
				EngineType: compiler.EngineTypeFlowx,
				FlowData:   validFlowData,
			},
			mockRepo: func(ctrl *gomock.Controller) repository.IWorkflowRepository {
				repo := repomocks.NewMockIWorkflowRepository(ctrl)
				repo.EXPECT().UpdateProcessId(gomock.Any(), int64(1001), 1001).Return(nil)
				repo.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), 1001, 1).Return(nil)
				return repo
			},
			mockAdapter: func(ctrl *gomock.Controller) engine.IEngine {
				return nil
			},
			wantErr: "",
		},
		{
			name: "easy-workflow 引擎流程部署成功",
			workflow: domain.Workflow{
				Id:         2002,
				Name:       "Easyflow设备申请",
				EngineType: compiler.EngineTypeEasyFlow,
				FlowData:   validFlowData,
			},
			mockRepo: func(ctrl *gomock.Controller) repository.IWorkflowRepository {
				repo := repomocks.NewMockIWorkflowRepository(ctrl)
				repo.EXPECT().UpdateProcessId(gomock.Any(), int64(2002), 999).Return(nil)
				repo.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), 999, 1).Return(nil)
				return repo
			},
			mockAdapter: func(ctrl *gomock.Controller) engine.IEngine {
				adapter := enginemocks.NewMockIEngine(ctrl)
				adapter.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
				adapter.EXPECT().Deploy(gomock.Any(), domain.EngineTypeEasyFlow, gomock.AssignableToTypeOf(&model.Process{})).Return(domain.DeployInfo{ProcessID: 999, Version: 1}, nil)
				return adapter
			},
			wantErr: "",
		},
		{
			name: "默认引擎类型(空字符串)向下兼容为 easy-workflow",
			workflow: domain.Workflow{
				Id:         3003,
				Name:       "老数据未填引擎类型",
				EngineType: "",
				FlowData:   validFlowData,
			},
			mockRepo: func(ctrl *gomock.Controller) repository.IWorkflowRepository {
				repo := repomocks.NewMockIWorkflowRepository(ctrl)
				repo.EXPECT().UpdateProcessId(gomock.Any(), int64(3003), 888).Return(nil)
				repo.EXPECT().CreateSnapshot(gomock.Any(), gomock.Any(), 888, 2).Return(nil)
				return repo
			},
			mockAdapter: func(ctrl *gomock.Controller) engine.IEngine {
				adapter := enginemocks.NewMockIEngine(ctrl)
				adapter.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()
				adapter.EXPECT().Deploy(gomock.Any(), domain.EngineTypeEasyFlow, gomock.AssignableToTypeOf(&model.Process{})).Return(domain.DeployInfo{ProcessID: 888, Version: 2}, nil)
				return adapter
			},
			wantErr: "",
		},
		{
			name: "未知引擎类型报错拦截",
			workflow: domain.Workflow{
				Id:         4004,
				Name:       "未知引擎流程",
				EngineType: "unknown-engine",
				FlowData:   validFlowData,
			},
			mockRepo: func(ctrl *gomock.Controller) repository.IWorkflowRepository {
				return repomocks.NewMockIWorkflowRepository(ctrl)
			},
			mockAdapter: func(ctrl *gomock.Controller) engine.IEngine {
				return nil
			},
			wantErr: "不支持的流程引擎类型: unknown-engine",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := tc.mockRepo(ctrl)

			flowxEngine := flowx.NewEngine(memory.NewStorage())
			flowxDrv := flowxAdapter.NewEngine(flowxEngine, nil)

			adapters := []engine.IEngine{flowxDrv}
			if tc.mockAdapter != nil {
				if a := tc.mockAdapter(ctrl); a != nil {
					adapters = append(adapters, a)
				}
			}
			dispatcher := engine.NewEngineDispatcher(adapters...)
			compositeCompiler := compiler.NewDefaultCompositeCompiler()

			svc := NewWorkflowService(repo, dispatcher, compositeCompiler)
			err := svc.Deploy(context.Background(), tc.workflow)

			if tc.wantErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestService_GetHighlightedGraph(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := repomocks.NewMockIWorkflowRepository(ctrl)
	adapterMock := enginemocks.NewMockIEngine(ctrl)

	adapterMock.EXPECT().EngineType().Return(domain.EngineTypeEasyFlow).AnyTimes()

	// 1. Mock Find (GetHighlightedGraph + FindInstanceFlow 共调用 2 次)
	repo.EXPECT().Find(gomock.Any(), int64(10)).Return(domain.Workflow{
		Id: 10,
	}, nil).Times(2)

	// 2. Mock GetInstanceExecutionView
	adapterMock.EXPECT().GetInstanceExecutionView(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID(201), uint8(1)).Return(domain.InstanceExecutionView{
		ProcessID:      101,
		ProcessVersion: 1,
		TraversedEdges: map[string][]string{
			"start": {"audit"},
		},
		NodeStatusMap: map[string]int{
			"start": 1,
			"audit": 1,
		},
	}, nil)

	// 3. Mock FindSnapshotByWorkflowVersion (FindInstanceFlow 内部调用)
	repo.EXPECT().FindSnapshotByWorkflowVersion(gomock.Any(), int64(10), domain.EngineType(""), 1).Return(domain.Workflow{
		Id: 10,
		FlowData: domain.CanvasData{
			"nodes": []map[string]any{
				{"id": "start", "type": "start"},
				{"id": "audit", "type": "userTask"},
			},
			"edges": []map[string]any{
				{
					"id":           "e1",
					"sourceNodeId": "start",
					"targetNodeId": "audit",
					"properties":   map[string]any{},
				},
			},
		},
	}, nil)

	dispatcher := engine.NewEngineDispatcher(adapterMock)
	svc := NewWorkflowService(repo, dispatcher, nil)

	highlightedFlow, err := svc.GetHighlightedGraph(context.Background(), 10, domain.NewID(201), domain.Status(1))
	assert.NoError(t, err)

	edges, ok := highlightedFlow.FlowData["edges"].([]any)
	assert.True(t, ok)
	assert.Len(t, edges, 1)

	edgeMap := edges[0].(map[string]any)
	props := edgeMap["properties"].(map[string]any)
	assert.Equal(t, true, props["is_pass"])
}

func TestService_GetMaxSnapshotVersion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := repomocks.NewMockIWorkflowRepository(ctrl)
	repo.EXPECT().GetMaxSnapshotVersion(gomock.Any(), int64(44)).Return(3, nil)

	svc := NewWorkflowService(repo, nil, nil)
	ver, err := svc.GetMaxSnapshotVersion(context.Background(), 44)
	assert.NoError(t, err)
	assert.Equal(t, 3, ver)
}
