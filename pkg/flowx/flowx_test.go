package flowx

import (
	"context"
	"errors"
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/middleware"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// buildStandardLeaveProcess 构造包含或签、会签、条件分支的综合请假审批流程
func buildStandardLeaveProcess() model.ProcessDefinition {
	return NewProcess("leave_process", "员工请假审批流").
		Start("start", "发起申请").
		Approval("node_leader", "部门主管审批", model.SignOneVote, "leader_alice", "leader_bob"). // 或签
		ExclusiveGateway("gateway_days", "请假天数分支").
		Approval("node_director", "总监审批", model.SignAllPass, "director_carol", "director_dave"). // 会签
		End("end", "审批办结").
		Edge("start", "node_leader").
		Edge("node_leader", "gateway_days").
		ConditionEdge("gateway_days", "node_director", "days > 3").
		ConditionEdge("gateway_days", "end", "days <= 3").
		Edge("node_director", "end").
		Build()
}

// FlowxTestSuite 基于内存存储的 FlowX 工作流状态机集成测试套件
type FlowxTestSuite struct {
	suite.Suite
	ctx context.Context
}

func (s *FlowxTestSuite) SetupTest() {
	s.ctx = context.Background()
}

func TestFlowxTestSuite(t *testing.T) {
	suite.Run(t, new(FlowxTestSuite))
}

// newEngine 创建带有内存存储的全新引擎实例
func (s *FlowxTestSuite) newEngine() (Engine, *memory.Storage) {
	store := memory.NewStorage()
	return NewEngine(store), store
}

// pass 辅助函数：根据操作人定位其在途任务并执行审批通过
func (s *FlowxTestSuite) pass(engine Engine, store *memory.Storage, instID, operator, comment string) {
	tasks, err := store.ListTasksByInstance(s.ctx, instID)
	s.Require().NoError(err)
	task, found := lo.Find(tasks, func(t *model.Task) bool {
		return t.Assignee == operator && t.IsPending()
	})
	s.Require().Truef(found, "未找到操作人 [%s] 的待办任务", operator)
	s.Require().NoError(engine.Pass(s.ctx, PassReq{
		TaskID:   task.ID,
		Operator: operator,
		Comment:  comment,
	}))
}

// reject 辅助函数：根据操作人定位其在途任务并执行审批驳回
func (s *FlowxTestSuite) reject(engine Engine, store *memory.Storage, instID, operator, comment string) {
	tasks, err := store.ListTasksByInstance(s.ctx, instID)
	s.Require().NoError(err)
	task, found := lo.Find(tasks, func(t *model.Task) bool {
		return t.Assignee == operator && t.IsPending()
	})
	s.Require().Truef(found, "未找到操作人 [%s] 的待办任务", operator)
	s.Require().NoError(engine.Reject(s.ctx, RejectReq{
		TaskID:   task.ID,
		Operator: operator,
		Comment:  comment,
	}))
}

// deploy 辅助函数：部署流程定义并断言成功
func (s *FlowxTestSuite) deploy(engine Engine, def model.ProcessDefinition) model.ProcessDefinition {
	d, err := engine.Deploy(s.ctx, def)
	s.Require().NoError(err)
	return d
}

// TestApprovalAndBranch 测试或签与条件分流
func (s *FlowxTestSuite) TestApprovalAndBranch() {
	s.Run("请假2天_主管或签通过后直达END办结", func() {
		engine, store := s.newEngine()
		s.deploy(engine, buildStandardLeaveProcess())

		instID, err := engine.Start(s.ctx, StartReq{
			Key:       "leave_process",
			Starter:   "employee_tom",
			Variables: map[string]any{"days": 2},
		})
		require.NoError(s.T(), err)

		s.pass(engine, store, instID, "leader_alice", "批准短假")

		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
		assert.Equal(s.T(), "end", inst.CurrentNode)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		aliceTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "leader_alice" })
		bobTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "leader_bob" })
		assert.Equal(s.T(), model.TaskPassed, aliceTask.Status)
		assert.Equal(s.T(), model.TaskSkipped, bobTask.Status)
	})

	s.Run("请假5天_主管或签通过后流向总监会签", func() {
		engine, store := s.newEngine()
		s.deploy(engine, buildStandardLeaveProcess())

		instID, err := engine.Start(s.ctx, StartReq{
			Key:       "leave_process",
			Starter:   "employee_tom",
			Variables: map[string]any{"days": 5},
		})
		require.NoError(s.T(), err)

		s.pass(engine, store, instID, "leader_bob", "长假转总监审批")

		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), model.InstanceRunning, inst.Status)
		assert.Equal(s.T(), "node_director", inst.CurrentNode)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		pendingDirectors := lo.Filter(tasks, func(t *model.Task, _ int) bool {
			return t.NodeID == "node_director" && t.IsPending()
		})
		assert.Len(s.T(), pendingDirectors, 2)
	})
}

// TestCosignAndReject 测试会签全员通过与驳回原子清理
func (s *FlowxTestSuite) TestCosignAndReject() {
	s.Run("会签全员通过_流程正常办结", func() {
		engine, store := s.newEngine()
		s.deploy(engine, buildStandardLeaveProcess())

		instID, err := engine.Start(s.ctx, StartReq{
			Key:       "leave_process",
			Starter:   "employee_tom",
			Variables: map[string]any{"days": 5},
		})
		require.NoError(s.T(), err)

		s.pass(engine, store, instID, "leader_alice", "初审通过")
		s.pass(engine, store, instID, "director_carol", "总监1同意")
		s.pass(engine, store, instID, "director_dave", "总监2同意")

		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
		assert.Equal(s.T(), "end", inst.CurrentNode)
	})

	s.Run("会签一人驳回_原子取消兄弟待办并回退上游主管_批次号自增", func() {
		engine, store := s.newEngine()
		s.deploy(engine, buildStandardLeaveProcess())

		instID, err := engine.Start(s.ctx, StartReq{
			Key:       "leave_process",
			Starter:   "employee_tom",
			Variables: map[string]any{"days": 5},
		})
		require.NoError(s.T(), err)

		s.pass(engine, store, instID, "leader_alice", "初审通过")
		s.reject(engine, store, instID, "director_carol", "材料不全驳回")

		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), model.InstanceRunning, inst.Status)
		assert.Equal(s.T(), "node_leader", inst.CurrentNode)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		carolTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "director_carol" })
		daveTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "director_dave" })
		assert.Equal(s.T(), model.TaskRejected, carolTask.Status)
		assert.Equal(s.T(), model.TaskCancelled, daveTask.Status)

		// 批次 2 已激活主管新待办
		batch2Tasks := lo.Filter(tasks, func(t *model.Task, _ int) bool {
			return t.BatchCode == 2 && t.NodeID == "node_leader" && t.IsPending()
		})
		assert.NotEmpty(s.T(), batch2Tasks)
	})

	s.Run("会签驳回后第二轮审批重新提单并全员通过办结闭环", func() {
		engine, store := s.newEngine()
		s.deploy(engine, buildStandardLeaveProcess())

		instID, err := engine.Start(s.ctx, StartReq{
			Key:       "leave_process",
			Starter:   "employee_tom",
			Variables: map[string]any{"days": 5},
		})
		require.NoError(s.T(), err)

		// 第1轮：主管通过 -> Carol驳回
		s.pass(engine, store, instID, "leader_alice", "初审通过")
		s.reject(engine, store, instID, "director_carol", "驳回重审")

		// 第2轮：主管重审同意 -> Carol同意 -> Dave同意
		s.pass(engine, store, instID, "leader_alice", "补充说明重审通过")
		s.pass(engine, store, instID, "director_carol", "总监1复核同意")
		s.pass(engine, store, instID, "director_dave", "总监2复核同意")

		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
		assert.Equal(s.T(), "end", inst.CurrentNode)
	})
}

