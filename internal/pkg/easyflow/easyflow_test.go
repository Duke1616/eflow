package easyflow

import (
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLogicFlow_Deploy(t *testing.T) {
	testCases := []struct {
		name     string
		workflow Workflow
		verify   func(t *testing.T, nodes []model.Node)
	}{
		{
			name: "基础流程转换: Start -> User -> End",
			workflow: Workflow{
				Name:  "基础流程",
				Owner: "admin",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "node_start", "type": "start", "properties": map[string]interface{}{"name": "开始"}},
						{"id": "node_user", "type": "user", "properties": map[string]interface{}{"name": "审批", "approved": []string{"manager"}}},
						{"id": "node_end", "type": "end", "properties": map[string]interface{}{"name": "结束"}},
					},
					Edges: []map[string]interface{}{
						{"id": "edge1", "sourceNodeId": "node_start", "targetNodeId": "node_user"},
						{"id": "edge2", "sourceNodeId": "node_user", "targetNodeId": "node_end"},
					},
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
			workflow: Workflow{
				Name:  "网关流程",
				Owner: "admin",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n1", "type": "start"},
						{"id": "n2", "type": "condition", "properties": map[string]interface{}{"name": "条件"}},
						{"id": "n3", "type": "parallel", "properties": map[string]interface{}{"name": "并行汇聚"}},
						{"id": "n4", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "n1", "targetNodeId": "n2"},
						{"id": "e2", "sourceNodeId": "n2", "targetNodeId": "n3", "properties": map[string]interface{}{"expression": "a == 1"}},
						{"id": "e3", "sourceNodeId": "n3", "targetNodeId": "n4"},
					},
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

			process, err := converter.Convert(tc.workflow)
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

func TestAutomationNodeHandlerRejectsInvalidProperty(t *testing.T) {
	handler := &AutomationNodeHandler{}
	_, err := handler.Handle(&Context{}, Node{
		ID: "automation-1", Type: NodeTypeAuto,
		Properties: map[string]interface{}{"name": "部署", "runner_id": "invalid"},
	})

	require.ErrorContains(t, err, "解析自动化节点 automation-1 属性失败")
}

func TestAutomationNodeHandlerAllowsOptionalDefaultRunner(t *testing.T) {
	testCases := []struct {
		name       string
		codebookID any
		runnerID   any
		wantErr    string
	}{
		{name: "配置默认执行单元", codebookID: 20, runnerID: 10},
		{name: "默认执行单元可以为空", codebookID: 20},
		{name: "拒绝未配置脚本文件", runnerID: 10, wantErr: "未配置有效的脚本文件"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			properties := map[string]any{
				"name": "部署", "codebook_id": testCase.codebookID, "runner_id": testCase.runnerID,
			}
			_, err := (&AutomationNodeHandler{}).Handle(&Context{}, Node{
				ID: "automation-1", Type: NodeTypeAuto, Properties: properties,
			})
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
		})
	}
}

func TestAutomationNodeHandlerValidatesCompensationNode(t *testing.T) {
	recovery := Node{
		ID: "recovery", Type: NodeTypeAuto,
		Properties: map[string]any{
			"name": "权限回收", "codebook_id": 20, "runner_id": 2,
		},
	}
	testCases := []struct {
		name     string
		property map[string]any
		nodes    map[string]Node
		wantErr  string
	}{
		{
			name: "授权节点关联补偿节点",
			property: map[string]any{
				"name": "权限授权", "codebook_id": 10, "runner_id": 1, "compensation_node_id": "recovery",
			},
			nodes: map[string]Node{"recovery": recovery},
		},
		{
			name: "拒绝不存在的补偿节点",
			property: map[string]any{
				"name": "权限授权", "codebook_id": 10, "runner_id": 1, "compensation_node_id": "missing",
			},
			wantErr: "不存在或不是自动化节点",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			ctx := &Context{NodesMap: testCase.nodes}
			_, err := (&AutomationNodeHandler{}).Handle(ctx, Node{
				ID: "authorization", Type: NodeTypeAuto, Properties: testCase.property,
			})
			if testCase.wantErr == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, testCase.wantErr)
			}
		})
	}
}

