package process

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/event"
	ticketSvc "github.com/Duke1616/eflow/internal/service/ticket"
	workflowSvc "github.com/Duke1616/eflow/internal/service/workflow"
	"github.com/gotomicro/ego/core/elog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubTicketService struct {
	ticketSvc.Service
	ticket domain.Ticket
	getErr error
}

func (s *stubTicketService) GetByID(_ context.Context, _ int64) (domain.Ticket, error) {
	return s.ticket, s.getErr
}

type stubWorkflowService struct {
	workflowSvc.Service
	findCalled bool
}

func (s *stubWorkflowService) Find(_ context.Context, _ int64) (domain.Workflow, error) {
	s.findCalled = true
	return domain.Workflow{}, nil
}

func TestProcessEventConsumer_HandleTask_Idempotent(t *testing.T) {
	testCases := []struct {
		name           string
		evt            event.TicketEvent
		ticketSvc      *stubTicketService
		wantFindCalled bool
		wantErr        bool
	}{
		{
			name: "工单已绑定流程实例时幂等跳过",
			evt:  event.TicketEvent{Id: 100, WorkflowId: 1},
			ticketSvc: &stubTicketService{
				ticket: domain.Ticket{
					Id: 100,
					Process: domain.Process{
						InstanceId: 9999, // 已有实例 ID
					},
				},
			},
			wantFindCalled: false,
			wantErr:        false,
		},
		{
			name: "查询工单信息失败时返回错误",
			evt:  event.TicketEvent{Id: 101, WorkflowId: 1},
			ticketSvc: &stubTicketService{
				getErr: errors.New("db error"),
			},
			wantFindCalled: false,
			wantErr:        true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wfStub := &stubWorkflowService{}
			consumer := &ProcessEventConsumer{
				workFlowSvc: wfStub,
				ticketSvc:   tc.ticketSvc,
				logger:      elog.DefaultLogger,
			}

			err := consumer.handleTask(context.Background(), tc.evt)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, tc.wantFindCalled, wfStub.findCalled)
		})
	}
}