// TestRuleEngine 测试规则引擎多条件组合判断
func (s *FlowxTestSuite) TestRuleEngine() {
	proc := NewProcess("purchase_proc", "采购审批").
		Start("start", "发起采购").
		ExclusiveGateway("gateway_budget", "预算分流").
		Approval("audit_director", "总监直批", model.SignOneVote, "director_tom").
		End("end", "免批办结").
		Edge("start", "gateway_budget").
		RuleEdge("gateway_budget", "audit_director", model.ConditionGroup{
			Logic: "AND",
			Conditions: []model.Condition{
				{Field: "amount", Operator: model.OpGreaterThan, Value: 50000},
				{Field: "department", Operator: model.OpEqual, Value: "Tech"},
			},
		}).
		RuleEdge("gateway_budget", "end", model.ConditionGroup{
			Logic: "OR",
			Conditions: []model.Condition{
				{Field: "amount", Operator: model.OpLessEqual, Value: 50000},
				{Field: "trial", Operator: model.OpEqual, Value: true},
			},
		}).
		Build()

	testCases := []struct {
		name         string
		vars         map[string]any
		wantNextNode string
		wantFinished bool
	}{
		{
			name:         "大额且技术部_精准命中AND规则流向总监",
			vars:         map[string]any{"amount": 80000, "department": "Tech", "trial": false},
			wantNextNode: "audit_director",
			wantFinished: false,
		},
		{
			name:         "小额采购_命中OR规则快速办结",
			vars:         map[string]any{"amount": 10000, "department": "HR", "trial": false},
			wantNextNode: "end",
			wantFinished: true,
		},
		{
			name:         "大额但测试试用单_命中OR规则快速办结",
			vars:         map[string]any{"amount": 999999, "department": "Sales", "trial": true},
			wantNextNode: "end",
			wantFinished: true,
		},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			engine, store := s.newEngine()
			s.deploy(engine, proc)

			instID, err := engine.Start(s.ctx, StartReq{
				Key:       "purchase_proc",
				Starter:   "buyer_jack",
				Variables: tc.vars,
			})
			require.NoError(s.T(), err)

			inst, err := store.GetInstance(s.ctx, instID)
			require.NoError(s.T(), err)
			assert.Equal(s.T(), tc.wantNextNode, inst.CurrentNode)
			if tc.wantFinished {
				assert.Equal(s.T(), model.InstanceFinished, inst.Status)
			} else {
				assert.Equal(s.T(), model.InstanceRunning, inst.Status)
			}
		})
	}
}

// TestRuleEngine_EdgeOperators 测试所有高级规则操作符
func (s *FlowxTestSuite) TestRuleEngine_EdgeOperators() {
	proc := NewProcess("rule_edge_proc", "高级规则流").
		Start("start", "开始").
		ExclusiveGateway("gw", "网关").
		Approval("special_audit", "特批", model.SignOneVote, "boss").
		End("end", "结束").
		Edge("start", "gw").
		RuleEdge("gw", "special_audit", model.ConditionGroup{
			Logic: "AND",
			Conditions: []model.Condition{
				{Field: "tags", Operator: model.OpContains, Value: "VIP"},
				{Field: "approvers", Operator: model.OpNotEmpty},
				{Field: "notes", Operator: model.OpEmpty},
				{Field: "channel", Operator: model.OpNotIn, Value: []string{"web", "h5"}},
			},
		}).
		Edge("gw", "end").
		Build()

	s.Run("满足Contains_NotEmpty_Empty_NotIn所有高级条件_流向特批", func() {
		engine, store := s.newEngine()
		s.deploy(engine, proc)

		instID, err := engine.Start(s.ctx, StartReq{
			Key:     "rule_edge_proc",
			Starter: "tom",
			Variables: map[string]any{
				"tags":      "USER_VIP_GOLD",
				"approvers": []string{"leader"},
				"notes":     "",
				"channel":   "app",
			},
		})
		require.NoError(s.T(), err)

		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), "special_audit", inst.CurrentNode)
		assert.Equal(s.T(), model.InstanceRunning, inst.Status)
	})

	s.Run("不满足ChannelNotIn条件_默认兜底流向结束", func() {
		engine, store := s.newEngine()
		s.deploy(engine, proc)

		instID, err := engine.Start(s.ctx, StartReq{
			Key:     "rule_edge_proc",
			Starter: "tom",
			Variables: map[string]any{
				"tags":      "USER_VIP_GOLD",
				"approvers": []string{"leader"},
				"notes":     "",
				"channel":   "web", // 落在排除名单中
			},
		})
		require.NoError(s.T(), err)

		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), "end", inst.CurrentNode)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
	})
}

