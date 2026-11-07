package flowx

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
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

func TestFlowxConverter_ChatGroupNode(t *testing.T) {
	flow := map[string]any{
		"nodes": []any{
			map[string]any{"id": "start_1", "type": "start", "properties": map[string]any{"name": "提单"}},
			map[string]any{
				"id":   "chat_1",
				"type": "chatGroup",
				"properties": map[string]any{
					"name":           "发布通知群",
					"mode":           "existing",
					"chat_group_ids": []int64{1001, 1002},
				},
			},
			map[string]any{
				"id":   "approval_1",
				"type": "user",
				"properties": map[string]any{
					"name": "主管审批",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"manager"}},
					},
				},
			},
			map[string]any{"id": "end_1", "type": "end", "properties": map[string]any{"name": "归档"}},
		},
		"edges": []any{
			map[string]any{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "chat_1"},
			map[string]any{"id": "e2", "sourceNodeId": "chat_1", "targetNodeId": "approval_1"},
			map[string]any{"id": "e3", "sourceNodeId": "approval_1", "targetNodeId": "end_1"},
		},
	}

	graph, err := flowdsl.Parse(flow)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("chat_proc", "群通知流程测试", graph)
	require.NoError(t, err)
	assert.Equal(t, "chat_proc", def.Key)
	assert.Len(t, def.Nodes, 4)

	chatNode := def.Nodes[1]
	assert.Equal(t, model.NodeChatGroup, chatNode.Type)
	assert.Equal(t, "发布通知群", chatNode.Name)

	// 部署到 FlowX 引擎：群聊节点瞬时穿透，直接流转到主管审批
	ctx := context.Background()
	engine := flowx.NewEngine(memory.NewStorage())
	_, err = engine.Deploy(ctx, def)
	require.NoError(t, err)

	instID, err := engine.Start(ctx, flowx.StartReq{Key: "chat_proc", Starter: "alice"})
	require.NoError(t, err)

	tasks, err := engine.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "approval_1", tasks[0].NodeID)
	assert.Equal(t, "manager", tasks[0].Assignee)
}

func TestFlowxConverter_ConditionNodeCompatibility(t *testing.T) {
	// 模拟 LogicFlow 画布：selective 网关 -> condition 条件节点 -> user 用户节点
	flow := map[string]any{
		"nodes": []any{
			map[string]any{"id": "start_1", "type": "start", "properties": map[string]any{"name": "开始"}},
			map[string]any{"id": "gw_fork", "type": "selective", "properties": map[string]any{"name": "排他分支"}},
			map[string]any{"id": "cond_backend", "type": "condition", "properties": map[string]any{"name": "后端条件"}},
			map[string]any{
				"id":   "user_backend",
				"type": "user",
				"properties": map[string]any{
					"name": "后端审批",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"dev_backend"}},
					},
				},
			},
			map[string]any{"id": "end_1", "type": "end", "properties": map[string]any{"name": "办结"}},
		},
		"edges": []any{
			map[string]any{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "gw_fork"},
			map[string]any{"id": "e2", "sourceNodeId": "gw_fork", "targetNodeId": "cond_backend"},
			map[string]any{
				"id":           "e3",
				"sourceNodeId": "cond_backend",
				"targetNodeId": "user_backend",
				"properties": map[string]any{
					"expression": "$environment in ('backend')",
					"name":       "后端",
				},
			},
			map[string]any{"id": "e4", "sourceNodeId": "user_backend", "targetNodeId": "end_1"},
		},
	}

	graph, err := flowdsl.Parse(flow)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("condition_compat_proc", "兼容条件节点测试", graph)
	require.NoError(t, err)
	assert.Len(t, def.Nodes, 5)

	// 断言 condition 节点被成功转换为包容/条件并行网关
	condNode, found := lo.Find(def.Nodes, func(n model.NodeSpec) bool { return n.ID == "cond_backend" })
	require.True(t, found)
	assert.Equal(t, model.NodeGatewayInclusive, condNode.Type)

	// 断言连线上的条件表达式被正确规范化为标准表达式
	condEdge, foundEdge := lo.Find(def.Edges, func(e model.EdgeSpec) bool { return e.Source == "cond_backend" && e.Target == "user_backend" })
	require.True(t, foundEdge)
	assert.Equal(t, "environment in ['backend']", condEdge.Condition)
	require.NotNil(t, condEdge.RuleGroup)

	// 部署到 FlowX 引擎验证
	engine := flowx.NewEngine(memory.NewStorage())
	_, err = engine.Deploy(context.Background(), def)
	require.NoError(t, err)

	// 启动流程并验证命中 in 包含规则
	instID, err := engine.Start(context.Background(), flowx.StartReq{
		Def:     def,
		Starter: "alice",
		Variables: map[string]any{
			"environment": `["ocr","frontend","backend"]`,
		},
	})
	require.NoError(t, err)

	tasks, err := engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)
	require.NotEmpty(t, tasks)
	// 验证 user_backend 待办任务已成功被包含算子激活生成！
	_, hasBackendTask := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "user_backend" })
	assert.True(t, hasBackendTask)
}

