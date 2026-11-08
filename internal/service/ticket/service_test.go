package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	repomocks "github.com/Duke1616/eflow/internal/repository/mocks"
	enginemocks "github.com/Duke1616/eflow/internal/service/engine/mocks"
	workflowmocks "github.com/Duke1616/eflow/internal/service/workflow/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xen0n/go-workwx"
	"go.uber.org/mock/gomock"
)

func TestTicketService_Pass(t *testing.T) {
	defaultEasyTicket := domain.Ticket{
		Id:         1001,
		WorkflowId: 10,
		Process: domain.Process{
			EngineType: domain.EngineTypeEasyFlow,
			InstanceID: domain.NewID(1),
		},
	}
	defaultFlowxTicket := domain.Ticket{
		Id:         999,
		WorkflowId: 20,
		Process: domain.Process{
			EngineType: domain.EngineTypeFlowx,
			InstanceID: domain.NewID("inst_flowx_123"),
		},
	}

	testCases := []struct {
		name      string
		ticketId  int64
		taskId    string
		operator  string
		isAdmin   bool
		comment   string
		extraData map[string]interface{}
		mock      func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService)
		wantErr   error
	}{
		{
			name:      "任务已被处理防御性拦截",
			ticketId:  1001,
			taskId:    "101",
			operator:  "auditor1",
			isAdmin:   false,
			comment:   "同意",
			extraData: nil,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("101")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("101"),
					ProcessInstanceID: domain.NewID(1),
					Assignee:          "auditor1",
					IsFinished:        true,
				}, nil)

				return repo, dispatcher, wfSvc
			},
			wantErr: ErrTaskAlreadyFinished,
		},
		{
			name:      "非指派人且非管理员拦截越权操作",
			ticketId:  1001,
			taskId:    "101",
			operator:  "attacker",
			isAdmin:   false,
			comment:   "同意",
			extraData: nil,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("101")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("101"),
					ProcessInstanceID: domain.NewID(1),
					Assignee:          "auditor1",
					IsFinished:        false,
				}, nil)

				return repo, dispatcher, wfSvc
			},
			wantErr: domain.ErrTaskUnauthorized,
		},
		{
			name:      "无表单字段时直接触发引擎通过",
			ticketId:  1001,
			taskId:    "102",
			operator:  "auditor1",
			isAdmin:   false,
			comment:   "同意通过",
			extraData: nil,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("102")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("102"),
					ProcessInstanceID: domain.NewID(1),
					ProcessID:         10,
					ProcessVersion:    1,
					NodeID:            "node-audit",
					Assignee:          "auditor1",
					IsFinished:        false,
				}, nil)

				wfSvc.EXPECT().FindInstanceFlow(gomock.Any(), int64(10), 10, 1).Return(domain.Workflow{
					FlowData: domain.CanvasData{
						"nodes": []any{
							map[string]any{
								"id":   "node-audit",
								"type": "user",
								"properties": map[string]interface{}{
									"fields": []interface{}{},
								},
							},
						},
					},
				}, nil)

				dispatcher.EXPECT().Pass(gomock.Any(), domain.TaskPassReq{
					EngineType: domain.EngineTypeEasyFlow,
					TaskId:     domain.NewID("102"),
					Operator:   "auditor1",
					Comment:    "同意通过",
				}).Return(nil)

				return repo, dispatcher, wfSvc
			},
			wantErr: nil,
		},
		{
			name:      "表单必填字段校验失败",
			ticketId:  1001,
			taskId:    "103",
			operator:  "auditor1",
			isAdmin:   false,
			comment:   "同意",
			extraData: map[string]interface{}{
				"reason": "",
			},
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("103")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("103"),
					ProcessInstanceID: domain.NewID(1),
					ProcessID:         10,
					ProcessVersion:    1,
					NodeID:            "node-audit",
					Assignee:          "auditor1",
					IsFinished:        false,
				}, nil)

				wfSvc.EXPECT().FindInstanceFlow(gomock.Any(), int64(10), 10, 1).Return(domain.Workflow{
					FlowData: domain.CanvasData{
						"nodes": []any{
							map[string]any{
								"id":   "node-audit",
								"type": "user",
								"properties": map[string]interface{}{
									"fields": []interface{}{
										map[string]interface{}{
											"name":     "审批原因",
											"key":      "reason",
											"type":     "input",
											"required": true,
										},
									},
								},
							},
						},
					},
				}, nil)

				return repo, dispatcher, wfSvc
			},
			wantErr: ValidationError,
		},
		{
			name:      "包含表单字段时成功合并数据、归档快照并推进流程",
			ticketId:  1001,
			taskId:    "104",
			operator:  "admin_user",
			isAdmin:   true,
			comment:   "通过",
			extraData: map[string]interface{}{
				"opinion": "审批通过，准予发布",
			},
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(domain.Ticket{
					Id:         888,
					WorkflowId: 10,
					Process: domain.Process{
						EngineType: domain.EngineTypeEasyFlow,
						InstanceID: domain.NewID(2),
					},
				}, nil).AnyTimes()

				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("104")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("104"),
					ProcessInstanceID: domain.NewID(2),
					ProcessID:         10,
					ProcessVersion:    1,
					NodeID:            "node-audit",
					Assignee:          "auditor1",
					IsFinished:        false,
				}, nil)

				wfSvc.EXPECT().FindInstanceFlow(gomock.Any(), int64(10), 10, 1).Return(domain.Workflow{
					FlowData: domain.CanvasData{
						"nodes": []any{
							map[string]any{
								"id":   "node-audit",
								"type": "user",
								"properties": map[string]interface{}{
									"fields": []interface{}{
										map[string]interface{}{
											"name":     "审核意见",
											"key":      "opinion",
											"type":     "input",
											"required": true,
											"merge":    true,
										},
									},
								},
							},
						},
					},
				}, nil)

				repo.EXPECT().MergeTicketData(gomock.Any(), int64(888), map[string]interface{}{
					"审核意见": "审批通过，准予发布",
				}).Return(nil)

				repo.EXPECT().CreateTaskForm(gomock.Any(), domain.NewID(104), int64(888), []domain.FormValue{
					{
						Name:  "审核意见",
						Key:   "opinion",
						Type:  "input",
						Value: "审批通过，准予发布",
					},
				}).Return(nil)

				dispatcher.EXPECT().Pass(gomock.Any(), domain.TaskPassReq{
					EngineType: domain.EngineTypeEasyFlow,
					TaskId:     domain.NewID("104"),
					Operator:   "admin_user",
					Comment:    "通过",
					Variables:  map[string]interface{}{"审核意见": "审批通过，准予发布"},
				}).Return(nil)

				return repo, dispatcher, wfSvc
			},
			wantErr: nil,
		},
		{
			name:      "Flowx 引擎任务通过且根据 InstanceKey 寻址工单完成数据合并",
			ticketId:  999,
			taskId:    "task_flowx_999",
			operator:  "flowx_auditor",
			isAdmin:   false,
			comment:   "flowx 审批通过",
			extraData: map[string]interface{}{
				"opinion": "同意",
			},
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				repo.EXPECT().Detail(gomock.Any(), int64(999)).Return(defaultFlowxTicket, nil).AnyTimes()

				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeFlowx, domain.NewID("task_flowx_999")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeFlowx,
					TaskID:            domain.NewID("task_flowx_999"),
					ProcessInstanceID: domain.NewID("inst_flowx_123"),
					ProcessID:         20,
					ProcessVersion:    1,
					NodeID:            "node-audit",
					Assignee:          "flowx_auditor",
					IsFinished:        false,
				}, nil)

				wfSvc.EXPECT().FindInstanceFlow(gomock.Any(), int64(20), 20, 1).Return(domain.Workflow{
					FlowData: domain.CanvasData{
						"nodes": []any{
							map[string]any{
								"id":   "node-audit",
								"type": "user",
								"properties": map[string]interface{}{
									"fields": []interface{}{
										map[string]interface{}{
											"name":     "审核意见",
											"key":      "opinion",
											"type":     "input",
											"required": true,
											"merge":    true,
										},
									},
								},
							},
						},
					},
				}, nil)

				repo.EXPECT().MergeTicketData(gomock.Any(), int64(999), map[string]interface{}{
					"审核意见": "同意",
				}).Return(nil)

				repo.EXPECT().CreateTaskForm(gomock.Any(), domain.NewID("task_flowx_999"), int64(999), []domain.FormValue{
					{
						Name:  "审核意见",
						Key:   "opinion",
						Type:  "input",
						Value: "同意",
					},
				}).Return(nil)

				dispatcher.EXPECT().Pass(gomock.Any(), domain.TaskPassReq{
					EngineType: domain.EngineTypeFlowx,
					TaskId:     domain.NewID("task_flowx_999"),
					Operator:   "flowx_auditor",
					Comment:    "flowx 审批通过",
					Variables:  map[string]interface{}{"审核意见": "同意"},
				}).Return(nil)

				return repo, dispatcher, wfSvc
			},
			wantErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo, dispatcherStub, wfStub := tc.mock(ctrl)
			svc := NewService(repo, nil, dispatcherStub, wfStub, nil)

			err := svc.Pass(context.Background(), tc.ticketId, tc.taskId, tc.operator, tc.isAdmin, tc.comment, tc.extraData)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTicketService_Reject(t *testing.T) {
	defaultEasyTicket := domain.Ticket{
		Id:         1001,
		WorkflowId: 10,
		Process: domain.Process{
			EngineType: domain.EngineTypeEasyFlow,
			InstanceID: domain.NewID(1),
		},
	}

	testCases := []struct {
		name     string
		taskId   string
		operator string
		isAdmin  bool
		comment  string
		mock     func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine)
		wantErr  error
	}{
		{
			name:     "任务已提前处理拦截驳回",
			taskId:   "201",
			operator: "auditor1",
			isAdmin:  false,
			comment:  "驳回",
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("201")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("201"),
					ProcessInstanceID: domain.NewID(1),
					Assignee:          "auditor1",
					IsFinished:        true,
				}, nil)
				return repo, dispatcher
			},
			wantErr: ErrTaskAlreadyFinished,
		},
		{
			name:     "未处理任务成功委托引擎执行驳回",
			taskId:   "202",
			operator: "auditor1",
			isAdmin:  false,
			comment:  "材料不齐，驳回修改",
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("202")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("202"),
					ProcessInstanceID: domain.NewID(1),
					Assignee:          "auditor1",
					IsFinished:        false,
				}, nil)
				dispatcher.EXPECT().Reject(gomock.Any(), domain.TaskRejectReq{
					EngineType: domain.EngineTypeEasyFlow,
					TaskId:     domain.NewID("202"),
					Operator:   "auditor1",
					Comment:    "材料不齐，驳回修改",
				}).Return(nil)
				return repo, dispatcher
			},
			wantErr: nil,
		},
		{
			name:     "越权拦截非指派人驳回",
			taskId:   "203",
			operator: "someone_else",
			isAdmin:  false,
			comment:  "恶意驳回",
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("203")).Return(domain.TaskContext{
					EngineType:        domain.EngineTypeEasyFlow,
					TaskID:            domain.NewID("203"),
					ProcessInstanceID: domain.NewID(1),
					Assignee:          "auditor1",
					IsFinished:        false,
				}, nil)
				return repo, dispatcher
			},
			wantErr: domain.ErrTaskUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo, dispatcherStub := tc.mock(ctrl)
			svc := NewService(repo, nil, dispatcherStub, nil, nil)

			err := svc.Reject(context.Background(), 1001, tc.taskId, tc.operator, tc.isAdmin, tc.comment)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTicketService_Transfer(t *testing.T) {
	defaultEasyTicket := domain.Ticket{
		Id:         1001,
		WorkflowId: 10,
		Process: domain.Process{
			EngineType: domain.EngineTypeEasyFlow,
			InstanceID: domain.NewID(1),
		},
	}

	testCases := []struct {
		name        string
		taskId      string
		operator    string
		isAdmin     bool
		targetUsers []string
		mock        func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine)
		wantErr     error
	}{
		{
			name:        "转签目标人为空时拦截",
			taskId:      "301",
			operator:    "auditor1",
			isAdmin:     false,
			targetUsers: nil,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				return repo, enginemocks.NewMockIEngine(ctrl)
			},
			wantErr: domain.ErrInvalidParameter,
		},
		{
			name:        "非指派人转签被越权拦截",
			taskId:      "302",
			operator:    "intruder",
			isAdmin:     false,
			targetUsers: []string{"bob"},
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("302")).Return(domain.TaskContext{
					EngineType: domain.EngineTypeEasyFlow,
					TaskID:     domain.NewID(302),
					Assignee:   "auditor1",
					IsFinished: false,
				}, nil)
				return repo, dispatcher
			},
			wantErr: domain.ErrTaskUnauthorized,
		},
		{
			name:        "合法指派人成功转签",
			taskId:      "303",
			operator:    "auditor1",
			isAdmin:     false,
			targetUsers: []string{"bob"},
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("303")).Return(domain.TaskContext{
					EngineType: domain.EngineTypeEasyFlow,
					TaskID:     domain.NewID(303),
					Assignee:   "auditor1",
					IsFinished: false,
				}, nil)
				dispatcher.EXPECT().Transfer(gomock.Any(), domain.TaskTransferReq{
					EngineType:  domain.EngineTypeEasyFlow,
					TaskId:      domain.NewID("303"),
					TargetUsers: []string{"bob"},
				}).Return(nil)
				return repo, dispatcher
			},
			wantErr: nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo, dispatcherStub := tc.mock(ctrl)
			svc := NewService(repo, nil, dispatcherStub, nil, nil)

			err := svc.Transfer(context.Background(), 1001, tc.taskId, tc.operator, tc.isAdmin, tc.targetUsers)
			if tc.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tc.wantErr))
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestTicketService_GetTaskFormConfig(t *testing.T) {
	defaultEasyTicket := domain.Ticket{
		Id:         1001,
		WorkflowId: 10,
		Process: domain.Process{
			EngineType: domain.EngineTypeEasyFlow,
			InstanceID: domain.NewID(1),
		},
	}

	testCases := []struct {
		name       string
		taskId     string
		workflowId int64
		mock       func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService)
		wantLen    int
		wantErr    bool
	}{
		{
			name:       "查询任务信息失败",
			taskId:     "301",
			workflowId: 10,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("301")).Return(domain.TaskContext{}, errors.New("task not found"))
				return repo, dispatcher, wfSvc
			},
			wantErr: true,
		},
		{
			name:       "未匹配到节点返回空配置列表",
			taskId:     "302",
			workflowId: 10,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("302")).Return(domain.TaskContext{
					TaskID:            domain.NewID("302"),
					ProcessInstanceID: domain.NewID(3),
					ProcessID:         10,
					ProcessVersion:    1,
					NodeID:            "node-non-exist",
				}, nil)

				wfSvc.EXPECT().FindInstanceFlow(gomock.Any(), int64(10), 10, 1).Return(domain.Workflow{
					FlowData: domain.CanvasData{
						"nodes": []any{
							map[string]any{
								"id":   "node-other",
								"type": "user",
							},
						},
					},
				}, nil)

				return repo, dispatcher, wfSvc
			},
			wantLen: 0,
			wantErr: false,
		},
		{
			name:       "成功匹配节点并返回表单字段定义",
			taskId:     "303",
			workflowId: 10,
			mock: func(ctrl *gomock.Controller) (*repomocks.MockTicketRepository, *enginemocks.MockIEngine, *workflowmocks.MockService) {
				repo := repomocks.NewMockTicketRepository(ctrl)
				repo.EXPECT().Detail(gomock.Any(), int64(1001)).Return(defaultEasyTicket, nil).AnyTimes()
				dispatcher := enginemocks.NewMockIEngine(ctrl)
				wfSvc := workflowmocks.NewMockService(ctrl)

				dispatcher.EXPECT().GetTaskContext(gomock.Any(), domain.EngineTypeEasyFlow, domain.NewID("303")).Return(domain.TaskContext{
					TaskID:            domain.NewID("303"),
					ProcessInstanceID: domain.NewID(4),
					ProcessID:         10,
					ProcessVersion:    1,
					NodeID:            "node-audit",
				}, nil)

				wfSvc.EXPECT().FindInstanceFlow(gomock.Any(), int64(10), 10, 1).Return(domain.Workflow{
					FlowData: domain.CanvasData{
						"nodes": []any{
							map[string]any{
								"id":   "node-audit",
								"type": "user",
								"properties": map[string]interface{}{
									"fields": []interface{}{
										map[string]interface{}{
											"name":     "服务器IP",
											"key":      "server_ip",
											"type":     "input",
											"required": true,
										},
									},
								},
							},
						},
					},
				}, nil)

				return repo, dispatcher, wfSvc
			},
			wantLen: 1,
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo, dispatcherStub, wfStub := tc.mock(ctrl)
			svc := NewService(repo, nil, dispatcherStub, wfStub, nil)

			fields, err := svc.GetTaskFormConfig(context.Background(), 1001, tc.taskId, tc.workflowId)
			if tc.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, fields, tc.wantLen)
			}
		})
	}
}