// TestEventsAndHooks 测试同步Hook守卫与异步EventBus广播
func (s *FlowxTestSuite) TestEventsAndHooks() {
	def := NewProcess("event_flow", "生命周期事件与钩子流").
		Start("start", "发起").
		Approval("audit", "主管审批", model.SignOneVote).
		Hook("audit", "resolve_leader", "check_budget").
		End("end", "结束").
		Edge("start", "audit").
		Edge("audit", "end").
		Build()

	s.Run("合规申请_触发Hook算人与EventBus广播", func() {
		engine, store := s.newEngine()

		// 1. 注册同步 Hook (运行在主事务内，用于算人和守卫)
		engine.RegisterHook("resolve_leader", func(_ context.Context, hc *HookContext) error {
			hc.SetAssignees("dynamic_boss")
			return nil
		})
		engine.RegisterHook("check_budget", func(_ context.Context, hc *HookContext) error {
			return nil
		})

		// 2. 注册生命周期事件观察者 (解耦外围通知与业务联动)
		var passedEvents []Event
		engine.Subscribe([]EventType{EventTaskPassed, EventProcessFinished}, func(_ context.Context, ev Event) error {
			passedEvents = append(passedEvents, ev)
			if ev.Type == EventTaskPassed {
				ev.Instance.MergeVariables(map[string]any{"etask_done": true})
			}
			return nil
		})

		s.deploy(engine, def)
		instID, err := engine.Start(s.ctx, StartReq{
			Key:       "event_flow",
			Starter:   "tom",
			Variables: map[string]any{"amount": 5000},
		})
		require.NoError(s.T(), err)

		s.pass(engine, store, instID, "dynamic_boss", "通过")

		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
		assert.Equal(s.T(), true, inst.Variables["etask_done"])
		assert.Len(s.T(), passedEvents, 2) // TaskPassed + ProcessFinished
	})

	s.Run("超额申请_Hook被Abort主动阻断流转", func() {
		engine, _ := s.newEngine()
		engine.RegisterHook("resolve_leader", func(_ context.Context, hc *HookContext) error {
			hc.SetAssignees("dynamic_boss")
			return nil
		})
		engine.RegisterHook("check_budget", func(_ context.Context, hc *HookContext) error {
			hc.Abort(errors.New("超出预算限额，禁止流转"))
			return nil
		})

		s.deploy(engine, def)
		_, err := engine.Start(s.ctx, StartReq{
			Key:       "event_flow",
			Starter:   "tom",
			Variables: map[string]any{"amount": 99999},
		})
		require.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "超出预算限额，禁止流转")
	})
}

// TestDefinitionValidation 测试流程定义拓扑语法校验
func (s *FlowxTestSuite) TestDefinitionValidation() {
	s.Run("缺少开始节点", func() {
		engine, _ := s.newEngine()
		b := NewProcess("bad_proc_1", "无开始节点").
			Approval("n1", "审批", model.SignOneVote, "u1").
			End("end", "结束").
			Edge("n1", "end")
		require.Error(s.T(), b.Validate())
		_, err := engine.Deploy(s.ctx, b.Build())
		require.ErrorIs(s.T(), err, ErrInvalidDefinition)
	})

	s.Run("缺少结束节点", func() {
		b := NewProcess("bad_proc_2", "无结束节点").
			Start("start", "开始").
			Approval("n1", "审批", model.SignOneVote, "u1").
			Edge("start", "n1")
		require.Error(s.T(), b.Validate())
	})

	s.Run("存在悬空无效边", func() {
		b := NewProcess("bad_proc_3", "悬空边").
			Start("start", "开始").
			End("end", "结束").
			Edge("start", "non_existent_node")
		require.Error(s.T(), b.Validate())
	})

	s.Run("边存在无效自环连接", func() {
		b := NewProcess("bad_proc_4", "自环边").
			Start("start", "开始").
			Approval("n1", "审批", model.SignOneVote, "u1").
			End("end", "结束").
			Edge("start", "n1").
			Edge("n1", "n1").
			Edge("n1", "end")
		require.Error(s.T(), b.Validate())
	})

	s.Run("网关节点悬空无出边_FailFast静态拦截", func() {
		b := NewProcess("bad_proc_gw_no_out", "网关无出边").
			Start("start", "开始").
			ExclusiveGateway("gw", "孤立网关").
			End("end", "结束").
			Edge("start", "gw").
			Edge("start", "end")
		err := b.Validate()
		require.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "悬空异常")
	})

	s.Run("开始节点存在非法入边_FailFast静态拦截", func() {
		b := NewProcess("bad_proc_start_in", "开始节点有入边").
			Start("start", "开始").
			Approval("n1", "审批", model.SignOneVote, "u1").
			End("end", "结束").
			Edge("start", "n1").
			Edge("n1", "start").
			Edge("n1", "end")
		err := b.Validate()
		require.Error(s.T(), err)
		assert.Contains(s.T(), err.Error(), "开始节点 [start] 不能拥有入边")
	})
}

// TestVersionAutoIncrement 测试流程定义多次发布时的原子版本递增机制
func (s *FlowxTestSuite) TestVersionAutoIncrement() {
	s.Run("未指定版本发布自动从1自增递增", func() {
		engine, store := s.newEngine()

		baseDef := func() model.ProcessDefinition {
			return NewProcess("auto_incr_proc", "自增版本流程").
				Version(0).
				Start("start", "开始").
				End("end", "结束").
				Edge("start", "end").
				Build()
		}

		// 首次发布：应自增为版本 1
		defV1, err := engine.Deploy(s.ctx, baseDef())
		s.Require().NoError(err)
		s.Assert().Equal(1, defV1.Version)

		storedV1, err := store.GetDefinition(s.ctx, "auto_incr_proc", 1)
		s.Require().NoError(err)
		s.Assert().Equal(1, storedV1.Version)

		// 二次发布：应自增为版本 2
		defV2, err := engine.Deploy(s.ctx, baseDef())
		s.Require().NoError(err)
		s.Assert().Equal(2, defV2.Version)

		storedV2, err := store.GetDefinition(s.ctx, "auto_incr_proc", 2)
		s.Require().NoError(err)
		s.Assert().Equal(2, storedV2.Version)

		// 三次发布：应自增为版本 3
		defV3, err := engine.Deploy(s.ctx, baseDef())
		s.Require().NoError(err)
		s.Assert().Equal(3, defV3.Version)

		// 显式指定版本覆盖发布（例如修复已有版本 2）
		fixDef := baseDef()
		fixDef.Version = 2
		fixDef.Name = "修复后的版本2"
		defFixed, err := engine.Deploy(s.ctx, fixDef)
		s.Require().NoError(err)
		s.Assert().Equal(2, defFixed.Version)

		storedFixed, err := store.GetDefinition(s.ctx, "auto_incr_proc", 2)
		s.Require().NoError(err)
		s.Assert().Equal("修复后的版本2", storedFixed.Name)
	})
}

