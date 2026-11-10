package benchmark

import (
	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// TestFlow_MultiTechDeploy 模式四：多技术线协同与并行发布流 (14节点: 抄送 + 包容多选 + 并行双人会签 + 自动部署 + 群通知)
// 单文件内闭环覆盖工业级流水线的典型流转场景：单技术线命中与未命中虚线投影 / 会签全员同意穿透 / 审批驳回截断
func (s *FlowxBenchmarkTestSuite) TestFlow_MultiTechDeploy() {
	tpl := "flowx_comprehensive"

	testCases := []TestCase{
		{
			Name:     "情况1: 仅命中后端技术线 -> 抄送自动放行 -> 后端通过 -> 管理层双人会签全通 -> 自动部署与通知 -> 顺利办结",
			Template: tpl,
			Variables: map[string]any{
				FieldScope: ScopeBackend, // 仅命中后端分支 ($scope == 'backend')
			},
			Before: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 抄送提交人瞬间自动放行，任务直接停留在人工审批节点 [后端] (1人待办)
				s.AssertActiveTasksCount(instID, 1)
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 1. 通过第一层：后端审批
				s.PassNode(instID, "后端审批", UserBob)

				// 2. 此时技术线汇聚放行，穿透至第二层：管理层会签（领导 + 产品共 2 人待办）
				s.AssertActiveTasksCount(instID, 2)
				s.PassNode(instID, "领导审批", UserEve)
				s.PassNode(instID, "产品确认", UserFrank)

				// 3. 此时并行汇聚放行，系统自动执行部署并发送群通知，最终归档办结
				s.AssertActiveTasksCount(instID, 0)
			},
			WantStatus: model.InstanceFinished,
			WantPassedEdges: []Edge{
				{From: CompStartNode, To: CompCCNode},         // Start -> 抄送提交人 (高亮实线)
				{From: CompCCNode, To: CompTechForkGW},        // 抄送 -> 技术线分流网关 (高亮实线)
				{From: CompTechForkGW, To: CompBackendNode},   // 分流网关 -> 后端审批 (高亮实线)
				{From: CompBackendNode, To: CompTechJoinGW},   // 后端审批 -> 技术线汇聚 (高亮实线)
				{From: CompTechJoinGW, To: CompManageForkGW},  // 技术线汇聚 -> 会签分流 (高亮实线)
				{From: CompManageForkGW, To: CompLeaderNode},  // 会签分流 -> 领导审批 (高亮实线)
				{From: CompManageForkGW, To: CompProductNode}, // 会签分流 -> 产品确认 (高亮实线)
				{From: CompLeaderNode, To: CompManageJoinGW},  // 领导审批 -> 会签汇聚 (高亮实线)
				{From: CompProductNode, To: CompManageJoinGW}, // 产品确认 -> 会签汇聚 (高亮实线)
				{From: CompManageJoinGW, To: CompDeployNode},  // 会签汇聚 -> 自动化部署 (高亮实线)
				{From: CompDeployNode, To: CompChatNode},      // 自动化部署 -> 群通知 (高亮实线)
				{From: CompChatNode, To: CompEndNode},         // 群通知 -> End (高亮实线)
			},
			WantSkippedEdges: []Edge{
				{From: CompTechForkGW, To: CompAlgoNode},     // 未命中的算法分支 (投影为灰色虚线)
				{From: CompTechForkGW, To: CompFrontendNode}, // 未命中的前端分支 (投影为灰色虚线)
			},
		},
		{
			Name:     "情况2: 管理层双人会签单人审批通过 -> 另一人待办 -> 汇聚等待导致出边严格截断阻断",
			Template: tpl,
			Variables: map[string]any{
				FieldScope: ScopeBackend,
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				// 通过第一层技术线
				s.PassNode(instID, "后端审批", UserBob)

				// 第二层管理层会签中，仅领导通过，产品保持未审
				s.PassNode(instID, "领导审批", UserEve)
				s.AssertActiveTasksCount(instID, 1) // 剩余产品 1 人待办
			},
			WantStatus: model.InstanceRunning,
			WantPassedEdges: []Edge{
				{From: CompManageForkGW, To: CompLeaderNode}, // 领导审批入边点亮
				{From: CompLeaderNode, To: CompManageJoinGW}, // 领导流向会签汇聚网关的入边点亮
			},
			WantBlockedEdges: []Edge{
				{From: CompProductNode, To: CompManageJoinGW}, // 产品未通过
				{From: CompManageJoinGW, To: CompDeployNode},  // 汇聚网关因等待产品到达，出边严格截断阻断 (自动化部署绝对不亮)
				{From: CompDeployNode, To: CompChatNode},
				{From: CompChatNode, To: CompEndNode},
			},
		},
		{
			Name:     "情况3: 后端技术审批人直接驳回终止 -> 流程终止且后续会签与部署连线截断阻断",
			Template: tpl,
			Variables: map[string]any{
				FieldScope: ScopeBackend,
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.RejectNodeTerminate(instID, "后端审批", UserBob, "接口设计缺陷，驳回发布")
				s.AssertActiveTasksCount(instID, 0)
			},
			WantStatus: model.InstanceRejected,
			WantPassedEdges: []Edge{
				{From: CompStartNode, To: CompCCNode},
				{From: CompCCNode, To: CompTechForkGW},
				{From: CompTechForkGW, To: CompBackendNode},
			},
			WantBlockedEdges: []Edge{
				{From: CompBackendNode, To: CompTechJoinGW},  // 驳回节点流向技术线汇聚网关阻断
				{From: CompManageJoinGW, To: CompDeployNode}, // 自动化部署连线阻断
				{From: CompDeployNode, To: CompChatNode},
				{From: CompChatNode, To: CompEndNode},
			},
		},
	}

	s.ExecuteTableDriven(testCases)
}
