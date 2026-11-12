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
		{
			Name:     "情况4: 领导驳回回退至后端重审 -> 已退出的管理层与部署整段熄灭 -> 未命中技术线虚线保留",
			Template: tpl,
			Variables: map[string]any{
				FieldScope: ScopeBackend,
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.PassNode(instID, "后端审批", UserBob)
				s.AssertActiveTasksCount(instID, 2)

				// 驳回候选只含起点与真实审批通过的后端；被跳过的算法/前端严禁入选
				s.AssertRejectTargets(instID, "领导审批", []string{CompStartNode, CompBackendNode})

				// 领导默认回溯驳回：回到最近一层真实通过的后端，产品确认同批次联动取消
				s.RejectNode(instID, "领导审批", UserEve, "发布窗口不合适，打回后端重审")
				s.AssertActiveTasksCount(instID, 1)
			},
			WantStatus: model.InstanceRunning,
			WantPassedEdges: []Edge{
				{From: CompStartNode, To: CompCCNode},
				{From: CompCCNode, To: CompTechForkGW},
				{From: CompTechForkGW, To: CompBackendNode}, // 回退目标重新待办，入边保持点亮
			},
			WantSkippedEdges: []Edge{
				{From: CompTechForkGW, To: CompAlgoNode},
				{From: CompTechForkGW, To: CompFrontendNode},
				{From: CompAlgoNode, To: CompTechJoinGW},
				{From: CompFrontendNode, To: CompTechJoinGW},
			},
			WantBlockedEdges: []Edge{
				// 后端等待重审，入边阻断；汇聚屏障不再满足，其出边与整个下游必须熄灭。
				// 曾出现缺陷：跳过分支被当作已完成通路喂通汇聚，形成"出边亮、入边黑"的不可能拓扑
				{From: CompBackendNode, To: CompTechJoinGW},
				{From: CompTechJoinGW, To: CompManageForkGW},
				{From: CompManageForkGW, To: CompLeaderNode},
				{From: CompManageForkGW, To: CompProductNode},
				{From: CompLeaderNode, To: CompManageJoinGW},
				{From: CompProductNode, To: CompManageJoinGW},
				{From: CompManageJoinGW, To: CompDeployNode},
				{From: CompDeployNode, To: CompChatNode},
				{From: CompChatNode, To: CompEndNode},
			},
		},
		{
			Name:     "情况5: 领导定向驳回至提单人 -> 开始节点重新待办 -> 全图连线归零等待重报",
			Template: tpl,
			Variables: map[string]any{
				FieldScope: ScopeBackend,
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.PassNode(instID, "后端审批", UserBob)
				s.RejectNodeTo(instID, "领导审批", UserEve, "需求描述不清，退回提单人重写", CompStartNode)
				s.AssertActiveTasksCount(instID, 1)
			},
			WantStatus: model.InstanceRunning,
			WantBlockedEdges: []Edge{
				{From: CompStartNode, To: CompCCNode}, // 提单人尚未重新提交，起点出边不得点亮
				{From: CompCCNode, To: CompTechForkGW},
				{From: CompTechForkGW, To: CompBackendNode},
				{From: CompTechJoinGW, To: CompManageForkGW},
				{From: CompManageJoinGW, To: CompDeployNode},
				{From: CompChatNode, To: CompEndNode},
			},
		},
		{
			Name:     "情况6: 领导驳回回退 -> 后端重审再次通过 -> 汇聚重新开闸并回流至管理层会签",
			Template: tpl,
			Variables: map[string]any{
				FieldScope: ScopeBackend,
			},
			Run: func(s *FlowxBenchmarkTestSuite, instID string) {
				s.PassNode(instID, "后端审批", UserBob)
				s.RejectNode(instID, "领导审批", UserEve, "打回后端重审")

				// 回退瞬间下游必须先熄灭，避免修复反向过头把正常流转也一并阻断
				retreated := s.getDecoratedCanvas(tpl, instID)
				s.AssertEdgeNotTraversed(retreated, CompTechJoinGW, CompManageForkGW)
				s.AssertEdgeNotTraversed(retreated, CompManageForkGW, CompLeaderNode)

				// 后端重审通过，汇聚屏障重新满足，流程再次推进到管理层会签
				s.PassNode(instID, "后端审批", UserBob)
				s.AssertActiveTasksCount(instID, 2)
			},
			WantStatus: model.InstanceRunning,
			WantPassedEdges: []Edge{
				{From: CompBackendNode, To: CompTechJoinGW},
				{From: CompTechJoinGW, To: CompManageForkGW},
				{From: CompManageForkGW, To: CompLeaderNode},
				{From: CompManageForkGW, To: CompProductNode},
			},
			WantBlockedEdges: []Edge{
				{From: CompManageJoinGW, To: CompDeployNode}, // 会签尚未全通，部署仍须阻断
			},
		},
	}

	s.ExecuteTableDriven(testCases)
}