// TestGateways 测试各类网关行为
func (s *FlowxTestSuite) TestGateways() {
	s.Run("排他网关XOR_多条件满足时首个短路仅走单条分支", func() {
		engine, store := s.newEngine()
		def := NewProcess("exclusive_proc", "排他网关测试").
			Start("start", "开始").
			ExclusiveGateway("xor_gw", "排他分支").
			Approval("branch_first", "第一匹配分支", model.SignOneVote, "alice").
			Approval("branch_second", "第二匹配分支", model.SignOneVote, "bob").
			End("end", "结束").
			Edge("start", "xor_gw").
			ConditionEdge("xor_gw", "branch_first", "score >= 60").
			ConditionEdge("xor_gw", "branch_second", "score >= 80").
			Edge("branch_first", "end").
			Edge("branch_second", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{
			Def:       def,
			Starter:   "tom",
			Variables: map[string]any{"score": 90},
		})
		require.NoError(s.T(), err)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.Len(s.T(), tasks, 1)
		assert.Equal(s.T(), "branch_first", tasks[0].NodeID)
	})

	s.Run("并行网关AND_分支全开且汇聚必须全员到达", func() {
		engine, store := s.newEngine()
		def := NewProcess("parallel_proc", "并行网关测试").
			Start("start", "开始").
			ParallelGateway("fork_gw", "并行分流").
			Approval("audit_dev", "研发审批", model.SignOneVote, "alice").
			Approval("audit_sec", "安全审批", model.SignOneVote, "bob").
			ParallelGateway("join_gw", "并行汇聚").
			End("end", "结束").
			Edge("start", "fork_gw").
			Edge("fork_gw", "audit_dev").
			Edge("fork_gw", "audit_sec").
			Edge("audit_dev", "join_gw").
			Edge("audit_sec", "join_gw").
			Edge("join_gw", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "tom"})
		require.NoError(s.T(), err)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		t1, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "audit_dev" })
		t2, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "audit_sec" })

		_ = engine.Pass(s.ctx, PassReq{TaskID: t1.ID, Operator: "alice"})
		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceRunning, inst.Status)

		_ = engine.Pass(s.ctx, PassReq{TaskID: t2.ID, Operator: "bob"})
		inst, _ = store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
	})

	s.Run("包容网关OR_多分支部分命中自动Skip未激活分支且汇聚正常推进", func() {
		engine, store := s.newEngine()
		def := NewProcess("inclusive_multi_proc", "包容网关多分支测试").
			Start("start", "开始").
			InclusiveGateway("fork", "三路分流").
			Approval("node_dev", "研发", model.SignOneVote, "alice").
			Approval("node_sec", "安全", model.SignOneVote, "bob").
			Approval("node_ops", "运维", model.SignOneVote, "carol").
			InclusiveGateway("join", "三路汇聚").
			End("end", "结束").
			Edge("start", "fork").
			ConditionEdge("fork", "node_dev", "is_dev == true").
			ConditionEdge("fork", "node_sec", "is_sec == true").
			ConditionEdge("fork", "node_ops", "is_ops == true").
			Edge("node_dev", "join").
			Edge("node_sec", "join").
			Edge("node_ops", "join").
			Edge("join", "end").
			Build()

		// 仅激活 dev 和 sec，ops 自动标记为 TaskSkipped
		instID, err := engine.Start(s.ctx, StartReq{
			Def:       def,
			Starter:   "tom",
			Variables: map[string]any{"is_dev": true, "is_sec": true, "is_ops": false},
		})
		require.NoError(s.T(), err)

		allTasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.Len(s.T(), allTasks, 3)

		devTask, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "node_dev" })
		secTask, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "node_sec" })
		opsTask, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "node_ops" })

		assert.Equal(s.T(), model.TaskPending, devTask.Status)
		assert.Equal(s.T(), model.TaskPending, secTask.Status)
		assert.Equal(s.T(), model.TaskSkipped, opsTask.Status)

		// dev 审批通过，汇聚等待 sec
		_ = engine.Pass(s.ctx, PassReq{TaskID: devTask.ID, Operator: "alice"})
		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceRunning, inst.Status)

		// sec 审批通过，ops 已 Skip 无需死等，全单办结！
		_ = engine.Pass(s.ctx, PassReq{TaskID: secTask.ID, Operator: "bob"})
		inst, _ = store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
	})
}

// TestNodeTask_ChainedWorkers 测试自动化节点连续链式调度与变量透传
func (s *FlowxTestSuite) TestNodeTask_ChainedWorkers() {
	s.Run("连续双自动化节点自动链式执行并合并增量上下文", func() {
		engine, store := s.newEngine()
		engine.RegisterWorker("SERVICE_A", func(c context.Context, tc *TaskContext) (*TaskResult, error) {
			return &TaskResult{Variables: map[string]any{"token": "JWT_AAA"}}, nil
		})
		engine.RegisterWorker("SERVICE_B", func(c context.Context, tc *TaskContext) (*TaskResult, error) {
			assert.Equal(s.T(), "JWT_AAA", tc.Variables["token"], "ServiceB 应读取到 ServiceA 的输出变量")
			return &TaskResult{Variables: map[string]any{"asset_id": 8888}}, nil
		})

		def := NewProcess("chained_task_proc", "连续任务链").
			Start("start", "开始").
			Task("task_a", "调用鉴权服务", "SERVICE_A").
			Task("task_b", "调用资产服务", "SERVICE_B").
			Approval("final_audit", "终审", model.SignOneVote, "boss").
			End("end", "结束").
			Edge("start", "task_a").
			Edge("task_a", "task_b").
			Edge("task_b", "final_audit").
			Edge("final_audit", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "tom"})
		require.NoError(s.T(), err)

		// 流程自动连续穿透 task_a 与 task_b，直接停留在 final_audit
		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "final_audit", inst.CurrentNode)
		assert.Equal(s.T(), "JWT_AAA", inst.Variables["token"])
		assert.Equal(s.T(), 8888, inst.Variables["asset_id"])

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.Len(s.T(), tasks, 3) // task_a (passed) + task_b (passed) + final_audit (pending)
	})
}

