package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStatus_AllowsProcessRestart(t *testing.T) {
	testCases := []struct {
		name       string
		status     Status
		wantResult bool
	}{
		{name: "START 允许重启", status: START, wantResult: true},
		{name: "START_FAILED 允许重启", status: START_FAILED, wantResult: true},
		{name: "PROCESS 不允许重启", status: PROCESS, wantResult: false},
		{name: "END 不允许重启", status: END, wantResult: false},
		{name: "WITHDRAW 不允许重启", status: WITHDRAW, wantResult: false},
		{name: "WITHDRAWING 不允许重启", status: WITHDRAWING, wantResult: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantResult, tc.status.AllowsProcessRestart())
		})
	}
}

func TestProcess_IsBound(t *testing.T) {
	testCases := []struct {
		name       string
		process    Process
		wantResult bool
	}{
		{name: "未绑定实例", process: Process{InstanceId: 0}, wantResult: false},
		{name: "已绑定有效实例", process: Process{InstanceId: 100}, wantResult: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantResult, tc.process.IsBound())
		})
	}
}

func TestTicket_CanRestartProcess(t *testing.T) {
	testCases := []struct {
		name       string
		ticket     Ticket
		wantResult bool
	}{
		{
			name: "启动失败且未绑定实例允许重启",
			ticket: Ticket{
				Status:  START_FAILED,
				Process: Process{InstanceId: 0},
			},
			wantResult: true,
		},
		{
			name: "启动中且未绑定实例允许重启",
			ticket: Ticket{
				Status:  START,
				Process: Process{InstanceId: 0},
			},
			wantResult: true,
		},
		{
			name: "已绑定流程实例不允许重启",
			ticket: Ticket{
				Status:  START_FAILED,
				Process: Process{InstanceId: 100},
			},
			wantResult: false,
		},
		{
			name: "审批中状态不允许重启",
			ticket: Ticket{
				Status:  PROCESS,
				Process: Process{InstanceId: 0},
			},
			wantResult: false,
		},
		{
			name: "已办结状态不允许重启",
			ticket: Ticket{
				Status:  END,
				Process: Process{InstanceId: 0},
			},
			wantResult: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantResult, tc.ticket.CanRestartProcess())
		})
	}
}

func TestStatus_IsTerminalAndCanRevoke(t *testing.T) {
	assert.True(t, END.IsTerminal())
	assert.True(t, WITHDRAW.IsTerminal())
	assert.False(t, PROCESS.IsTerminal())
	assert.False(t, START.IsTerminal())

	assert.True(t, START.CanRevoke())
	assert.True(t, PROCESS.CanRevoke())
	assert.False(t, END.CanRevoke())
	assert.False(t, WITHDRAW.CanRevoke())
	assert.False(t, WITHDRAWING.CanRevoke())
}

