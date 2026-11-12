package benchmark

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// TestFlow_OrCompetition 模式三：多人竞争审批流 (6节点: 包容分流 + 审批人 A/B 并发派发 + 排他汇聚 + End)
// 单文件内闭环覆盖经典的“任一人通过即向下完成”的竞争审批机制：A 先通过办结 / B 先通过办结 / 提单人撤回作废
func (s *FlowxBenchmarkTestSuite) TestFlow_OrCompetition() {
	tpl := "flowx_or_branch"

	testCases := []TestCase{
		{
			Name:     "情况1: 任务并发派发 A 与 B -> 审批人 A 率先通过 -> 排他网关放行直接办结 -> 审批人 B 待办作废",
			Template: tpl,
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 并发派发给审批人 A 和审批人 B，共 2 个活跃待办任务
				s.AssertActiveTasksCount(instID, 2)
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 审批人 A 率先点击通过
				s.PassNode(instID, "审批人 A", UserBob)
				// 此时排他汇聚网关触发，流程直接越过汇聚放行至 End，待办立即清零
				s.AssertActiveTasksCount(instID, 0)
			},
			WantStatus: model.InstanceFinished,
			WantPassedEdges: []Edge{
				{From: OrStartNode, To: OrForkGateway},     // Start -> 包容分流网关
				{From: OrForkGateway, To: OrApproverANode}, // 分流 -> 审批人 A
				{From: OrApproverANode, To: OrJoinGateway}, // 审批人 A -> 排他汇聚网关
				{From: OrJoinGateway, To: OrEndNode},       // 排他汇聚网关 -> End
			},
		},
		{
			Name:     "情况2: 任务并发派发 A 与 B -> 审批人 B 率先通过 -> 排他网关放行直接办结 -> 审批人 A 待办作废",
			Template: tpl,
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.AssertActiveTasksCount(instID, 2)
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 审批人 B 率先点击通过
				s.PassNode(instID, "审批人 B", UserCarol)
				s.AssertActiveTasksCount(instID, 0)
			},
			WantStatus: model.InstanceFinished,
			WantPassedEdges: []Edge{
				{From: OrStartNode, To: OrForkGateway},     // Start -> 包容分流网关
				{From: OrForkGateway, To: OrApproverBNode}, // 分流 -> 审批人 B
				{From: OrApproverBNode, To: OrJoinGateway}, // 审批人 B -> 排他汇聚网关
				{From: OrJoinGateway, To: OrEndNode},       // 排他汇聚网关 -> End
			},
		},
		{
			Name:     "情况3: 并发审批中发起人主动撤单 -> 全量分支待办自动取消 (实例作废)",
			Template: tpl,
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.AssertActiveTasksCount(instID, 2)
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.Revoke(instID, UserAlice, "提错工单，撤回重新提交")
				s.AssertActiveTasksCount(instID, 0) // 并发待办全部清零
			},
			WantStatus: model.InstanceCancelled,
			WantPassedEdges: []Edge{
				{From: OrStartNode, To: OrForkGateway},
			},
			WantBlockedEdges: []Edge{
				{From: OrJoinGateway, To: OrEndNode},
			},
		},
		{
			Name:     "情况4: 并发待办中审批人 A 驳回不终止 -> 兄弟 B 待办联动取消 -> 回退提单人重新编辑",
			Template: tpl,
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.AssertActiveTasksCount(instID, 2)
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 竞争分支尚未有人通过，驳回候选只能追溯到提单起点
				s.AssertRejectTargets(instID, "审批人 A", []string{OrStartNode})

				s.RejectNode(instID, "审批人 A", UserBob, "方案不成立，退回提单人")
				s.AssertActiveTasksCount(instID, 1) // B 的同批次待办被联动取消，只剩提单人重报
			},
			WantStatus: model.InstanceRunning,
			WantBlockedEdges: []Edge{
				{From: OrStartNode, To: OrForkGateway}, // 提单人未重新提交，起点出边归零
				{From: OrForkGateway, To: OrApproverANode},
				{From: OrForkGateway, To: OrApproverBNode},
				{From: OrApproverANode, To: OrJoinGateway},
				{From: OrApproverBNode, To: OrJoinGateway},
				{From: OrJoinGateway, To: OrEndNode},
			},
		},
	}

	s.ExecuteTableDriven(testCases)
}