// TestTimeline_MultiBatchRejectionAudit 测试多轮驳回回退时的时间线按批次完全隔离与真实轨迹审计
func (s *FlowxTestSuite) TestTimeline_MultiBatchRejectionAudit() {
	s.Run("多轮驳回回退时时间线按批次完全分离呈现真实审计轨迹", func() {
		engine, store := s.newEngine()
		def := NewProcess("audit_timeline_proc", "审计时间线流程").
			Start("start", "开始").
			Approval("step_leader", "初审", model.SignOneVote, "leader").
			Approval("step_cfo", "复审", model.SignOneVote, "cfo").
			End("end", "结束").
			Edge("start", "step_leader").
			Edge("step_leader", "step_cfo").
			Edge("step_cfo", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "tom"})
		require.NoError(s.T(), err)

		// 第 1 轮初审通过
		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		_ = engine.Pass(s.ctx, PassReq{TaskID: tasks[0].ID, Operator: "leader"})

		// 第 1 轮复审驳回回退至初审
		tasks, _ = store.ListTasksByInstance(s.ctx, instID)
		cfoTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "step_cfo" && t.IsPending() })
		_ = engine.Reject(s.ctx, RejectReq{TaskID: cfoTask.ID, Operator: "cfo", Comment: "发票模糊"})

		// 第 2 轮初审再次通过
		tasks, _ = store.ListTasksByInstance(s.ctx, instID)
		newLeaderTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "step_leader" && t.IsPending() })
		assert.Equal(s.T(), 2, newLeaderTask.BatchCode)
		_ = engine.Pass(s.ctx, PassReq{TaskID: newLeaderTask.ID, Operator: "leader"})

		// 第 2 轮复审同意
		tasks, _ = store.ListTasksByInstance(s.ctx, instID)
		newCfoTask, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "step_cfo" && t.IsPending() })
		assert.Equal(s.T(), 2, newCfoTask.BatchCode)
		_ = engine.Pass(s.ctx, PassReq{TaskID: newCfoTask.ID, Operator: "cfo"})

		// 获取完整时间线
		timeline, err := engine.GetTimeline(s.ctx, instID)
		require.NoError(s.T(), err)
		require.Len(s.T(), timeline, 4)

		// 校验批次严格递增隔离：批次1初审 -> 批次1复审(驳回) -> 批次2初审 -> 批次2复审(通过)
		assert.Equal(s.T(), "step_leader", timeline[0].NodeID)
		assert.Equal(s.T(), 1, timeline[0].BatchCode)

		assert.Equal(s.T(), "step_cfo", timeline[1].NodeID)
		assert.Equal(s.T(), 1, timeline[1].BatchCode)
		assert.Equal(s.T(), model.TaskRejected, timeline[1].Tasks[0].Status)

		assert.Equal(s.T(), "step_leader", timeline[2].NodeID)
		assert.Equal(s.T(), 2, timeline[2].BatchCode)

		assert.Equal(s.T(), "step_cfo", timeline[3].NodeID)
		assert.Equal(s.T(), 2, timeline[3].BatchCode)
		assert.Equal(s.T(), model.TaskPassed, timeline[3].Tasks[0].Status)
	})
}

// TestTargetedReject 包含直接作废与指定节点回退测试
func (s *FlowxTestSuite) TestTargetedReject() {
	s.Run("审批人驳回直接作废工单_不回退", func() {
		engine, store := s.newEngine()
		def := NewProcess("reject_terminate", "直接作废流程").
			Start("start", "开始").
			Approval("first_audit", "初审", model.SignOneVote, "leader").
			End("end", "办结").
			Edge("start", "first_audit").
			Edge("first_audit", "end").
			Build()

		instID, _ := engine.Start(s.ctx, StartReq{Def: def, Starter: "alice"})
		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		_ = engine.Reject(s.ctx, RejectReq{
			TaskID:    tasks[0].ID,
			Operator:  "leader",
			Comment:   "直接终止",
			Terminate: true,
		})

		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceRejected, inst.Status)
	})

	s.Run("跨多级驳回至指定历史人工节点", func() {
		engine, store := s.newEngine()
		def := NewProcess("multilevel_reject", "多级审批驳回").
			Start("start", "开始").
			Approval("audit_a", "节点A", model.SignOneVote, "user_a").
			Approval("audit_b", "节点B", model.SignOneVote, "user_b").
			Approval("audit_c", "节点C", model.SignOneVote, "user_c").
			End("end", "办结").
			Edge("start", "audit_a").
			Edge("audit_a", "audit_b").
			Edge("audit_b", "audit_c").
			Edge("audit_c", "end").
			Build()

		instID, _ := engine.Start(s.ctx, StartReq{Def: def, Starter: "alice"})
		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		tA, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "audit_a" })
		_ = engine.Pass(s.ctx, PassReq{TaskID: tA.ID, Operator: "user_a"})

		tasks, _ = store.ListTasksByInstance(s.ctx, instID)
		tB, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "audit_b" })
		_ = engine.Pass(s.ctx, PassReq{TaskID: tB.ID, Operator: "user_b"})

		tasks, _ = store.ListTasksByInstance(s.ctx, instID)
		tC, _ := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "audit_c" })
		_ = engine.Reject(s.ctx, RejectReq{
			TaskID:       tC.ID,
			Operator:     "user_c",
			TargetNodeID: "audit_a",
		})

		allTasks, _ := store.ListTasksByInstance(s.ctx, instID)
		newATask, found := lo.Find(allTasks, func(t *model.Task) bool {
			return t.NodeID == "audit_a" && t.BatchCode == 2 && t.Status == model.TaskPending
		})
		assert.True(s.T(), found)
		assert.Equal(s.T(), "user_a", newATask.Assignee)
	})
}

// TestRevoke 测试提单人主动撤回全场景
func (s *FlowxTestSuite) TestRevoke() {
	def := NewProcess("revoke_flow", "撤回流程").
		Start("start", "开始").
		Approval("audit_node", "部门审批", model.SignOneVote, "boss").
		End("end", "办结").
		Edge("start", "audit_node").
		Edge("audit_node", "end").
		Build()

	s.Run("已终结工单撤回防御拦截", func() {
		engine, store := s.newEngine()
		instID, _ := engine.Start(s.ctx, StartReq{Def: def, Starter: "alice"})
		// 先正常审批推进至办结
		s.pass(engine, store, instID, "boss", "审批办结")
		// 再次撤回已办结工单应当被物理拦截
		err := engine.Revoke(s.ctx, RevokeReq{InstanceID: instID, Operator: "alice"})
		require.Error(s.T(), err)
		assert.True(s.T(), errors.Is(err, ErrInstanceNotRunning))
	})

	s.Run("流转中工单主动撤回直接作废终结", func() {
		engine, store := s.newEngine()
		instID, _ := engine.Start(s.ctx, StartReq{Def: def, Starter: "alice"})
		err := engine.Revoke(s.ctx, RevokeReq{InstanceID: instID, Operator: "alice", BackToStarter: false})
		require.NoError(s.T(), err)

		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceCancelled, inst.Status)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.NotEmpty(s.T(), tasks)
		for _, t := range tasks {
			assert.Equal(s.T(), model.TaskCancelled, t.Status)
		}
	})
}

