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
