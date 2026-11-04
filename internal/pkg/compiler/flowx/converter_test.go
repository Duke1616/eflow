package flowx

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlowxConverter(t *testing.T) {
	testCases := []struct {
		name       string
		procKey    string
		procName   string
		flow       domain.LogicFlow
		assertFunc func(t *testing.T, def model.ProcessDefinition)
		wantErr    bool
		errSubStr  string
	}{
		{
			name:     "标准串行审批流_转换并在引擎部署推进",
			procKey:  "linear_proc",
			procName: "标准串行审批流",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "start_1", "type": "start", "properties": map[string]any{"name": "提单"}},
					{
						"id":   "approval_1",
						"type": "user",
						"properties": map[string]any{
							"name":        "主管审批",
							"is_cosigned": false,
							"approved":    []string{"zhangsan", "lisi"},
						},
					},
					{"id": "end_1", "type": "end", "properties": map[string]any{"name": "归档办结"}},
				},
				Edges: []domain.FlowEdge{
					{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "approval_1"},
					{"id": "e2", "sourceNodeId": "approval_1", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				assert.Equal(t, "linear_proc", def.Key)
				assert.Len(t, def.Nodes, 3)
				assert.Len(t, def.Edges, 2)

				// 验证节点属性与或签规则
				approvalNode := def.Nodes[1]
				assert.Equal(t, model.NodeApproval, approvalNode.Type)
				assert.Equal(t, model.SignOneVote, approvalNode.SignRule)
				assert.ElementsMatch(t, []string{"zhangsan", "lisi"}, approvalNode.Assignees)

				// 引擎部署并在内存推进
				ctx := context.Background()
				engine := flowx.NewEngine(memory.NewStorage())
				require.NoError(t, engine.Deploy(ctx, def))

				instID, err := engine.Start(ctx, flowx.StartReq{Key: "linear_proc", Starter: "tom"})
				require.NoError(t, err)
				assert.NotEmpty(t, instID)
			},
		},
		{
			name:     "多维动态算人_规则规范化为URN并自动注入Hook",
			procKey:  "dynamic_proc",
			procName: "动态多规则流程",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "start_1", "type": "start"},
					{
						"id":   "approval_1",
						"type": "user",
						"properties": map[string]any{
							"name": "多规则审批",
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"admin"}},
								{"rule": "team", "values": []string{"sre_team"}},
								{"rule": "on_call", "values": []string{"db_duty"}},
								{"rule": "leaders", "values": []string{}},
								{"rule": "department", "values": []string{"sec_dept"}},
							},
						},
					},
					{"id": "end_1", "type": "end"},
				},
				Edges: []domain.FlowEdge{
					{"sourceNodeId": "start_1", "targetNodeId": "approval_1"},
					{"sourceNodeId": "approval_1", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				approvalNode := def.Nodes[1]
				assert.Equal(t, model.NodeApproval, approvalNode.Type)

				// 验证 URN 协议格式
				assert.Contains(t, approvalNode.Assignees, "admin")
				assert.Contains(t, approvalNode.Assignees, "team:sre_team")
				assert.Contains(t, approvalNode.Assignees, "oncall:db_duty")
				assert.Contains(t, approvalNode.Assignees, "leader:starter")
				assert.Contains(t, approvalNode.Assignees, "dept:sec_dept")

				// 验证自动挂载动态算人 Hook
				assert.Contains(t, approvalNode.Hooks, "dynamic_assignee_resolver")
			},
		},
		{
			name:     "条件排他网关_前缀表达式清洗并在引擎流转验证",
			procKey:  "condition_proc",
			procName: "条件排他审批流",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "start_1", "type": "start"},
					{"id": "node_check", "type": "user", "properties": map[string]any{"name": "初审", "approved": []string{"auditor"}}},
					{"id": "gw_1", "type": "selective"},
					{"id": "node_boss", "type": "user", "properties": map[string]any{"name": "总监特批", "is_cosigned": true, "approved": []string{"boss"}}},
					{"id": "end_1", "type": "end"},
				},
				Edges: []domain.FlowEdge{
					{"sourceNodeId": "start_1", "targetNodeId": "node_check"},
					{"sourceNodeId": "node_check", "targetNodeId": "gw_1"},
					{"sourceNodeId": "gw_1", "targetNodeId": "node_boss", "properties": map[string]any{"expression": "$amount >= 10000"}},
					{"sourceNodeId": "gw_1", "targetNodeId": "end_1", "properties": map[string]any{"expression": "$amount < 10000"}},
					{"sourceNodeId": "node_boss", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				// 网关节点类型
				assert.Equal(t, model.NodeGatewayExclusive, def.Nodes[2].Type)

				// 校验清洗后的条件表达式
				bossEdge := def.Edges[2]
				assert.Equal(t, "amount >= 10000", bossEdge.Condition)
				require.NotNil(t, bossEdge.RuleGroup)

				// 引擎内走小额分支实测
				ctx := context.Background()
				store := memory.NewStorage()
				engine := flowx.NewEngine(store)
				require.NoError(t, engine.Deploy(ctx, def))

				instID, err := engine.Start(ctx, flowx.StartReq{
					Key:       "condition_proc",
					Starter:   "jack",
					Variables: map[string]any{"amount": 5000},
				})
				require.NoError(t, err)

				timeline, err := engine.GetTimeline(ctx, instID)
				require.NoError(t, err)
				err = engine.Pass(ctx, flowx.PassReq{TaskID: timeline[0].Tasks[0].ID, Operator: "auditor"})
				require.NoError(t, err)

				inst, err := store.GetInstance(ctx, instID)
				require.NoError(t, err)
				assert.Equal(t, model.InstanceFinished, inst.Status)
				assert.Equal(t, "end_1", inst.CurrentNode)
			},
		},
		{
			name:     "并行分流与抄送_并行汇聚后抄送穿透办结",
			procKey:  "parallel_cc_proc",
			procName: "并行汇聚与抄送流程",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "start_1", "type": "start"},
					{"id": "gw_fork", "type": "parallel"},
					{"id": "node_sre", "type": "user", "properties": map[string]any{"name": "SRE审核", "approved": []string{"sre_user"}}},
					{"id": "node_dba", "type": "user", "properties": map[string]any{"name": "DBA审核", "approved": []string{"dba_user"}}},
					{"id": "gw_join", "type": "parallel"},
					{"id": "node_cc", "type": "user", "properties": map[string]any{"name": "抄送知会", "is_cc": true, "approved": []string{"notifier"}}},
					{"id": "end_1", "type": "end"},
				},
				Edges: []domain.FlowEdge{
					{"sourceNodeId": "start_1", "targetNodeId": "gw_fork"},
					{"sourceNodeId": "gw_fork", "targetNodeId": "node_sre"},
					{"sourceNodeId": "gw_fork", "targetNodeId": "node_dba"},
					{"sourceNodeId": "node_sre", "targetNodeId": "gw_join"},
					{"sourceNodeId": "node_dba", "targetNodeId": "gw_join"},
					{"sourceNodeId": "gw_join", "targetNodeId": "node_cc"},
					{"sourceNodeId": "node_cc", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				assert.Equal(t, model.NodeCarbonCopy, def.Nodes[5].Type)

				ctx := context.Background()
				store := memory.NewStorage()
				engine := flowx.NewEngine(store)
				require.NoError(t, engine.Deploy(ctx, def))

				instID, err := engine.Start(ctx, flowx.StartReq{Key: "parallel_cc_proc", Starter: "alice"})
				require.NoError(t, err)

				timeline, err := engine.GetTimeline(ctx, instID)
				require.NoError(t, err)
				for _, event := range timeline {
					for _, tsk := range event.Tasks {
						err = engine.Pass(ctx, flowx.PassReq{TaskID: tsk.ID, Operator: tsk.Assignee})
						require.NoError(t, err)
					}
				}

				inst, err := store.GetInstance(ctx, instID)
				require.NoError(t, err)
				assert.Equal(t, model.InstanceFinished, inst.Status)
				assert.Equal(t, "end_1", inst.CurrentNode)
			},
		},
		{
			name:     "自动化任务节点_正确转换WorkerType与配置属性",
			procKey:  "auto_proc",
			procName: "自动化流程",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "start_1", "type": "start"},
					{
						"id":   "auto_1",
						"type": "automation",
						"properties": map[string]any{
							"name":        "执行脚本",
							"codebook_id": int64(10086),
							"runner_id":   int64(2001),
						},
					},
					{"id": "end_1", "type": "end"},
				},
				Edges: []domain.FlowEdge{
					{"sourceNodeId": "start_1", "targetNodeId": "auto_1"},
					{"sourceNodeId": "auto_1", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				autoNode := def.Nodes[1]
				assert.Equal(t, model.NodeTask, autoNode.Type)
				assert.Equal(t, "automation", autoNode.WorkerType)
				assert.Equal(t, int64(10086), autoNode.Props["codebook_id"])
				assert.Equal(t, int64(2001), autoNode.Props["runner_id"])
			},
		},
		{
			name:      "异常校验_缺失流程Key报错",
			procKey:   "",
			wantErr:   true,
			errSubStr: "procKey) 不能为空",
		},
		{
			name:     "异常校验_未知节点类型拦截",
			procKey:  "bad_type_proc",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "n1", "type": "magic_node"},
				},
			},
			wantErr:   true,
			errSubStr: "不支持的画布节点类型: [magic_node]",
		},
		{
			name:     "异常校验_拓扑缺少开始节点拦截",
			procKey:  "no_start_proc",
			flow: domain.LogicFlow{
				Nodes: []domain.FlowNode{
					{"id": "end_1", "type": "end"},
				},
			},
			wantErr:   true,
			errSubStr: "流程必须有且仅有 1 个开始节点",
		},
	}

	converter := NewConverter()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			def, err := converter.Convert(tc.procKey, tc.procName, 1, tc.flow)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubStr != "" {
					assert.Contains(t, err.Error(), tc.errSubStr)
				}
				return
			}

			require.NoError(t, err)
			if tc.assertFunc != nil {
				tc.assertFunc(t, def)
			}
		})
	}
}