// TestMiddlewarePipeline 测试全局中间件拦截与追踪
func (s *FlowxTestSuite) TestMiddlewarePipeline() {
	s.Run("洋葱模型中间件执行顺序与上下文传递", func() {
		engine, store := s.newEngine()
		var traceLog []string
		mw := func(next middleware.Handler) middleware.Handler {
			return func(c context.Context, ac *middleware.ActionContext) error {
				traceLog = append(traceLog, "in:"+string(ac.ActionType))
				err := next(c, ac)
				traceLog = append(traceLog, "out:"+string(ac.ActionType))
				return err
			}
		}
		engine.Use(mw)

		def := NewProcess("mw_proc", "中间件流").
			Start("start", "开始").
			Approval("audit", "审批", model.SignOneVote, "alice").
			End("end", "结束").
			Edge("start", "audit").
			Edge("audit", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "tom"})
		require.NoError(s.T(), err)
		assert.Equal(s.T(), []string{"in:START", "out:START"}, traceLog)

		traceLog = nil
		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		_ = engine.Pass(s.ctx, PassReq{TaskID: tasks[0].ID, Operator: "alice"})
		assert.Equal(s.T(), []string{"in:PASS", "out:PASS"}, traceLog)
	})
}

// TestBuilder_FailFast 测试构建器语法早错与中断
func (s *FlowxTestSuite) TestBuilder_FailFast() {
	s.Run("配置生命周期Hook时若节点不存在明确报错且Build中断", func() {
		b := NewProcess("fail_fast_flow", "快速失败测试").
			Start("start", "开始").
			End("end", "结束")

		b.Hook("audit_not_exist", "some_hook")
		require.Error(s.T(), b.Validate())
		assert.Panics(s.T(), func() {
			b.Build()
		})
	})
}

// TestCompleteTask 测试外部异步任务回调推进与变量合并
func (s *FlowxTestSuite) TestCompleteTask() {
	s.Run("外部系统回调CompleteTask顺利推进流程并合并增量表单数据", func() {
		engine, store := s.newEngine()
		def := NewProcess("async_callback_proc", "异步回调流程").
			Start("start", "开始").
			Approval("audit_user", "人工审批", model.SignOneVote, "leader").
			Approval("audit_boss", "终审", model.SignOneVote, "boss").
			End("end", "结束").
			Edge("start", "audit_user").
			Edge("audit_user", "audit_boss").
			Edge("audit_boss", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "tom"})
		require.NoError(s.T(), err)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.Len(s.T(), tasks, 1)

		// 模拟外部自动化代理直接调用 CompleteTask 回调推进
		err = engine.CompleteTask(s.ctx, CompleteTaskReq{
			TaskID:    tasks[0].ID,
			Comment:   "外部机器人自动放行",
			Variables: map[string]any{"async_result": "SUCCESS_200"},
		})
		require.NoError(s.T(), err)

		// 验证当前流程已成功流转至下游节点 audit_boss
		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.Equal(s.T(), "audit_boss", inst.CurrentNode)
		assert.Equal(s.T(), "SUCCESS_200", inst.Variables["async_result"])

		// 验证首节点任务已由系统回调完成
		allTasks, _ := store.ListTasksByInstance(s.ctx, instID)
		t1, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "audit_user" })
		assert.Equal(s.T(), model.TaskPassed, t1.Status)
		assert.Equal(s.T(), "外部机器人自动放行", t1.Comment)
	})

	s.Run("抄送节点CC_生成已办结审计任务且瞬时自动穿透推进至下游", func() {
		engine, store := s.newEngine()

		var notifiedAssignees []string
		engine.Subscribe([]EventType{EventTaskPassed}, func(c context.Context, ev Event) error {
			if ev.Node != nil && ev.Node.ID == "cc_hr" && ev.Task != nil {
				notifiedAssignees = append(notifiedAssignees, ev.Task.Assignee)
			}
			return nil
		})

		def := NewProcess("cc_test_proc", "抄送测试流程").
			Start("start", "开始").
			Approval("audit", "主管审批", model.SignOneVote, "leader_zhang").
			CarbonCopy("cc_hr", "抄送HR与财务", "hr_alice", "finance_bob").
			Approval("final_audit", "总监终审", model.SignOneVote, "boss_li").
			End("end", "结束").
			Edge("start", "audit").
			Edge("audit", "cc_hr").
			Edge("cc_hr", "final_audit").
			Edge("final_audit", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "dev_tom"})
		require.NoError(s.T(), err)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.Len(s.T(), tasks, 1)
		assert.Equal(s.T(), "audit", tasks[0].NodeID)

		// 主管审批通过
		err = engine.Pass(s.ctx, PassReq{TaskID: tasks[0].ID, Operator: "leader_zhang"})
		require.NoError(s.T(), err)

		// 验证：抄送节点无需人工操作，瞬时穿透，流程已直接流转至 final_audit 节点
		inst, _ := store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceRunning, inst.Status)
		assert.Equal(s.T(), "final_audit", inst.CurrentNode)

		// 验证抄送任务生成状态：为 hr_alice 和 finance_bob 生成了 TaskPassed 记录
		allTasks, _ := store.ListTasksByInstance(s.ctx, instID)
		ccAlice, hasAlice := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "cc_hr" && t.Assignee == "hr_alice" })
		ccBob, hasBob := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "cc_hr" && t.Assignee == "finance_bob" })
		require.True(s.T(), hasAlice)
		require.True(s.T(), hasBob)
		assert.Equal(s.T(), model.TaskPassed, ccAlice.Status)
		assert.True(s.T(), ccAlice.IsPassed())
		assert.Equal(s.T(), model.TaskPassed, ccBob.Status)
		assert.True(s.T(), ccBob.IsPassed())

		// 验证抄送通知事件已成功触发并收到指定抄送人
		assert.ElementsMatch(s.T(), []string{"hr_alice", "finance_bob"}, notifiedAssignees)

		// 终审总监通过，整单正常办结
		finalTask, _ := lo.Find(allTasks, func(t *model.Task) bool { return t.NodeID == "final_audit" })
		err = engine.Pass(s.ctx, PassReq{TaskID: finalTask.ID, Operator: "boss_li"})
		require.NoError(s.T(), err)

		inst, _ = store.GetInstance(s.ctx, instID)
		assert.Equal(s.T(), model.InstanceFinished, inst.Status)
	})
}