func TestFlowxConverter_FounderAssigneeResolution(t *testing.T) {
	// 验证配置了 rule=founder 的节点，转换为 model.AssigneeStarter，并在引擎启动后自动指派给发起人本人，无任何 Hook 依赖
	flow := map[string]any{
		"nodes": []any{
			map[string]any{"id": "start_1", "type": "start"},
			map[string]any{
				"id":   "submitter_cc",
				"type": "user",
				"properties": map[string]any{
					"name":  "抄送提交人",
					"is_cc": true,
					"rule":  "founder",
					"assignees": []map[string]any{
						{"rule": "founder", "values": []string{}},
					},
				},
			},
			map[string]any{"id": "end_1", "type": "end"},
		},
		"edges": []any{
			map[string]any{"sourceNodeId": "start_1", "targetNodeId": "submitter_cc"},
			map[string]any{"sourceNodeId": "submitter_cc", "targetNodeId": "end_1"},
		},
	}

	graph, err := flowdsl.Parse(flow)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("founder_proc", "发起人抄送测试", graph)
	require.NoError(t, err)

	ccNode := def.Nodes[1]
	assert.Equal(t, model.NodeCarbonCopy, ccNode.Type)
	assert.Equal(t, []string{model.AssigneeStarter}, ccNode.Assignees)
	assert.Empty(t, ccNode.Hooks) // 确保彻底移除了多余的魔法 Hook！

	// 部署到 FlowX 引擎并以 "luankz" 发起
	engine := flowx.NewEngine(memory.NewStorage())
	_, err = engine.Deploy(context.Background(), def)
	require.NoError(t, err)

	instID, err := engine.Start(context.Background(), flowx.StartReq{
		Key:     "founder_proc",
		Starter: "luankz",
	})
	require.NoError(t, err)

	// 验证抄送任务自动指派给 luankz
	tasks, err := engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "luankz", tasks[0].Assignee)
	assert.Equal(t, "submitter_cc", tasks[0].NodeID)
}

