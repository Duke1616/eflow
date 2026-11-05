package flowx

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
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
		flow       map[string]any
		nilGraph   bool
		assertFunc func(t *testing.T, def model.ProcessDefinition)
		wantErr    bool
		errSubStr  string
	}{
		{
			name:     "标准串行审批流_转换并在引擎部署推进",
			procKey:  "linear_proc",
			procName: "标准串行审批流",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "start_1", "type": "start", "properties": map[string]any{"name": "提单"}},
					map[string]any{
						"id":   "approval_1",
						"type": "user",
						"properties": map[string]any{
							"name":        "主管审批",
							"is_cosigned": false,
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"zhangsan", "lisi"}},
							},
						},
					},
					map[string]any{"id": "end_1", "type": "end", "properties": map[string]any{"name": "归档办结"}},
				},
				"edges": []any{
					map[string]any{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "approval_1"},
					map[string]any{"id": "e2", "sourceNodeId": "approval_1", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				assert.Equal(t, "linear_proc", def.Key)
				assert.Equal(t, "标准串行审批流", def.Name)
				assert.Len(t, def.Nodes, 3)
				assert.Len(t, def.Edges, 2)

				approvalNode := def.Nodes[1]
				assert.Equal(t, model.NodeApproval, approvalNode.Type)
				assert.ElementsMatch(t, []string{"zhangsan", "lisi"}, approvalNode.Assignees)
				assert.Equal(t, model.SignOneVote, approvalNode.SignRule)

				// 部署到 FlowX 引擎并执行验证
				ctx := context.Background()
				store := memory.NewStorage()
				engine := flowx.NewEngine(store)
				_, err := engine.Deploy(ctx, def)
				require.NoError(t, err)

				instID, err := engine.Start(ctx, flowx.StartReq{Key: "linear_proc", Starter: "zhangsan"})
				require.NoError(t, err)
				assert.NotEmpty(t, instID)
			},
		},
		{
			name:     "多维动态算人_规则规范化为URN并自动注入Hook",
			procKey:  "dynamic_proc",
			procName: "动态多规则流程",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "start_1", "type": "start"},
					map[string]any{
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
					map[string]any{"id": "end_1", "type": "end"},
				},
				"edges": []any{
					map[string]any{"sourceNodeId": "start_1", "targetNodeId": "approval_1"},
					map[string]any{"sourceNodeId": "approval_1", "targetNodeId": "end_1"},
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
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "start_1", "type": "start"},
					map[string]any{
						"id":   "node_check",
						"type": "user",
						"properties": map[string]any{
							"name": "初审",
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"auditor"}},
							},
						},
					},
					map[string]any{"id": "gw_1", "type": "selective"},
					map[string]any{
						"id":   "node_boss",
						"type": "user",
						"properties": map[string]any{
							"name":        "总监特批",
							"is_cosigned": true,
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"boss"}},
							},
						},
					},
					map[string]any{"id": "end_1", "type": "end"},
				},
				"edges": []any{
					map[string]any{"sourceNodeId": "start_1", "targetNodeId": "node_check"},
					map[string]any{"sourceNodeId": "node_check", "targetNodeId": "gw_1"},
					map[string]any{"sourceNodeId": "gw_1", "targetNodeId": "node_boss", "properties": map[string]any{"expression": "$amount >= 10000"}},
					map[string]any{"sourceNodeId": "gw_1", "targetNodeId": "end_1", "properties": map[string]any{"expression": "$amount < 10000"}},
					map[string]any{"sourceNodeId": "node_boss", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				assert.Len(t, def.Edges, 5)

				// 部署到 FlowX 引擎并执行验证
				ctx := context.Background()
				store := memory.NewStorage()
				engine := flowx.NewEngine(store)
				_, err := engine.Deploy(ctx, def)
				require.NoError(t, err)

				// 分支 1: 金额 5000 (< 10000)，直接办结跳过 node_boss
				instID1, err := engine.Start(ctx, flowx.StartReq{
					Key:       "condition_proc",
					Starter:   "alice",
					Variables: map[string]any{"amount": 5000},
				})
				require.NoError(t, err)

				timeline1, err := engine.GetTimeline(ctx, instID1)
				require.NoError(t, err)
				require.NotEmpty(t, timeline1)
				auditTask1 := timeline1[0].Tasks[0]

				err = engine.Pass(ctx, flowx.PassReq{TaskID: auditTask1.ID, Operator: "auditor"})
				require.NoError(t, err)

				inst1, err := store.GetInstance(ctx, instID1)
				require.NoError(t, err)
				assert.Equal(t, model.InstanceFinished, inst1.Status)
				assert.Equal(t, "end_1", inst1.CurrentNode)

				// 分支 2: 金额 20000 (>= 10000)，流向 node_boss
				instID2, err := engine.Start(ctx, flowx.StartReq{
					Key:       "condition_proc",
					Starter:   "bob",
					Variables: map[string]any{"amount": 20000},
				})
				require.NoError(t, err)

				timeline2, err := engine.GetTimeline(ctx, instID2)
				require.NoError(t, err)
				auditTask2 := timeline2[0].Tasks[0]

				err = engine.Pass(ctx, flowx.PassReq{TaskID: auditTask2.ID, Operator: "auditor"})
				require.NoError(t, err)

				inst2, err := store.GetInstance(ctx, instID2)
				require.NoError(t, err)
				assert.Equal(t, model.InstanceRunning, inst2.Status)
				assert.Equal(t, "node_boss", inst2.CurrentNode)
			},
		},
		{
			name:     "并行分流与抄送_并行汇聚后抄送穿透办结",
			procKey:  "parallel_cc_proc",
			procName: "并行汇聚与抄送流程",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "start_1", "type": "start"},
					map[string]any{"id": "gw_fork", "type": "parallel"},
					map[string]any{
						"id":   "node_sre",
						"type": "user",
						"properties": map[string]any{
							"name": "SRE审核",
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"sre_user"}},
							},
						},
					},
					map[string]any{
						"id":   "node_dba",
						"type": "user",
						"properties": map[string]any{
							"name": "DBA审核",
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"dba_user"}},
							},
						},
					},
					map[string]any{"id": "gw_join", "type": "parallel"},
					map[string]any{
						"id":   "node_cc",
						"type": "user",
						"properties": map[string]any{
							"name":  "抄送知会",
							"is_cc": true,
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"notifier"}},
							},
						},
					},
					map[string]any{"id": "end_1", "type": "end"},
				},
				"edges": []any{
					map[string]any{"sourceNodeId": "start_1", "targetNodeId": "gw_fork"},
					map[string]any{"sourceNodeId": "gw_fork", "targetNodeId": "node_sre"},
					map[string]any{"sourceNodeId": "gw_fork", "targetNodeId": "node_dba"},
					map[string]any{"sourceNodeId": "node_sre", "targetNodeId": "gw_join"},
					map[string]any{"sourceNodeId": "node_dba", "targetNodeId": "gw_join"},
					map[string]any{"sourceNodeId": "gw_join", "targetNodeId": "node_cc"},
					map[string]any{"sourceNodeId": "node_cc", "targetNodeId": "end_1"},
				},
			},
			assertFunc: func(t *testing.T, def model.ProcessDefinition) {
				assert.Equal(t, model.NodeCarbonCopy, def.Nodes[5].Type)

				ctx := context.Background()
				store := memory.NewStorage()
				engine := flowx.NewEngine(store)
				_, err := engine.Deploy(ctx, def)
				require.NoError(t, err)

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
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "start_1", "type": "start"},
					map[string]any{
						"id":   "auto_1",
						"type": "automation",
						"properties": map[string]any{
							"name":        "执行脚本",
							"codebook_id": int64(10086),
							"runner_id":   int64(2001),
						},
					},
					map[string]any{"id": "end_1", "type": "end"},
				},
				"edges": []any{
					map[string]any{"sourceNodeId": "start_1", "targetNodeId": "auto_1"},
					map[string]any{"sourceNodeId": "auto_1", "targetNodeId": "end_1"},
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
			flow:      map[string]any{"nodes": []any{}},
			wantErr:   true,
			errSubStr: "procKey) 不能为空",
		},
		{
			name:     "异常校验_未知节点类型拦截",
			procKey:  "bad_type_proc",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "n1", "type": "magic_node"},
				},
			},
			wantErr:   true,
			errSubStr: "不支持的画布节点类型: [magic_node]",
		},
		{
			name:     "异常校验_拓扑缺少开始节点拦截",
			procKey:  "no_start_proc",
			flow: map[string]any{
				"nodes": []any{
					map[string]any{"id": "end_1", "type": "end"},
				},
			},
			wantErr:   true,
			errSubStr: "流程必须有且仅有 1 个开始节点",
		},
		{
			name:      "异常校验_graph为nil拦截",
			procKey:   "nil_graph_proc",
			nilGraph:  true,
			wantErr:   true,
			errSubStr: "流程拓扑图 (graph) 不能为空",
		},
	}

	converter := NewConverter()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var graph flowdsl.IGraph
			if !tc.nilGraph {
				var err error
				graph, err = flowdsl.Parse(tc.flow)
				require.NoError(t, err)
			}

			def, err := converter.Convert(tc.procKey, tc.procName, graph)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errSubStr != "" {
					assert.Contains(t, err.Error(), tc.errSubStr)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, 0, def.Version) // 转换期版本默认置零，交由引擎发布期原子自增分配
			if tc.assertFunc != nil {
				tc.assertFunc(t, def)
			}
		})
	}
}