// mockAuditListener 模拟实现 IEventListener 接口的独立监听器
type mockAuditListener struct {
	types  []EventType
	events []Event
}

func (m *mockAuditListener) InterestIn() []EventType {
	return m.types
}

func (m *mockAuditListener) OnEvent(_ context.Context, event Event) error {
	m.events = append(m.events, event)
	return nil
}

// TestEventBus_Architecture 验证异步EventBus观察者监听、类型过滤与Panic安全兜底
func (s *FlowxTestSuite) TestEventBus_Architecture() {
	s.Run("自定义IEventListener监听器按需接收事件并支持Panic兜底隔离", func() {
		engine, store := s.newEngine()

		auditListener := &mockAuditListener{
			types: []EventType{EventTaskCreated, EventTaskPassed},
		}
		engine.AddListener(auditListener)

		// 注册一个故意抛出 panic 的极端监听器，验证引擎不被第三方崩溃影响
		engine.Subscribe([]EventType{EventTaskCreated}, func(ctx context.Context, event Event) error {
			panic("simulated external notification crash")
		})

		def := NewProcess("eventbus_test", "EventBus高可用测试").
			Start("start", "开始").
			Approval("audit", "主管审批", model.SignOneVote, "leader").
			End("end", "结束").
			Edge("start", "audit").
			Edge("audit", "end").
			Build()

		// 尽管监听器发生 panic，流转依然安全成功
		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "tom"})
		require.NoError(s.T(), err)

		tasks, _ := store.ListTasksByInstance(s.ctx, instID)
		require.Len(s.T(), tasks, 1)

		err = engine.Pass(s.ctx, PassReq{TaskID: tasks[0].ID, Operator: "leader"})
		require.NoError(s.T(), err)

		// 验证合法的 auditListener 正常捕获了 Created 和 Passed 两次事件
		assert.Len(s.T(), auditListener.events, 2)
		assert.Equal(s.T(), EventTaskCreated, auditListener.events[0].Type)
		assert.Equal(s.T(), EventTaskPassed, auditListener.events[1].Type)
	})

	s.Run("内置占位符_AssigneeStarter自动解析为工单发起人", func() {
		store := memory.NewStorage()
		engine := NewEngine(store)

		def := NewProcess("starter_placeholder_proc", "发起人占位符流").
			Start("start", "开始").
			Approval("submitter_review", "提交人预审", model.SignOneVote, model.AssigneeStarter).
			Approval("cosigned_node", "发起人与主管双签", model.SignAllPass, model.AssigneeInitiator, "manager_bob").
			End("end", "办结").
			Edge("start", "submitter_review").
			Edge("submitter_review", "cosigned_node").
			Edge("cosigned_node", "end").
			Build()

		// 以 "luankz" 作为发起人启动流程
		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "luankz"})
		require.NoError(s.T(), err)

		// 验证首个审批任务的处理人已就地展开为 "luankz"
		tasks, err := store.ListTasksByInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		require.Len(s.T(), tasks, 1)
		assert.Equal(s.T(), "submitter_review", tasks[0].NodeID)
		assert.Equal(s.T(), "luankz", tasks[0].Assignee)

		// 提单人通过后推进到下一个会签节点
		err = engine.Pass(s.ctx, PassReq{TaskID: tasks[0].ID, Operator: "luankz"})
		require.NoError(s.T(), err)

		nextTasks, err := store.ListTasksByInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		require.Len(s.T(), nextTasks, 3) // 1个历史通过任务 + 2个当前并发待办

		activeAssignees := lo.FilterMap(nextTasks, func(t *model.Task, _ int) (string, bool) {
			return t.Assignee, t.IsPending()
		})
		assert.ElementsMatch(s.T(), []string{"luankz", "manager_bob"}, activeAssignees)
	})

	s.Run("机制策略分离_代理与代办审批流转_内核作为通用状态机推进流程并将操作人记录至审计事件", func() {
		store := memory.NewStorage()
		engine := NewEngine(store)

		var passedOperator string
		engine.Subscribe([]EventType{EventTaskPassed}, func(ctx context.Context, evt Event) error {
			passedOperator = evt.Operator
			return nil
		})

		def := NewProcess("admin_proxy_proc", "代办审批流转测试").
			Start("start", "开始").
			Approval("audit_node", "部门审批", model.SignOneVote, "peicg").
			End("end", "结束").
			Edge("start", "audit_node").
			Edge("audit_node", "end").
			Build()

		instID, err := engine.Start(s.ctx, StartReq{Def: def, Starter: "starter_user"})
		require.NoError(s.T(), err)

		tasks, err := store.ListTasksByInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		require.Len(s.T(), tasks, 1)
		task := tasks[0]
		assert.Equal(s.T(), "peicg", task.Assignee)

		// 管理员或受委托人 luankz 推进审批
		errPass := engine.Pass(s.ctx, PassReq{
			TaskID:   task.ID,
			Operator: "luankz",
			Comment:  "管理员代办同意",
		})
		require.NoError(s.T(), errPass)

		// 校验内核作为通用状态机顺利完结实例，并将实际操作人准确广播至审计事件
		inst, err := store.GetInstance(s.ctx, instID)
		require.NoError(s.T(), err)
		assert.True(s.T(), inst.IsFinished())
		assert.Equal(s.T(), "luankz", passedOperator)

		// 再次重复操作时受物理防卫拦截
		errRepeat := engine.Pass(s.ctx, PassReq{
			TaskID:   task.ID,
			Operator: "luankz",
		})
		require.Error(s.T(), errRepeat)
		require.ErrorIs(s.T(), errRepeat, ErrTaskNotPending)
	})
}

// TestEngine_GetExecutionView 测试引擎执行视图与高亮连线投影
func TestEngine_GetExecutionView(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	engine := NewEngine(store)
	def := buildStandardLeaveProcess()
	_, err := engine.Deploy(ctx, def)
	require.NoError(t, err)

	instID, err := engine.Start(ctx, StartReq{
		Key:       "leave_process",
		Starter:   "employee_tom",
		Variables: map[string]any{"days": 2},
	})
	require.NoError(t, err)

	view, err := engine.GetExecutionView(ctx, instID)
	require.NoError(t, err)
	assert.Equal(t, "leave_process", view.ProcessKey)
	assert.Contains(t, view.TraversedEdges["start"], "node_leader")
	assert.Contains(t, view.PendingUsers, "leader_alice")
}

