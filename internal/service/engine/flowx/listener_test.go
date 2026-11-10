package flowx

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestEventListener_OnEvent(t *testing.T) {
	testCases := []struct {
		name       string
		event      flowx.Event
		setupMock  func(repo *repomocks.MockTicketRepository)
		wantErr    bool
		errMessage string
	}{
		{
			name: "流程正常办结_成功更新工单状态为END",
			event: flowx.Event{
				Type: flowx.EventProcessFinished,
				Instance: &flowxModel.Instance{
					ID: "inst_test_1001",
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository) {
				repo.EXPECT().
					UpdateStatusByProcessInstance(gomock.Any(), domain.NewID("inst_test_1001"), domain.END.ToUint8(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "流程驳回_成功更新工单状态为WITHDRAW",
			event: flowx.Event{
				Type: flowx.EventProcessRejected,
				Instance: &flowxModel.Instance{
					ID: "inst_test_1002",
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository) {
				repo.EXPECT().
					UpdateStatusByProcessInstance(gomock.Any(), domain.NewID("inst_test_1002"), domain.WITHDRAW.ToUint8(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "流程撤销_成功更新工单状态为WITHDRAW",
			event: flowx.Event{
				Type: flowx.EventProcessCancelled,
				Instance: &flowxModel.Instance{
					ID: "inst_test_1003",
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository) {
				repo.EXPECT().
					UpdateStatusByProcessInstance(gomock.Any(), domain.NewID("inst_test_1003"), domain.WITHDRAW.ToUint8(), gomock.Any()).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name: "更新状态返回错误_正确包装错误",
			event: flowx.Event{
				Type: flowx.EventProcessFinished,
				Instance: &flowxModel.Instance{
					ID: "inst_test_1004",
				},
			},
			setupMock: func(repo *repomocks.MockTicketRepository) {
				repo.EXPECT().
					UpdateStatusByProcessInstance(gomock.Any(), domain.NewID("inst_test_1004"), domain.END.ToUint8(), gomock.Any()).
					Return(errors.New("db error"))
			},
			wantErr:    true,
			errMessage: "更新工单办结状态失败",
		},
		{
			name: "空实例事件_安全忽略无panic",
			event: flowx.Event{
				Type:     flowx.EventProcessFinished,
				Instance: nil,
			},
			setupMock: func(repo *repomocks.MockTicketRepository) {},
			wantErr:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := repomocks.NewMockTicketRepository(ctrl)
			tc.setupMock(repo)

			listener := NewEventListener(repo)
			err := listener.OnEvent(context.Background(), tc.event)

			if tc.wantErr {
				assert.Error(t, err)
				if tc.errMessage != "" {
					assert.Contains(t, err.Error(), tc.errMessage)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestEventListener_InterestIn(t *testing.T) {
	listener := NewEventListener(nil)
	interests := listener.InterestIn()
	assert.Contains(t, interests, flowx.EventProcessFinished)
	assert.Contains(t, interests, flowx.EventProcessRejected)
	assert.Contains(t, interests, flowx.EventProcessCancelled)
}
