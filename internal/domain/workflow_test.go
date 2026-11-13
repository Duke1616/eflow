package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDefaultFlowxPolicy 默认策略四模式全开，历史未注入策略的工单统一走 AUTO 引擎决策
func TestDefaultFlowxPolicy(t *testing.T) {
	policy := DefaultFlowxPolicy()
	assert.True(t, policy.AllowReject)
	assert.False(t, policy.AllowTransfer)
	assert.Equal(t, []RejectMode{RejectModeStarter, RejectModeHistory, RejectModeTerminate}, policy.RejectModes)
	assert.True(t, policy.CanReturn())
	assert.True(t, policy.CanTerminate())
}

// TestResolveFlowPolicy 覆盖历史空数据兜底、未配置模式补全与 easyflow 强制降级
func TestResolveFlowPolicy(t *testing.T) {
	testCases := []struct {
		name      string
		workflow  Workflow
		wantModes []RejectMode
	}{
		{
			name:      "flowx零值策略_兜底四模式全开",
			workflow:  Workflow{EngineType: EngineTypeFlowx},
			wantModes: []RejectMode{RejectModeStarter, RejectModeHistory, RejectModeTerminate},
		},
		{
			name: "flowx开启驳回但未配置模式_补全四模式",
			workflow: Workflow{
				EngineType: EngineTypeFlowx,
				FlowPolicy: FlowPolicy{AllowReject: true},
			},
			wantModes: []RejectMode{RejectModeStarter, RejectModeHistory, RejectModeTerminate},
		},
		{
			name: "flowx显式收窄仅history_尊重配置不补全",
			workflow: Workflow{
				EngineType: EngineTypeFlowx,
				FlowPolicy: FlowPolicy{AllowReject: true, RejectModes: []RejectMode{RejectModeHistory}},
			},
			wantModes: []RejectMode{RejectModeHistory},
		},
		{
			name:      "easyflow零值策略_兜底仅否决",
			workflow:  Workflow{EngineType: EngineTypeEasyFlow},
			wantModes: []RejectMode{RejectModeTerminate},
		},
		{
			name: "easyflow显式回退模式_强制降级为仅否决",
			workflow: Workflow{
				EngineType: EngineTypeEasyFlow,
				FlowPolicy: FlowPolicy{AllowReject: true, RejectModes: []RejectMode{RejectModeStarter, RejectModeHistory}},
			},
			wantModes: []RejectMode{RejectModeTerminate},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantModes, tc.workflow.ResolveFlowPolicy().RejectModes)
		})
	}
}