func TestUserProperty_NormalizeAssignees(t *testing.T) {
	testCases := []struct {
		name     string
		property UserProperty
		want     []Assignee
	}{
		{
			name: "新版数据模式",
			property: UserProperty{
				Assignees: []Assignee{
					{Rule: APPOINT, Values: []string{"user1", "user2"}},
				},
			},
			want: []Assignee{
				{Rule: APPOINT, Values: []string{"user1", "user2"}},
			},
		},
		{
			name: "老版本模式-模板字段",
			property: UserProperty{
				Rule:          TEMPLATE,
				TemplateField: "manager",
			},
			want: []Assignee{
				{Rule: TEMPLATE, Values: []string{"manager"}},
			},
		},
		{
			name: "老版本模式-指定人",
			property: UserProperty{
				Rule:     APPOINT,
				Approved: []string{"user3"},
			},
			want: []Assignee{
				{Rule: APPOINT, Values: []string{"user3"}},
			},
		},
		{
			name: "老版本模式-缺省规则但有审批人",
			property: UserProperty{
				Approved: []string{"user4", "user5"},
			},
			want: []Assignee{
				{Rule: APPOINT, Values: []string{"user4", "user5"}},
			},
		},
		{
			name: "新版数据模式-缺省规则但有审批人",
			property: UserProperty{
				Assignees: []Assignee{
					{Values: []string{"user6", "user7"}},
				},
			},
			want: []Assignee{
				{Rule: APPOINT, Values: []string{"user6", "user7"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.property.NormalizeAssignees()
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUpdateEdgeProperties(t *testing.T) {
	testCases := []struct {
		name          string
		edges         []Edge
		edgeMap       map[string][]string
		nodeStatusMap map[string]int
		verify        func(t *testing.T, updated []Edge)
	}{
		{
			name: "普通节点跳过: Target 节点状态为 5 (已跳过)",
			edges: []Edge{
				{SourceNodeId: "user1", TargetNodeId: "user2", Properties: map[string]interface{}{}},
			},
			edgeMap: map[string][]string{
				"user1": {"user2"},
			},
			nodeStatusMap: map[string]int{
				"user1": 1,
				"user2": 5,
			},
			verify: func(t *testing.T, updated []Edge) {
				require.Len(t, updated, 1)
				props := updated[0].Properties.(map[string]interface{})
				assert.True(t, props["is_skipped"].(bool))
				assert.Nil(t, props["is_pass"])
			},
		},
		{
			name: "普通节点通过: 正常审批通过分支",
			edges: []Edge{
				{SourceNodeId: "user1", TargetNodeId: "user2", Properties: map[string]interface{}{}},
			},
			edgeMap: map[string][]string{
				"user1": {"user2"},
			},
			nodeStatusMap: map[string]int{
				"user1": 1,
				"user2": 1,
			},
			verify: func(t *testing.T, updated []Edge) {
				require.Len(t, updated, 1)
				props := updated[0].Properties.(map[string]interface{})
				assert.True(t, props["is_pass"].(bool))
				assert.Nil(t, props["is_skipped"])
			},
		},
		{
			name: "网关直连场景被跳过: 条件网关直连并行网关，proxy 节点状态为 5",
			edges: []Edge{
				{SourceNodeId: "gateway_cond", TargetNodeId: "gateway_parallel", Properties: map[string]interface{}{}},
			},
			edgeMap: map[string][]string{
				"gateway_cond": {"gateway_parallel"},
			},
			nodeStatusMap: map[string]int{
				// 网关节点本身不在任务表中，但底层生成的虚拟代理节点记录了跳过状态
				"proxy_gateway_cond_gateway_parallel": 5,
			},
			verify: func(t *testing.T, updated []Edge) {
				require.Len(t, updated, 1)
				props := updated[0].Properties.(map[string]interface{})
				assert.True(t, props["is_skipped"].(bool))
				assert.Nil(t, props["is_pass"])
			},
		},
		{
			name: "网关直连场景正常通过: 条件网关直连并行网关，proxy 节点状态为 1",
			edges: []Edge{
				{SourceNodeId: "gateway_cond", TargetNodeId: "gateway_parallel", Properties: map[string]interface{}{}},
			},
			edgeMap: map[string][]string{
				"gateway_cond": {"gateway_parallel"},
			},
			nodeStatusMap: map[string]int{
				// 虚拟代理节点正常执行流转完成
				"proxy_gateway_cond_gateway_parallel": 1,
			},
			verify: func(t *testing.T, updated []Edge) {
				require.Len(t, updated, 1)
				props := updated[0].Properties.(map[string]interface{})
				assert.True(t, props["is_pass"].(bool))
				assert.Nil(t, props["is_skipped"])
			},
		},
		{
			name: "未流转分支: 边未在 edgeMap 中激活",
			edges: []Edge{
				{SourceNodeId: "node_a", TargetNodeId: "node_b", Properties: map[string]interface{}{}},
			},
			edgeMap: map[string][]string{
				"node_a": {"node_c"},
			},
			nodeStatusMap: map[string]int{
				"node_a": 1,
				"node_b": 1,
			},
			verify: func(t *testing.T, updated []Edge) {
				require.Len(t, updated, 1)
				props := updated[0].Properties.(map[string]interface{})
				assert.Nil(t, props["is_pass"])
				assert.Nil(t, props["is_skipped"])
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			updated := UpdateEdgeProperties(tc.edges, tc.edgeMap, tc.nodeStatusMap)
			tc.verify(t, updated)
		})
	}
}

func TestToNodePropertyParsesSchedule(t *testing.T) {
	property, err := ToNodeProperty[AutomationProperty](Node{Properties: map[string]interface{}{
		"schedule": map[string]interface{}{
			"type": "at",
			"source": map[string]interface{}{
				"type": "template_field", "template_id": 12, "field": "execute_date", "time_field": "execute_time",
			},
			"timezone": "Asia/Shanghai",
		},
	}})

	require.NoError(t, err)
	require.NotNil(t, property.Schedule)
	assert.Equal(t, ScheduleAt, property.Schedule.Type)
	assert.Equal(t, int64(12), property.Schedule.Source.TemplateID)
	assert.Equal(t, "execute_date", property.Schedule.Source.Field)
	assert.Equal(t, "execute_time", property.Schedule.Source.TimeField)
}

func TestDeduplicateEdges(t *testing.T) {
	testCases := []struct {
		name     string
		edges    []Edge
		expected []Edge
	}{
		{
			name: "多条同源同目标连线去重保留一条",
			edges: []Edge{
				{ID: "e1", SourceNodeId: "nodeA", TargetNodeId: "nodeB"},
				{ID: "e2", SourceNodeId: "nodeA", TargetNodeId: "nodeB"},
			},
			expected: []Edge{
				{ID: "e1", SourceNodeId: "nodeA", TargetNodeId: "nodeB"},
			},
		},
		{
			name: "重复连线中优先保留包含有效表达式配置的连线",
			edges: []Edge{
				{ID: "e1", SourceNodeId: "cond", TargetNodeId: "user", Properties: map[string]interface{}{"expression": ""}},
				{ID: "e2", SourceNodeId: "cond", TargetNodeId: "user", Properties: map[string]interface{}{"expression": "$amount > 100"}},
			},
			expected: []Edge{
				{ID: "e2", SourceNodeId: "cond", TargetNodeId: "user", Properties: map[string]interface{}{"expression": "$amount > 100"}},
			},
		},
		{
			name: "多分支不同目标连线互不影响",
			edges: []Edge{
				{ID: "e1", SourceNodeId: "fork", TargetNodeId: "branch1"},
				{ID: "e2", SourceNodeId: "fork", TargetNodeId: "branch2"},
				{ID: "e3", SourceNodeId: "fork", TargetNodeId: "branch1"}, // 与 e1 重复
			},
			expected: []Edge{
				{ID: "e1", SourceNodeId: "fork", TargetNodeId: "branch1"},
				{ID: "e2", SourceNodeId: "fork", TargetNodeId: "branch2"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := deduplicateEdges(tc.edges)
			assert.Equal(t, tc.expected, got)
		})
	}
}
