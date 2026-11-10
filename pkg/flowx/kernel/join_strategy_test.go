package kernel

import (
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
)

func TestJoinStrategies(t *testing.T) {
	testCases := []struct {
		name      string
		nodeType  model.NodeType
		branches  []BranchState
		wantAllow bool
	}{
		{
			name:     "并行网关_全部分支通过_允许放行",
			nodeType: model.NodeGatewayParallel,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsPassed: true},
			},
			wantAllow: true,
		},
		{
			name:     "并行网关_存在未到达分支_阻断",
			nodeType: model.NodeGatewayParallel,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: false},
			},
			wantAllow: false,
		},
		{
			name:     "并行网关_存在待办未通过分支_阻断",
			nodeType: model.NodeGatewayParallel,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsPassed: false, IsSkipped: false},
			},
			wantAllow: false,
		},
		{
			name:     "并行网关_分支通过且另一分支被跳过_允许放行",
			nodeType: model.NodeGatewayParallel,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsSkipped: true},
			},
			wantAllow: true,
		},
		{
			name:     "并行网关_全部分支被跳过_防卫阻断",
			nodeType: model.NodeGatewayParallel,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsSkipped: true},
				{NodeID: "b", HasTask: true, IsSkipped: true},
			},
			wantAllow: false,
		},
		{
			name:     "包容网关_部分通过且部分跳过_允许放行",
			nodeType: model.NodeGatewayInclusive,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
				{NodeID: "b", HasTask: true, IsSkipped: true},
			},
			wantAllow: true,
		},
		{
			name:     "排他网关_直接放行",
			nodeType: model.NodeGatewayExclusive,
			branches: []BranchState{
				{NodeID: "a", HasTask: true, IsPassed: true},
			},
			wantAllow: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			strategy := GetJoinStrategy(tc.nodeType)
			got := strategy.IsSatisfied(tc.branches)
			assert.Equal(t, tc.wantAllow, got)
		})
	}
}