func TestFlowxConverter_MultiBranchInclusiveExecution(t *testing.T) {
	// 完整复刻用户产线发版画布拓扑：
	// start -> selective -> 3个condition (backend/frontend/ocr) -> 3个审批节点 -> parallel -> 产品审批 -> end
	flow := map[string]any{
		"nodes": []any{
			map[string]any{"id": "node_start", "type": "start", "text": map[string]any{"value": "开始"}},
			map[string]any{"id": "gw_split", "type": "selective", "text": map[string]any{"value": "包容条件分发"}},
			map[string]any{"id": "cond_backend", "type": "condition", "text": map[string]any{"value": "后端条件"}},
			map[string]any{"id": "cond_frontend", "type": "condition", "text": map[string]any{"value": "前端条件"}},
			map[string]any{"id": "cond_ocr", "type": "condition", "text": map[string]any{"value": "算法条件"}},
			map[string]any{"id": "user_backend", "type": "user", "text": map[string]any{"value": "后端"}, "properties": map[string]any{"assignees": []map[string]any{{"rule": "appoint", "values": []string{"peicg"}}}}},
			map[string]any{"id": "user_frontend", "type": "user", "text": map[string]any{"value": "前端"}, "properties": map[string]any{"assignees": []map[string]any{{"rule": "appoint", "values": []string{"luankz"}}}}},
			map[string]any{"id": "user_ocr", "type": "user", "text": map[string]any{"value": "算法"}, "properties": map[string]any{"assignees": []map[string]any{{"rule": "appoint", "values": []string{"alg_user"}}}}},
			map[string]any{"id": "gw_join", "type": "parallel", "text": map[string]any{"value": "并行汇聚"}},
			map[string]any{"id": "user_product", "type": "user", "text": map[string]any{"value": "产品"}, "properties": map[string]any{"assignees": []map[string]any{{"rule": "appoint", "values": []string{"product_lead"}}}}},
			map[string]any{"id": "node_end", "type": "end", "text": map[string]any{"value": "结束"}},
		},
		"edges": []any{
			map[string]any{"sourceNodeId": "node_start", "targetNodeId": "gw_split"},
			// selective 到 3 个条件节点
			map[string]any{"sourceNodeId": "gw_split", "targetNodeId": "cond_backend"},
			map[string]any{"sourceNodeId": "gw_split", "targetNodeId": "cond_frontend"},
			map[string]any{"sourceNodeId": "gw_split", "targetNodeId": "cond_ocr"},
			// 3 个条件节点带有包含规则，流向各自人工节点
			map[string]any{"sourceNodeId": "cond_backend", "targetNodeId": "user_backend", "properties": map[string]any{"expression": "environment in ('backend')"}},
			map[string]any{"sourceNodeId": "cond_frontend", "targetNodeId": "user_frontend", "properties": map[string]any{"expression": "environment in ('frontend')"}},
			map[string]any{"sourceNodeId": "cond_ocr", "targetNodeId": "user_ocr", "properties": map[string]any{"expression": "environment in ('ocr')"}},
			// 各自人工节点汇聚到 join 网关
			map[string]any{"sourceNodeId": "user_backend", "targetNodeId": "gw_join"},
			map[string]any{"sourceNodeId": "user_frontend", "targetNodeId": "gw_join"},
			map[string]any{"sourceNodeId": "user_ocr", "targetNodeId": "gw_join"},
			map[string]any{"sourceNodeId": "gw_join", "targetNodeId": "user_product"},
			map[string]any{"sourceNodeId": "user_product", "targetNodeId": "node_end"},
		},
	}

	graph, err := flowdsl.Parse(flow)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("deploy_pipeline", "多环境分发发版", graph)
	require.NoError(t, err)

	engine := flowx.NewEngine(memory.NewStorage())
	_, err = engine.Deploy(context.Background(), def)
	require.NoError(t, err)

	// 提单：同时选择 "backend" 和 "frontend" (未选 ocr)
	instID, err := engine.Start(context.Background(), flowx.StartReq{
		Def:     def,
		Starter: "luankz",
		Variables: map[string]any{
			"environment": `["frontend","backend"]`,
		},
	})
	require.NoError(t, err)

	// 验证：backend 和 frontend 两个待办任务均成功并发生成！
	tasks, err := engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)

	pendingBackend, hasBackend := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "user_backend" && t.IsPending() })
	pendingFrontend, hasFrontend := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "user_frontend" && t.IsPending() })
	_, hasOCR := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "user_ocr" && t.IsPending() })

	assert.True(t, hasBackend, "应当并发生成后端审批任务")
	assert.True(t, hasFrontend, "应当并发生成前端审批任务")
	assert.False(t, hasOCR, "未选中的算法任务不应处于待办状态")

	// 审批后端任务
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: pendingBackend.ID, Operator: "peicg"})
	require.NoError(t, err)

	// 审批前端任务
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: pendingFrontend.ID, Operator: "luankz"})
	require.NoError(t, err)

	// 打印所有任务状态帮助分析
	allTasks, err := engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)
	for _, tk := range allTasks {
		t.Logf("Task in DB: NodeID=%s, Name=%s, Assignee=%s, Status=%d", tk.NodeID, tk.NodeName, tk.Assignee, tk.Status)
	}

	instAfter, _ := engine.GetInstance(context.Background(), instID)
	t.Logf("Instance after: CurrentNode=%s, Status=%d", instAfter.CurrentNode, instAfter.Status)

	// 验证已顺利汇聚并激活下一阶段的“产品”审批任务！
	allTasks, err = engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)
	productTask, hasProduct := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "user_product" && t.IsPending() })
	assert.True(t, hasProduct, "汇聚网关应当放行并激活产品审批节点")

	// 产品审批通过直达办结
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: productTask.ID, Operator: "product_lead"})
	require.NoError(t, err)

	inst, err := engine.GetInstance(context.Background(), instID)
	require.NoError(t, err)
	assert.True(t, inst.IsFinished(), "流程应当最终顺利办结")
}