func TestFlowxConverter_AntV_X6(t *testing.T) {
	x6Canvas := map[string]any{
		"cells": []map[string]any{
			{
				"id":    "start_1",
				"shape": "start-node",
				"label": "发起",
			},
			{
				"id":    "approval_1",
				"shape": "user-task",
				"data": map[string]any{
					"name":        "经理审批",
					"is_cosigned": false,
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"manager"}},
					},
				},
			},
			{
				"id":    "end_1",
				"shape": "end-node",
				"label": "办结",
			},
			{
				"id":     "e1",
				"shape":  "edge",
				"source": map[string]any{"cell": "start_1"},
				"target": map[string]any{"cell": "approval_1"},
			},
			{
				"id":     "e2",
				"shape":  "edge",
				"source": "approval_1",
				"target": "end_1",
			},
		},
	}

	graph, err := flowdsl.Parse(x6Canvas)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("x6_proc", "X6流程测试", graph)
	require.NoError(t, err)
	assert.Equal(t, "x6_proc", def.Key)
	assert.Len(t, def.Nodes, 3)
	assert.Len(t, def.Edges, 2)

	// 部署到 FlowX 引擎并执行验证
	ctx := context.Background()
	engine := flowx.NewEngine(memory.NewStorage())
	_, err = engine.Deploy(ctx, def)
	require.NoError(t, err)

	instID, err := engine.Start(ctx, flowx.StartReq{Key: "x6_proc", Starter: "alice"})
	require.NoError(t, err)
	assert.NotEmpty(t, instID)

	timeline, err := engine.GetTimeline(ctx, instID)
	require.NoError(t, err)
	require.NotEmpty(t, timeline)
	assert.Equal(t, "manager", timeline[0].Tasks[0].Assignee)
}
