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
	// 包含已通过的群通知 Task 快照与待处理的主管审批任务
	require.Len(t, tasks, 2)
	chatTask, ok := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "chat_1" })
	require.True(t, ok)
	assert.Equal(t, model.TaskPassed, chatTask.Status)

	approvalTask, ok := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "approval_1" })
	require.True(t, ok)
	assert.Equal(t, "manager", approvalTask.Assignee)
	assert.Equal(t, model.TaskPending, approvalTask.Status)
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

	// 连线保留画布原文，条件在加载时编译成树；原文与规范化文本对解析器等价
	condEdge, foundEdge := lo.Find(def.Edges, func(e model.EdgeSpec) bool { return e.Source == "cond_backend" && e.Target == "user_backend" })
	require.True(t, foundEdge)
	assert.Equal(t, "$environment in ('backend')", condEdge.Condition)
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

// TestFlowxConverter_CompetitiveJoinSelectiveGateway 验证排他汇聚网关 (selective) 首到即走与兄弟分支待办自动取消
func TestFlowxConverter_CompetitiveJoinSelectiveGateway(t *testing.T) {
	testCases := []struct {
		name          string
		firstPassUser string // 率先通过的用户
		skippedUser   string // 期望被系统联动跳过的用户
	}{
		{
			name:          "技术人员率先审批通过，流程直接办结且人事待办自动跳过",
			firstPassUser: "tech_user",
			skippedUser:   "hr_user",
		},
		{
			name:          "人事人员率先审批通过，流程直接办结且技术待办自动跳过",
			firstPassUser: "hr_user",
			skippedUser:   "tech_user",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			flow := map[string]any{
				"nodes": []any{
					map[string]any{"id": "start_1", "type": "start", "properties": map[string]any{"name": "开始"}},
					map[string]any{"id": "gw_fork", "type": "inclusion", "properties": map[string]any{"name": "包容分流"}},
					map[string]any{
						"id":   "user_tech",
						"type": "user",
						"properties": map[string]any{
							"name": "技术",
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"tech_user"}},
							},
						},
					},
					map[string]any{
						"id":   "user_hr",
						"type": "user",
						"properties": map[string]any{
							"name": "人事",
							"assignees": []map[string]any{
								{"rule": "appoint", "values": []string{"hr_user"}},
							},
						},
					},
					map[string]any{"id": "gw_join", "type": "selective", "properties": map[string]any{"name": "分支合并"}},
					map[string]any{"id": "end_1", "type": "end", "properties": map[string]any{"name": "办结"}},
				},
				"edges": []any{
					map[string]any{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "gw_fork"},
					map[string]any{"id": "e2", "sourceNodeId": "gw_fork", "targetNodeId": "user_tech"},
					map[string]any{"id": "e3", "sourceNodeId": "gw_fork", "targetNodeId": "user_hr"},
					map[string]any{"id": "e4", "sourceNodeId": "user_tech", "targetNodeId": "gw_join"},
					map[string]any{"id": "e5", "sourceNodeId": "user_hr", "targetNodeId": "gw_join"},
					map[string]any{"id": "e6", "sourceNodeId": "gw_join", "targetNodeId": "end_1"},
				},
			}

			graph, err := flowdsl.Parse(flow)
			require.NoError(t, err)

			converter := NewConverter()
			def, err := converter.Convert("competitive_join_proc", "竞争汇聚测试", graph)
			require.NoError(t, err)

			// 1. 断言作为多路汇聚点的 selective 节点被正确编译为 NodeGatewayExclusive (排他直通网关)
			joinNode, found := lo.Find(def.Nodes, func(n model.NodeSpec) bool { return n.ID == "gw_join" })
			require.True(t, found)
			assert.Equal(t, model.NodeGatewayExclusive, joinNode.Type, "作为汇聚点的 selective 网关必须编译为排他汇聚")

			// 2. 部署并启动 FlowX 流程实例
			engine := flowx.NewEngine(memory.NewStorage())
			_, err = engine.Deploy(context.Background(), def)
			require.NoError(t, err)

			instID, err := engine.Start(context.Background(), flowx.StartReq{
				Def:     def,
				Starter: "starter_user",
			})
			require.NoError(t, err)

			tasks, err := engine.ListTasksByInstance(context.Background(), instID)
			require.NoError(t, err)
			assert.Len(t, tasks, 2, "分流网关应同时激活技术和人事两个待办任务")

			firstTask, foundFirst := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == tc.firstPassUser })
			require.True(t, foundFirst)
			assert.True(t, firstTask.IsPending())

			// 3. 率先审批通过
			err = engine.Pass(context.Background(), flowx.PassReq{TaskID: firstTask.ID, Operator: tc.firstPassUser})
			require.NoError(t, err)

			// 4. 验证流程实例直接圆满办结
			inst, err := engine.GetInstance(context.Background(), instID)
			require.NoError(t, err)
			assert.True(t, inst.IsFinished(), "排他汇聚首到即走，流程应当直接办结")

			// 5. 验证竞争失败的兄弟分支任务被自动标记为 TaskSkipped
			allTasks, err := engine.ListTasksByInstance(context.Background(), instID)
			require.NoError(t, err)

			skippedTask, foundSkipped := lo.Find(allTasks, func(t *model.Task) bool { return t.Assignee == tc.skippedUser })
			require.True(t, foundSkipped)
			assert.Equal(t, model.TaskSkipped, skippedTask.Status, "兄弟未决分支任务应当被联动跳过")

			// 6. 验证已跳过任务禁止重复审批
			err = engine.Pass(context.Background(), flowx.PassReq{TaskID: skippedTask.ID, Operator: tc.skippedUser})
			assert.ErrorIs(t, err, flowx.ErrTaskNotPending, "已被跳过的任务不允许再次审批")
		})
	}
}

