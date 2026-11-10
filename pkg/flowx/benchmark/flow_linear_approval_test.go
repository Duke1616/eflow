package benchmark

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// TestFlow_LinearApproval 模式一：基础串行审批流 (5节点: Start -> 主管审批 -> 自动化任务 -> 群通知 -> End)
// 单文件内闭环覆盖该流程模式下的典型流转场景：全链路串行推进 / 审批驳回阻断 / 提单人主动撤单
func (s *FlowxBenchmarkTestSuite) TestFlow_LinearApproval() {
	tpl := "linear_approval"

	testCases := []TestCase{
		{
			Name:     "情况1: 主管审批通过 -> 自动驱动自动化执行与群通知 -> 顺利办结归档",
			Template: tpl,
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.AssertActiveTasksCount(instID, 1) // 初始生成 1 个主管待办任务
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.PassNode(instID, "部门主管审批", UserBob)
			},
			WantStatus: model.InstanceFinished,
			WantPassedEdges: []Edge{
				{From: LinearStartNode, To: LinearLeaderNode}, // Start -> 主管审批 (高亮实线)
				{From: LinearLeaderNode, To: LinearAutoNode},  // 主管审批 -> 自动化任务 (高亮实线)
				{From: LinearAutoNode, To: LinearChatNode},    // 自动化任务 -> 群通知 (高亮实线)
				{From: LinearChatNode, To: LinearEndNode},     // 群通知 -> End (高亮实线)
			},
		},
		{
			Name:     "情况2: 主管审批驳回 -> 流程直接终止 (后续自动化与通知连线截断阻断)",
			Template: tpl,
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.RejectNodeTerminate(instID, "部门主管审批", UserBob, "资质不合规，驳回终止")
			},
			WantStatus: model.InstanceRejected,
			WantPassedEdges: []Edge{
				{From: LinearStartNode, To: LinearLeaderNode}, // 审批前的连线高亮
			},
			WantBlockedEdges: []Edge{
				{From: LinearLeaderNode, To: LinearAutoNode}, // 审批驳回后，流向自动化任务的连线阻断不高亮
				{From: LinearAutoNode, To: LinearChatNode},
				{From: LinearChatNode, To: LinearEndNode},
			},
		},
		{
			Name:     "情况3: 审批中发起人主动撤单 -> 任务自动取消 (实例作废)",
			Template: tpl,
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.Revoke(instID, UserAlice, "信息有误，撤回重新提交")
				s.AssertActiveTasksCount(instID, 0) // 待办已清空
			},
			WantStatus: model.InstanceCancelled,
			WantPassedEdges: []Edge{
				{From: LinearStartNode, To: LinearLeaderNode},
			},
			WantBlockedEdges: []Edge{
				{From: LinearLeaderNode, To: LinearAutoNode},
				{From: LinearAutoNode, To: LinearChatNode},
				{From: LinearChatNode, To: LinearEndNode},
			},
		},
	}

	s.ExecuteTableDriven(testCases)
}
