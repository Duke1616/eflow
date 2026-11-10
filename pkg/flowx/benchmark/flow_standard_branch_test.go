package benchmark

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// TestFlow_StandardBranch 模式二：条件分支审批流 (5节点: Start -> 排他网关 -> 高风险人工/常规自动放行 -> End)
// 单文件内闭环覆盖该流程模式下的典型流转场景：高风险人工审批 / 常规自动放行与虚线跳过投影 / 节点驳回终止
func (s *FlowxBenchmarkTestSuite) TestFlow_StandardBranch() {
	tpl := "standard_branch"

	testCases := []TestCase{
		{
			Name:     "情况1: 命中高风险变更分支 -> 技术主管审批通过 -> 常规分支虚线跳过 -> 顺利办结",
			Template: tpl,
			Variables: map[string]any{
				FieldLevel: LevelHigh, // 变量命中高风险人工审批分支 ($level in ('high') || $risk >= 3)
				FieldRisk:  3,
			},
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.AssertActiveTasksCount(instID, 1) // 停留在技术主管待办 (1人待办)
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.PassNode(instID, "技术主管审批", UserBob)
			},
			WantStatus: model.InstanceFinished,
			WantPassedEdges: []Edge{
				{From: BranchStartNode, To: BranchGatewayNode},  // Start -> 排他网关 (高亮实线)
				{From: BranchGatewayNode, To: BranchLeaderNode}, // 排他网关 -> 技术主管审批 (高亮实线)
				{From: BranchLeaderNode, To: BranchEndNode},     // 技术主管审批 -> End (高亮实线)
			},
			WantSkippedEdges: []Edge{
				{From: BranchGatewayNode, To: BranchAutoNode}, // 未命中的常规放行分支应投影为虚线
			},
		},
		{
			Name:     "情况2: 命中常规低风险分支 -> 自动化配置同步自动放行 -> 人工分支虚线跳过 -> 顺利办结",
			Template: tpl,
			Variables: map[string]any{
				FieldLevel: LevelLow, // 变量命中常规放行分支 ($level not in ('high'))
			},
			WantStatus: model.InstanceFinished,
			WantPassedEdges: []Edge{
				{From: BranchStartNode, To: BranchGatewayNode}, // Start -> 排他网关 (高亮实线)
				{From: BranchGatewayNode, To: BranchAutoNode},  // 排他网关 -> 自动化配置同步 (高亮实线)
				{From: BranchAutoNode, To: BranchEndNode},      // 自动化配置同步 -> End (高亮实线)
			},
			WantSkippedEdges: []Edge{
				{From: BranchGatewayNode, To: BranchLeaderNode}, // 未命中的高风险人工审批分支应投影为虚线
			},
		},
		{
			Name:     "情况3: 高风险审批中技术主管驳回 -> 流程直接终止且后续连线截断阻断",
			Template: tpl,
			Variables: map[string]any{
				FieldLevel: LevelHigh,
				FieldRisk:  3,
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.RejectNodeTerminate(instID, "技术主管审批", UserBob, "架构方案不合理，驳回修改")
				s.AssertActiveTasksCount(instID, 0)
			},
			WantStatus: model.InstanceRejected,
			WantPassedEdges: []Edge{
				{From: BranchStartNode, To: BranchGatewayNode},
				{From: BranchGatewayNode, To: BranchLeaderNode},
			},
			WantBlockedEdges: []Edge{
				{From: BranchLeaderNode, To: BranchEndNode}, // 驳回后流向 End 的连线阻断
			},
		},
	}

	s.ExecuteTableDriven(testCases)
}