// TestFlowxConverter_BossOrProductsCosigned 验证进阶场景：老板 1 人通过 OR 两个产品会签全部通过
func TestFlowxConverter_BossOrProductsCosigned(t *testing.T) {
	// 拓扑图：开始 -> 包容分流 -> [老板审批(单人)] & [产品审批(产品A+B会签)] -> 排他汇聚 -> 结束
	flow := map[string]any{
		"nodes": []any{
			map[string]any{"id": "start_1", "type": "start", "properties": map[string]any{"name": "开始"}},
			map[string]any{"id": "gw_fork", "type": "inclusion", "properties": map[string]any{"name": "分流"}},
			map[string]any{
				"id":   "user_boss",
				"type": "user",
				"properties": map[string]any{
					"name": "老板审批",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"boss"}},
					},
				},
			},
			map[string]any{
				"id":   "user_pm",
				"type": "user",
				"properties": map[string]any{
					"name": "产品会签",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"pm_a", "pm_b"}},
					},
					"is_cosigned": true, // 会签：产品 A 和 B 必须全部通过才算产品分支完成
				},
			},
			map[string]any{"id": "gw_join", "type": "selective", "properties": map[string]any{"name": "合并网关"}},
			map[string]any{"id": "end_1", "type": "end", "properties": map[string]any{"name": "办结"}},
		},
		"edges": []any{
			map[string]any{"id": "e1", "sourceNodeId": "start_1", "targetNodeId": "gw_fork"},
			map[string]any{"id": "e2", "sourceNodeId": "gw_fork", "targetNodeId": "user_boss"},
			map[string]any{"id": "e3", "sourceNodeId": "gw_fork", "targetNodeId": "user_pm"},
			map[string]any{"id": "e4", "sourceNodeId": "user_boss", "targetNodeId": "gw_join"},
			map[string]any{"id": "e5", "sourceNodeId": "user_pm", "targetNodeId": "gw_join"},
			map[string]any{"id": "e6", "sourceNodeId": "gw_join", "targetNodeId": "end_1"},
		},
	}

	graph, err := flowdsl.Parse(flow)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("boss_or_pm_proc", "老板与产品竞争测试", graph)
	require.NoError(t, err)

	t.Run("场景 1：老板直接通过，两个产品的会签待办全部自动跳过，工单办结", func(t *testing.T) {
		engine := flowx.NewEngine(memory.NewStorage())
		_, err := engine.Deploy(context.Background(), def)
		require.NoError(t, err)

		instID, err := engine.Start(context.Background(), flowx.StartReq{Def: def, Starter: "alice"})
		require.NoError(t, err)

		tasks, _ := engine.ListTasksByInstance(context.Background(), instID)
		bossTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "boss" })

		// 老板审批
		err = engine.Pass(context.Background(), flowx.PassReq{TaskID: bossTask.ID, Operator: "boss"})
		require.NoError(t, err)

		inst, _ := engine.GetInstance(context.Background(), instID)
		assert.True(t, inst.IsFinished(), "老板通过，流程立即办结")

		allTasks, _ := engine.ListTasksByInstance(context.Background(), instID)
		pmTasks := lo.Filter(allTasks, func(t *model.Task, _ int) bool { return t.NodeID == "user_pm" })
		assert.Len(t, pmTasks, 2)
		for _, pt := range pmTasks {
			assert.Equal(t, model.TaskSkipped, pt.Status, "两个产品的待办必须全被跳过")
		}
	})

	t.Run("场景 2：产品 A 先通过但产品 B 未通过时保持等待；老板随后通过，产品 B 自动跳过并办结", func(t *testing.T) {
		engine := flowx.NewEngine(memory.NewStorage())
		_, err := engine.Deploy(context.Background(), def)
		require.NoError(t, err)

		instID, err := engine.Start(context.Background(), flowx.StartReq{Def: def, Starter: "alice"})
		require.NoError(t, err)

		tasks, _ := engine.ListTasksByInstance(context.Background(), instID)
		pmATask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "pm_a" })
		bossTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "boss" })

		// 产品 A 审批通过
		err = engine.Pass(context.Background(), flowx.PassReq{TaskID: pmATask.ID, Operator: "pm_a"})
		require.NoError(t, err)

		// 此时产品会签尚未全部完成，流程保持在运行中
		inst, _ := engine.GetInstance(context.Background(), instID)
		assert.Equal(t, model.InstanceRunning, inst.Status)

		// 老板随后通过
		err = engine.Pass(context.Background(), flowx.PassReq{TaskID: bossTask.ID, Operator: "boss"})
		require.NoError(t, err)

		inst, _ = engine.GetInstance(context.Background(), instID)
		assert.True(t, inst.IsFinished(), "老板通过后，流程直接办结")

		allTasks, _ := engine.ListTasksByInstance(context.Background(), instID)
		pmBTask, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.Assignee == "pm_b" })
		assert.Equal(t, model.TaskSkipped, pmBTask.Status, "产品 B 待办必须被自动跳过")
	})

	t.Run("场景 3：老板没审，产品 A 和产品 B 均通过，达成条件顺利办结，老板待办自动跳过", func(t *testing.T) {
		engine := flowx.NewEngine(memory.NewStorage())
		_, err := engine.Deploy(context.Background(), def)
		require.NoError(t, err)

		instID, err := engine.Start(context.Background(), flowx.StartReq{Def: def, Starter: "alice"})
		require.NoError(t, err)

		tasks, _ := engine.ListTasksByInstance(context.Background(), instID)
		pmATask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "pm_a" })
		pmBTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "pm_b" })

		// 产品 A 和 B 均通过
		_ = engine.Pass(context.Background(), flowx.PassReq{TaskID: pmATask.ID, Operator: "pm_a"})
		err = engine.Pass(context.Background(), flowx.PassReq{TaskID: pmBTask.ID, Operator: "pm_b"})
		require.NoError(t, err)

		inst, _ := engine.GetInstance(context.Background(), instID)
		assert.True(t, inst.IsFinished(), "产品会签全过，流程办结")

		allTasks, _ := engine.ListTasksByInstance(context.Background(), instID)
		bossTask, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.Assignee == "boss" })
		assert.Equal(t, model.TaskSkipped, bossTask.Status, "老板的待办必须被自动跳过")
	})
}

