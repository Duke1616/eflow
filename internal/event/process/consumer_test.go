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
	ticket  domain.Ticket
	getErr  error
	bindErr error
	bindId  int
	bindKey string
}

func (s *stubTicketService) GetByID(_ context.Context, _ int64) (domain.Ticket, error) {
	return s.ticket, s.getErr
}

func (s *stubTicketService) BindProcess(_ context.Context, _ int64, engineType domain.EngineType, instanceID domain.ID) error {
	if engineType == domain.EngineTypeFlowx {
		s.bindKey = instanceID.String()
	} else {
		s.bindId = instanceID.Int()
	}
	return s.bindErr
}

func (s *stubTicketService) BindProcessInstanceID(_ context.Context, _ int64, instanceId int) error {
	s.bindId = instanceId
	return s.bindErr
}

func (s *stubTicketService) BindProcessInstanceKey(_ context.Context, _ int64, instanceKey string) error {
	s.bindKey = instanceKey
	return s.bindErr
}

type stubWorkflowService struct {
	workflowSvc.Service
	workflow   domain.Workflow
	findCalled bool
}

func (s *stubWorkflowService) Find(_ context.Context, _ int64) (domain.Workflow, error) {
	s.findCalled = true
	return s.workflow, nil
}

type stubLauncher struct {
	called bool
	req    domain.ProcessLaunchReq
	res    domain.ProcessLaunchResult
	err    error
}

func (s *stubLauncher) Start(_ context.Context, req domain.ProcessLaunchReq) (domain.ProcessLaunchResult, error) {
	s.called = true
	s.req = req
	return s.res, s.err
}

func TestProcessEventConsumer_HandleTask_Idempotent(t *testing.T) {
	testCases := []struct {
		name           string
		evt            event.TicketEvent
		ticketSvc      *stubTicketService
		workflow       domain.Workflow
		launcher       *stubLauncher
		wantFindCalled bool
		wantLaunch     bool
		wantBindId     int
		wantBindKey    string
		wantErr        bool
	}{
		{
			name: "工单已绑定流程实例时幂等跳过",
			evt:  event.TicketEvent{Id: 100, WorkflowId: 1},
			ticketSvc: &stubTicketService{
				ticket: domain.Ticket{
					Id: 100,
					Process: domain.Process{
						EngineType: domain.EngineTypeEasyFlow,
						InstanceID: domain.NewID(9999), // 已有实例 ID
					},
				},
			},
			wantFindCalled: false,
			wantLaunch:     false,
			wantErr:        false,
		},
		{
			name: "查询工单信息失败时返回错误",
			evt:  event.TicketEvent{Id: 101, WorkflowId: 1},
			ticketSvc: &stubTicketService{
				getErr: errors.New("db error"),
			},
			wantFindCalled: false,
			wantLaunch:     false,
			wantErr:        true,
		},
		{
			name: "未绑定实例时成功调度Launcher并发起",
			evt:  event.TicketEvent{Id: 102, WorkflowId: 88, Variables: `{"days":3}`},
			ticketSvc: &stubTicketService{
				ticket: domain.Ticket{
					Id:       102,
					CreateBy: "alex",
				},
			},
			workflow: domain.Workflow{
				Id:         88,
				Name:       "请假单",
				EngineType: domain.EngineTypeFlowx,
			},
			launcher: &stubLauncher{
				res: domain.ProcessLaunchResult{
					EngineType: domain.EngineTypeFlowx,
					InstanceID: domain.NewID("inst_102"),
				},
			},
			wantFindCalled: true,
			wantLaunch:     true,
			wantBindKey:    "inst_102",
			wantErr:        false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			wfStub := &stubWorkflowService{workflow: tc.workflow}
			launcher := tc.launcher
			if launcher == nil {
				launcher = &stubLauncher{}
			}
			consumer := &ProcessEventConsumer{
				launcher:    launcher,
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
			assert.Equal(t, tc.wantLaunch, launcher.called)
			if tc.wantBindId > 0 {
				assert.Equal(t, tc.wantBindId, tc.ticketSvc.bindId)
			}
			if tc.wantBindKey != "" {
				assert.Equal(t, tc.wantBindKey, tc.ticketSvc.bindKey)
			}
		})
	}
}