// TestEngine_GetExecutionView_OrSignPassNotOverwrittenBySkipped 验证或签场景下通过的任务不会被兄弟跳过任务覆盖状态
func TestEngine_GetExecutionView_OrSignPassNotOverwrittenBySkipped(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	engine := NewEngine(store)

	def := model.ProcessDefinition{
		Key:     "or_sign_flow",
		Name:    "或签高亮测试",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{
				ID: "hr_node", Type: model.NodeApproval, Name: "人事",
				SignRule: model.SignOneVote,
				Assignees: []string{"luankz", "peicg"},
			},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "hr_node"},
			{Source: "hr_node", Target: "end"},
		},
	}
	_, err := engine.Deploy(ctx, def)
	require.NoError(t, err)

	instID, err := engine.Start(ctx, StartReq{
		Key:     "or_sign_flow",
		Starter: "alice",
	})
	require.NoError(t, err)

	tasks, err := store.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	// 查找 luankz 的任务并审批通过
	luanTask, ok := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "luankz" })
	require.True(t, ok)

	err = engine.Pass(ctx, PassReq{
		TaskID:   luanTask.ID,
		Operator: "luankz",
		Comment:  "同意",
	})
	require.NoError(t, err)

	// 检查当前任务状态：luankz 任务为 Passed (1)，peicg 任务为 Skipped (5)
	afterTasks, err := store.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	peicgTask, ok := lo.Find(afterTasks, func(t *model.Task) bool { return t.Assignee == "peicg" })
	require.True(t, ok)
	assert.Equal(t, model.TaskSkipped, peicgTask.Status, "兄弟任务应当被联动置为 Skipped (5)")

	// 核心断言：获取执行视图时，hr_node 的综合状态必须为 TaskPassed (1)，绝不能被 5 覆盖！
	view, err := engine.GetExecutionView(ctx, instID)
	require.NoError(t, err)
	assert.Equal(t, int(model.TaskPassed), view.NodeStatusMap["hr_node"], "或签一人通过后，节点综合状态必须为 Passed(1)")
	assert.Contains(t, view.TraversedEdges["start"], "hr_node", "start 到 hr_node 的边必须高亮")
	assert.Contains(t, view.TraversedEdges["hr_node"], "end", "hr_node 到 end 的边必须高亮")
}

// TestEngine_ChatGroupNodeTraversalAndTask 验证群聊通知节点在穿透推进时正确落库 Task 快照并点亮拓扑连线
func TestEngine_ChatGroupNodeTraversalAndTask(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	engine := NewEngine(store)

	def := model.ProcessDefinition{
		Key:     "chat_group_flow",
		Name:    "群通知测试流程",
		Version: 1,
		Nodes: []model.NodeSpec{
			{ID: "start", Type: model.NodeStart, Name: "开始"},
			{ID: "chat_node", Type: model.NodeChatGroup, Name: "发版群通知"},
			{ID: "end", Type: model.NodeEnd, Name: "结束"},
		},
		Edges: []model.EdgeSpec{
			{Source: "start", Target: "chat_node"},
			{Source: "chat_node", Target: "end"},
		},
	}
	_, err := engine.Deploy(ctx, def)
	require.NoError(t, err)

	// 启动流程：应当自动经过 chat_node 穿透流转，直接全单办结
	instID, err := engine.Start(ctx, StartReq{
		Key:     "chat_group_flow",
		Starter: "luankz",
	})
	require.NoError(t, err)

	inst, err := store.GetInstance(ctx, instID)
	require.NoError(t, err)
	assert.True(t, inst.IsTerminal(), "群通知穿透后流程应当顺利办结归档")

	// 1. 验证生成了已通过的群通知 Task 快照记录
	tasks, err := store.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	chatTask, ok := lo.Find(tasks, func(t *model.Task) bool { return t.NodeID == "chat_node" })
	require.True(t, ok, "必须落库群通知节点的 Task 记录")
	assert.Equal(t, model.TaskPassed, chatTask.Status)
	assert.Equal(t, "chat_group", chatTask.Assignee)

	// 2. 验证执行视图拓扑高亮完整贯穿：start -> chat_node -> end
	view, err := engine.GetExecutionView(ctx, instID)
	require.NoError(t, err)
	assert.Contains(t, view.TraversedEdges["start"], "chat_node", "start 到 chat_node 的边必须高亮")
	assert.Contains(t, view.TraversedEdges["chat_node"], "end", "chat_node 到 end 的边必须高亮")
	assert.Equal(t, int(model.TaskPassed), view.NodeStatusMap["chat_node"])
}

// BenchmarkFlowxFullProcess 基准测试：单次完整综合流程（启动 -> 或签通过 -> 条件分流 -> 会签双人通过 -> 自动办结）
func BenchmarkFlowxFullProcess(b *testing.B) {
	ctx := context.Background()
	store := memory.NewStorage()
	engine := NewEngine(store)
	def := buildStandardLeaveProcess()
	_, err := engine.Deploy(ctx, def)
	if err != nil {
		b.Fatalf("deploy failed: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		// 1. 启动流程 (days=5)
		instID, err := engine.Start(ctx, StartReq{
			Key:       "leave_process",
			Starter:   "employee_tom",
			Variables: map[string]any{"days": 5},
		})
		if err != nil {
			b.Fatalf("start failed: %v", err)
		}

		// 2. 主管或签通过 (leader_alice)
		tasks, _ := store.ListTasksByInstance(ctx, instID)
		t1, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "leader_alice" && t.IsPending() })
		_ = engine.Pass(ctx, PassReq{TaskID: t1.ID, Operator: "leader_alice"})

		// 3. 总监会签第1人通过 (director_carol)
		tasks, _ = store.ListTasksByInstance(ctx, instID)
		t2, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "director_carol" && t.IsPending() })
		_ = engine.Pass(ctx, PassReq{TaskID: t2.ID, Operator: "director_carol"})

		// 4. 总监会签第2人通过 (director_dave) -> 自动办结
		tasks, _ = store.ListTasksByInstance(ctx, instID)
		t3, _ := lo.Find(tasks, func(t *model.Task) bool { return t.Assignee == "director_dave" && t.IsPending() })
		_ = engine.Pass(ctx, PassReq{TaskID: t3.ID, Operator: "director_dave"})
	}
}