// TestFlowxConverter_ComplexRejectMultiBranch 还原工单 2772 拓扑：领导驳回时完整退回上游多路分支(前端+后端)，且流程图高亮准确
func TestFlowxConverter_ComplexRejectMultiBranch(t *testing.T) {
	// 拓扑：开始 -> 抄送提交人 -> 分流网关 -> (后端 & 前端) -> 并行汇聚网关 -> (产品 & 领导) -> 结束
	flow := map[string]any{
		"nodes": []any{
			map[string]any{"id": "node_start", "type": "start", "properties": map[string]any{"name": "开始"}},
			map[string]any{"id": "node_cc", "type": "user", "properties": map[string]any{"name": "抄送提交人", "is_cc": true}},
			map[string]any{"id": "gw_fork", "type": "inclusion", "properties": map[string]any{"name": "分流"}},
			map[string]any{
				"id":   "node_backend",
				"type": "user",
				"properties": map[string]any{
					"name": "后端",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"peicg"}},
					},
				},
			},
			map[string]any{
				"id":   "node_frontend",
				"type": "user",
				"properties": map[string]any{
					"name": "前端",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"luankz"}},
					},
				},
			},
			map[string]any{"id": "gw_join", "type": "parallel", "properties": map[string]any{"name": "并行汇聚"}},
			map[string]any{
				"id":   "node_product",
				"type": "user",
				"properties": map[string]any{
					"name": "产品",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"langtt"}},
					},
				},
			},
			map[string]any{
				"id":   "node_leader",
				"type": "user",
				"properties": map[string]any{
					"name": "领导",
					"assignees": []map[string]any{
						{"rule": "appoint", "values": []string{"boss_luankz"}},
					},
				},
			},
			map[string]any{"id": "gw_join2", "type": "parallel", "properties": map[string]any{"name": "二级并行汇聚"}},
			map[string]any{"id": "node_end", "type": "end", "properties": map[string]any{"name": "办结"}},
		},
		"edges": []any{
			map[string]any{"id": "e1", "sourceNodeId": "node_start", "targetNodeId": "node_cc"},
			map[string]any{"id": "e2", "sourceNodeId": "node_cc", "targetNodeId": "gw_fork"},
			map[string]any{"id": "e3", "sourceNodeId": "gw_fork", "targetNodeId": "node_backend"},
			map[string]any{"id": "e4", "sourceNodeId": "gw_fork", "targetNodeId": "node_frontend"},
			map[string]any{"id": "e5", "sourceNodeId": "node_backend", "targetNodeId": "gw_join"},
			map[string]any{"id": "e6", "sourceNodeId": "node_frontend", "targetNodeId": "gw_join"},
			map[string]any{"id": "e7", "sourceNodeId": "gw_join", "targetNodeId": "node_product"},
			map[string]any{"id": "e8", "sourceNodeId": "gw_join", "targetNodeId": "node_leader"},
			map[string]any{"id": "e9", "sourceNodeId": "node_product", "targetNodeId": "gw_join2"},
			map[string]any{"id": "e10", "sourceNodeId": "node_leader", "targetNodeId": "gw_join2"},
			map[string]any{"id": "e11", "sourceNodeId": "gw_join2", "targetNodeId": "node_end"},
		},
	}

	graph, err := flowdsl.Parse(flow)
	require.NoError(t, err)

	converter := NewConverter()
	def, err := converter.Convert("complex_reject_proc", "多分支驳回测试", graph)
	require.NoError(t, err)

	engine := flowx.NewEngine(memory.NewStorage())
	_, err = engine.Deploy(context.Background(), def)
	require.NoError(t, err)

	// 1. 发起流程
	instID, err := engine.Start(context.Background(), flowx.StartReq{Def: def, Starter: "starter"})
	require.NoError(t, err)

	tasks, _ := engine.ListTasksByInstance(context.Background(), instID)
	backendTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "node_backend" })
	frontendTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "node_frontend" })

	// 2. 第一阶段：后端与前端均通过
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: backendTask.ID, Operator: "peicg"})
	require.NoError(t, err)
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: frontendTask.ID, Operator: "luankz"})
	require.NoError(t, err)

	// 3. 汇聚网关放行，成功激活产品与领导待办
	tasks, _ = engine.ListTasksByInstance(context.Background(), instID)
	productTask, foundProduct := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "node_product" && t.IsPending() })
	require.True(t, foundProduct)
	leaderTask, foundLeader := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "node_leader" && t.IsPending() })
	require.True(t, foundLeader)

	// 产品先通过
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: productTask.ID, Operator: "langtt"})
	require.NoError(t, err)

	// 4. 核心：领导点击驳回
	err = engine.Reject(context.Background(), flowx.RejectReq{
		TaskID:   leaderTask.ID,
		Operator: "boss_luankz",
		Comment:  "方案不合规，退回重新审核",
	})
	require.NoError(t, err)

	// 5. 校验驳回后的实例状态
	inst, err := engine.GetInstance(context.Background(), instID)
	require.NoError(t, err)
	assert.Equal(t, model.InstanceRunning, inst.Status, "领导驳回后，工单绝不能被杀死，必须处于运行中")

	// 6. 校验上游多路分支被完整重新激活（批次号递增为 2）
	allTasks, err := engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)

	batch2Backend, foundB2 := lo.Find(allTasks, func(t *model.Task) bool {
		return t.NodeID == "node_backend" && t.BatchCode == 2 && t.IsPending()
	})
	require.True(t, foundB2, "批次 2 必须重新激活后端待办")
	assert.Equal(t, "peicg", batch2Backend.Assignee)

	batch2Frontend, foundF2 := lo.Find(allTasks, func(t *model.Task) bool {
		return t.NodeID == "node_frontend" && t.BatchCode == 2 && t.IsPending()
	})
	require.True(t, foundF2, "批次 2 必须重新激活前端待办，绝不能遗漏前端")
	assert.Equal(t, "luankz", batch2Frontend.Assignee)

	// 7. 校验流程图高亮视图（GetExecutionView）
	view, err := engine.GetExecutionView(context.Background(), instID)
	require.NoError(t, err)

	// 后端和前端当前为批次 2 待办态 (Pending = 0)
	assert.Equal(t, int(model.TaskPending), view.NodeStatusMap["node_backend"], "后端在流程图中必须高亮为待办(Pending)")
	assert.Equal(t, int(model.TaskPending), view.NodeStatusMap["node_frontend"], "前端在流程图中必须高亮为待办(Pending)")
	// 领导为驳回态 (Rejected = 2)
	assert.Equal(t, int(model.TaskRejected), view.NodeStatusMap["node_leader"], "领导在流程图中必须高亮为驳回(Rejected)")

	// 8. 校验待办人列表
	assert.Contains(t, view.PendingUsers, "peicg")
	assert.Contains(t, view.PendingUsers, "luankz")
	assert.NotContains(t, view.PendingUsers, "boss_luankz")

	// 9. 第二轮流转：后端与前端重新审批通过
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: batch2Backend.ID, Operator: "peicg"})
	require.NoError(t, err)
	err = engine.Pass(context.Background(), flowx.PassReq{TaskID: batch2Frontend.ID, Operator: "luankz"})
	require.NoError(t, err)

	// 再次推进到产品和领导（批次 2 重新发起）
	allTasks, err = engine.ListTasksByInstance(context.Background(), instID)
	require.NoError(t, err)
	b2Product, foundP := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "node_product" && t.BatchCode == 2 && t.IsPending() })
	require.True(t, foundP)
	b2Leader, foundL := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "node_leader" && t.BatchCode == 2 && t.IsPending() })
	require.True(t, foundL)

	// 产品与领导最终全员通过办结
	_ = engine.Pass(context.Background(), flowx.PassReq{TaskID: b2Product.ID, Operator: "langtt"})
	_ = engine.Pass(context.Background(), flowx.PassReq{TaskID: b2Leader.ID, Operator: "boss_luankz"})

	instFinal, _ := engine.GetInstance(context.Background(), instID)
	assert.True(t, instFinal.IsFinished(), "二轮全员通过后，工单圆满办结")
}