func TestConvert_Safe(t *testing.T) {
	// 验证企微审批数据转换：空选项、ID 无连字符等边界条件不触发 Panic
	jsonStr := `{
		"apply_data": {
			"contents": [
				{
					"id": "Selector-12345",
					"control": "Selector",
					"value": {
						"selector": {
							"type": "single",
							"options": []
						}
					}
				},
				{
					"id": "reason_no_hyphen",
					"control": "Textarea",
					"value": {
						"text": "审批原因"
					}
				},
				{
					"id": "Selector-multi",
					"control": "Selector",
					"value": {
						"selector": {
							"type": "multi",
							"options": [
								{
									"value": [{"text": "选项A"}]
								}
							]
						}
					}
				}
			]
		}
	}`

	var oaData workwx.OAApprovalDetail
	err := json.Unmarshal([]byte(jsonStr), &oaData)
	require.NoError(t, err)

	result := convert(nil, oaData)
	require.Len(t, result, 2)
	assert.Equal(t, "reason_no_hyphen", result[0].Key)
	assert.Equal(t, "审批原因", result[0].Value)
	assert.Equal(t, "multi", result[1].Key)
	assert.Equal(t, []string{"选项A"}, result[1].Value)
}

func TestFormatVariableValue(t *testing.T) {
	testCases := []struct {
		name     string
		input    any
		expected any
	}{
		{name: "nil 防 panic 返回空字符串", input: nil, expected: ""},
		{name: "float64 格式化为浮点文本", input: float64(123.45), expected: "123.450000"},
		{name: "字符串保持原样", input: "hello", expected: "hello"},
		{name: "整型保持原样", input: 42, expected: 42},
		{name: "切片转为 JSON 字符串", input: []any{"a", "b"}, expected: `["a","b"]`},
		{name: "字典转为 JSON 字符串", input: map[string]any{"k": "v"}, expected: `{"k":"v"}`},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, formatVariableValue(tc.input))
		})
	}
}