func TestTicket_CanRevokeBy(t *testing.T) {
	testCases := []struct {
		name       string
		ticket     Ticket
		operator   string
		isAdmin    bool
		wantErr    error
		wantErrMsg string
	}{
		{
			name:       "非本人且非管理员越权拦截",
			ticket:     Ticket{CreateBy: "alice", Status: PROCESS},
			operator:   "bob",
			isAdmin:    false,
			wantErr:    ErrTicketUnauthorized,
			wantErrMsg: "仅允许工单发起人或管理员撤回",
		},
		{
			name:       "撤回中状态阻止重复撤回",
			ticket:     Ticket{CreateBy: "alice", Status: WITHDRAWING},
			operator:   "alice",
			isAdmin:    false,
			wantErr:    ErrTicketWithdrawing,
			wantErrMsg: "不可重复发起撤回",
		},
		{
			name:       "已结束终态阻止撤回",
			ticket:     Ticket{CreateBy: "alice", Status: END},
			operator:   "alice",
			isAdmin:    false,
			wantErr:    ErrTicketCannotRevoke,
			wantErrMsg: "当前状态为 已结束",
		},
		{
			name:     "发起人允许撤回流转中工单",
			ticket:   Ticket{CreateBy: "alice", Status: PROCESS},
			operator: "alice",
			isAdmin:  false,
			wantErr:  nil,
		},
		{
			name:     "管理员允许撤回他人流转中工单",
			ticket:   Ticket{CreateBy: "alice", Status: PROCESS},
			operator: "admin",
			isAdmin:  true,
			wantErr:  nil,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.ticket.CanRevokeBy(tc.operator, tc.isAdmin)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				if tc.wantErrMsg != "" {
					assert.Contains(t, err.Error(), tc.wantErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestTicket_CanBeRatedBy(t *testing.T) {
	testCases := []struct {
		name         string
		ticket       Ticket
		rater        string
		alreadyRated bool
		wantResult   bool
	}{
		{
			name:         "已完结且本人未评价允许评分",
			ticket:       Ticket{Status: END, CreateBy: "alice"},
			rater:        "alice",
			alreadyRated: false,
			wantResult:   true,
		},
		{
			name:         "非已完结状态不允许评分",
			ticket:       Ticket{Status: PROCESS, CreateBy: "alice"},
			rater:        "alice",
			alreadyRated: false,
			wantResult:   false,
		},
		{
			name:         "他人不允许对工单评分",
			ticket:       Ticket{Status: END, CreateBy: "alice"},
			rater:        "bob",
			alreadyRated: false,
			wantResult:   false,
		},
		{
			name:         "已评价过的工单不允许重复评分",
			ticket:       Ticket{Status: END, CreateBy: "alice"},
			rater:        "alice",
			alreadyRated: true,
			wantResult:   false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantResult, tc.ticket.CanBeRatedBy(tc.rater, tc.alreadyRated))
		})
	}
}

func TestTicketTaskView_ResolveStep(t *testing.T) {
	t.Run("无待办步骤时自动回退为工单语义化状态", func(t *testing.T) {
		view := TicketTaskView{
			Ticket: Ticket{Status: WITHDRAW},
		}
		view.ResolveStep(nil, nil)
		assert.Equal(t, "已撤回", view.CurrentStep)
	})

	t.Run("有待办步骤时解析节点名与处理人姓名", func(t *testing.T) {
		view := TicketTaskView{
			Ticket: Ticket{Status: PROCESS},
		}
		steps := []Instance{
			{CurrentNodeName: "技术主管审批", ApprovedBy: "leader1", TaskID: 666},
		}
		displayNames := map[string]string{
			"leader1": "张主管",
		}
		view.ResolveStep(steps, displayNames)
		assert.Equal(t, "技术主管审批", view.CurrentStep)
		assert.Equal(t, "张主管", view.ApprovedBy)
		assert.Equal(t, 666, view.TaskID)
	})
}

func TestValidateTaskOperable(t *testing.T) {
	testCases := []struct {
		name       string
		assignee   string
		isFinished bool
		operator   string
		isAdmin    bool
		wantErr    error
	}{
		{
			name:       "普通指派人正常操作未完成任务",
			assignee:   "alice",
			isFinished: false,
			operator:   "alice",
			isAdmin:    false,
			wantErr:    nil,
		},
		{
			name:       "管理员代办他人未完成任务",
			assignee:   "alice",
			isFinished: false,
			operator:   "admin_user",
			isAdmin:    true,
			wantErr:    nil,
		},
		{
			name:       "已完结任务拦截无论是否管理员",
			assignee:   "alice",
			isFinished: true,
			operator:   "alice",
			isAdmin:    true,
			wantErr:    ErrTaskAlreadyFinished,
		},
		{
			name:       "非指派人非管理员拦截越权操作",
			assignee:   "alice",
			isFinished: false,
			operator:   "bob",
			isAdmin:    false,
			wantErr:    ErrTaskUnauthorized,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTaskOperable(tc.assignee, tc.isFinished, tc.operator, tc.isAdmin)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
