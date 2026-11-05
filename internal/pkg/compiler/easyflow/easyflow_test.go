package easyflow

import (
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogicFlow_Deploy(t *testing.T) {
	testCases := []struct {
		name   string
		flow   map[string]any
		verify func(t *testing.T, nodes []model.Node)
	}{
		{
			name: "基础流程转换: Start -> User -> End",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "node_start", "type": "start", "properties": map[string]any{"name": "开始"}},
					map[string]any{"id": "node_user", "type": "user", "properties": map[string]any{"name": "审批", "approved": []string{"manager"}}},
					map[string]any{"id": "node_end", "type": "end", "properties": map[string]any{"name": "结束"}},
				},
				"edges": []any{
					map[string]any{"id": "edge1", "sourceNodeId": "node_start", "targetNodeId": "node_user"},
					map[string]any{"id": "edge2", "sourceNodeId": "node_user", "targetNodeId": "node_end"},
				},
			},
			verify: func(t *testing.T, nodes []model.Node) {
				require.Len(t, nodes, 3)

				startNode, _ := findNode(nodes, "node_start")
				require.NotNil(t, startNode)
				assert.Equal(t, model.NodeType(0), startNode.NodeType)

				userNode, _ := findNode(nodes, "node_user")
				require.NotNil(t, userNode)
				assert.Contains(t, userNode.UserIDs, "manager")

				endNode, _ := findNode(nodes, "node_end")
				require.NotNil(t, endNode)
				assert.Equal(t, model.NodeType(3), endNode.NodeType)
			},
		},
		{
			name: "复杂过程转换: 网关与代理节点",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "n1", "type": "start"},
					map[string]any{"id": "n2", "type": "condition", "properties": map[string]any{"name": "条件"}},
					map[string]any{"id": "n3", "type": "parallel", "properties": map[string]any{"name": "并行汇聚"}},
					map[string]any{"id": "n4", "type": "end"},
				},
				"edges": []any{
					map[string]any{"id": "e1", "sourceNodeId": "n1", "targetNodeId": "n2"},
					map[string]any{"id": "e2", "sourceNodeId": "n2", "targetNodeId": "n3", "properties": map[string]any{"expression": "a == 1"}},
					map[string]any{"id": "e3", "sourceNodeId": "n3", "targetNodeId": "n4"},
				},
			},
			verify: func(t *testing.T, nodes []model.Node) {
				// n1, n2, n3, n4 + 1个proxy节点
				assert.Len(t, nodes, 5)

				proxyID := "proxy_n2_n3"
				proxyNode, ok := findNode(nodes, proxyID)
				require.True(t, ok)
				assert.Equal(t, SysProxyNodeName, proxyNode.NodeName)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			converter := NewDefaultConverter()
			converter.Register(&StartNodeHandler{})
			converter.Register(&EndNodeHandler{})
			converter.Register(&UserNodeHandler{})
			converter.Register(&ParallelHandler{})
			converter.Register(&SelectiveHandler{})
			converter.Register(&ConditionHandler{})

			graph, err := flowdsl.Parse(tc.flow)
			require.NoError(t, err)

			process, err := converter.Convert(tc.name, graph)
			require.NoError(t, err)

			tc.verify(t, process.Nodes)
		})
	}
}

func findNode(nodes []model.Node, id string) (model.Node, bool) {
	for _, n := range nodes {
		if n.NodeID == id {
			return n, true
		}
	}
	return model.Node{}, false
}

func TestAutomationNodeHandlerAllowsOptionalDefaultRunner(t *testing.T) {
	testCases := []struct {
		name       string
		codebookID int64
		runnerID   int64
		wantErr    string
	}{
		{name: "配置默认执行单元", codebookID: 20, runnerID: 10},
		{name: "默认执行单元可以为空", codebookID: 20},
		{name: "拒绝未配置脚本文件", runnerID: 10, wantErr: "未配置有效的脚本文件"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			node := &flowdsl.Node{
				ID:   "automation-1",
				Name: "部署",
				Type: flowdsl.NodeAutoTask,
				AutoTask: &flowdsl.AutoTaskConfig{
					CodebookID: testCase.codebookID,
					RunnerID:   testCase.runnerID,
				},
			}
			_, err := (&AutomationNodeHandler{}).Handle(&Context{}, node)
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
		})
	}
}

func TestAutomationNodeHandlerValidatesCompensationNode(t *testing.T) {
	recovery := &flowdsl.Node{
		ID:   "recovery",
		Name: "权限回收",
		Type: flowdsl.NodeAutoTask,
		AutoTask: &flowdsl.AutoTaskConfig{
			CodebookID: 20,
			RunnerID:   2,
		},
	}
	testCases := []struct {
		name     string
		autoTask *flowdsl.AutoTaskConfig
		nodes    map[string]*flowdsl.Node
		wantErr  string
	}{
		{
			name: "授权节点关联补偿节点",
			autoTask: &flowdsl.AutoTaskConfig{
				CodebookID:         10,
				RunnerID:           1,
				CompensationNodeID: "recovery",
			},
			nodes: map[string]*flowdsl.Node{"recovery": recovery},
		},
		{
			name: "拒绝不存在的补偿节点",
			autoTask: &flowdsl.AutoTaskConfig{
				CodebookID:         10,
				RunnerID:           1,
				CompensationNodeID: "missing",
			},
			wantErr: "不存在或不是自动化节点",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := &Context{NodesMap: testCase.nodes}
			_, err := (&AutomationNodeHandler{}).Handle(ctx, &flowdsl.Node{
				ID:       "authorization",
				Name:     "权限授权",
				Type:     flowdsl.NodeAutoTask,
				AutoTask: testCase.autoTask,
			})
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
		})
	}
}
