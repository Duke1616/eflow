package workflow

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
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

func TestService_GetAutomationProperty(t *testing.T) {
	svc := &workflowService{}

	testCases := []struct {
		name     string
		flow     easyflow.Workflow
		nodeID   string
		wantName string
		wantErr  string
	}{
		{
			name: "成功获取自动化节点属性",
			flow: easyflow.Workflow{
				FlowData: easyflow.LogicFlow{
					Nodes: []map[string]interface{}{
						{
							"id":   "auto-1",
							"type": easyflow.NodeTypeAuto,
							"properties": map[string]interface{}{
								"name":        "执行发布脚本",
								"codebook_id": float64(101),
							},
						},
					},
				},
			},
			nodeID:   "auto-1",
			wantName: "执行发布脚本",
			wantErr:  "",
		},
		{
			name: "节点不存在返回错误",
			flow: easyflow.Workflow{
				FlowData: easyflow.LogicFlow{
					Nodes: []map[string]interface{}{
						{
							"id":   "auto-1",
							"type": easyflow.NodeTypeAuto,
						},
					},
				},
			},
			nodeID:  "auto-not-exists",
			wantErr: "node not found",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			prop, err := svc.GetAutomationProperty(tc.flow, tc.nodeID)
			if tc.wantErr != "" {
				assert.EqualError(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tc.wantName, prop.Name)
			}
		})
	}
}
