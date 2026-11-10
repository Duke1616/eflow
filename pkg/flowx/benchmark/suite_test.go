package benchmark

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	compiler "github.com/Duke1616/eflow/internal/pkg/compiler/flowx"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
	"github.com/Duke1616/eflow/pkg/flowx"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/suite"
)

// ─────────────────────────────────────────────────────────────────────────────
// 4 大官方标准示范形态的节点业务语义常量 (稳定易读，让人一眼看懂拓扑)
// ─────────────────────────────────────────────────────────────────────────────

const (
	// 模式一：linear_approval (基础串行审批流: 5节点)
	LinearStartNode  = "linear-node-1" // 开始
	LinearLeaderNode = "linear-node-2" // 部门主管审批 (人工审批)
	LinearAutoNode   = "linear-node-3" // 自动化任务执行 (系统自动执行)
	LinearChatNode   = "linear-node-4" // 结果群通知 (IM广播)
	LinearEndNode    = "linear-node-5" // 结束

	// 模式二：standard_branch (条件分支审批流: 5节点)
	BranchStartNode   = "branch-node-1" // 开始
	BranchGatewayNode = "branch-node-2" // 排他网关 (条件分流)
	BranchLeaderNode  = "branch-node-3" // 技术主管审批 (高风险人工审批)
	BranchAutoNode    = "branch-node-4" // 自动化配置同步 (常规自动放行)
	BranchEndNode     = "branch-node-5" // 结束

	// 模式三：flowx_or_branch (多人竞争审批流 / 任一人通过即向下: 6节点)
	OrStartNode     = "or-branch-node-1" // 开始
	OrForkGateway   = "or-branch-node-2" // 包容分流网关
	OrApproverANode = "or-branch-node-3" // 审批人 A (并发派发)
	OrApproverBNode = "or-branch-node-4" // 审批人 B (并发派发)
	OrJoinGateway   = "or-branch-node-5" // 排他汇聚网关 (任一人通过即向下)
	OrEndNode       = "or-branch-node-6" // 结束

	// 模式四：flowx_comprehensive (多技术线协同与并行发布流: 14节点)
	CompStartNode    = "comprehensive-node-1"  // 开始
	CompCCNode       = "comprehensive-node-2"  // 抄送提交人 (自动放行)
	CompTechForkGW   = "comprehensive-node-3"  // 包容分流网关 (技术线分流: 后端/算法/前端)
	CompBackendNode  = "comprehensive-node-4"  // 人工审批: "后端"
	CompAlgoNode     = "comprehensive-node-5"  // 人工审批: "算法"
	CompFrontendNode = "comprehensive-node-6"  // 人工审批: "前端"
	CompTechJoinGW   = "comprehensive-node-7"  // 包容汇聚网关 (技术线汇聚)
	CompManageForkGW = "comprehensive-node-8"  // 并行分流网关 (管理层会签分流)
	CompLeaderNode   = "comprehensive-node-9"  // 人工审批: "领导审批" (双人会签)
	CompProductNode  = "comprehensive-node-10" // 人工审批: "产品确认" (双人会签)
	CompManageJoinGW = "comprehensive-node-11" // 并行汇聚网关 (管理层会签汇聚)
	CompDeployNode   = "comprehensive-node-12" // 自动化任务: "自动化-部署"
	CompChatNode     = "comprehensive-node-13" // 消息通知: "群通知"
	CompEndNode      = "comprehensive-node-14" // 办结归档
)

// ─────────────────────────────────────────────────────────────────────────────
// 标准测试角色常量 (全英文字典标准，彻底杜绝 IDE 拼写波浪线与无用法警告)
// ─────────────────────────────────────────────────────────────────────────────

const (
	UserAlice = "alice" // 默认提单发起人
	UserBob   = "bob"   // 主力审批人 / 技术负责人
	UserCarol = "carol" // 并行或签 / 会签审批人
	UserDave  = "dave"  // 算法负责 / 会签协同人
	UserEve   = "eve"   // 人事行政 / 部门主管审批人
	UserFrank = "frank" // 终审负责人 / 老板审批人
)

// ─────────────────────────────────────────────────────────────────────────────
// 表单业务变量与分支路由常量 (精准匹配 LogicFlow 条件表达式，消除魔术字符串)
// ─────────────────────────────────────────────────────────────────────────────

const (
	// 模式二：条件分支流程 (standard_branch) 表单字段与风险等级值
	FieldLevel = "level" // 风险等级字段: $level in ('high') || $risk >= 3
	FieldRisk  = "risk"  // 风险数值字段
	LevelHigh  = "high"  // 高风险
	LevelLow   = "low"   // 常规低风险

	// 模式四：多技术线协同流程 (flowx_comprehensive) 表单字段与技术线分支值
	FieldScope    = "scope"    // 技术审批范围字段: $scope == 'backend' / 'algo' / 'frontend'
	ScopeBackend  = "backend"  // 后端技术线
	ScopeAlgo     = "algo"     // 算法技术线
	ScopeFrontend = "frontend" // 前端技术线
)

// Edge 语义化连线断言元组 [Source -> Target]
type Edge struct {
	From string
	To   string
}

// TestCase 流程模型表驱动测试用例结构 (声明环境 -> 驱动流转 -> 期望校验)
type TestCase struct {
	Name      string         // 业务场景中文描述
	Template  string         // 使用的真实 LogicFlow 模板名称
	Variables map[string]any // 初始提单表单变量

	// 1. Before: 发起后前置断言 (可选)
	Before func(s *FlowxBenchmarkTestSuite, instID string)

	// 2. Run: 模拟业务操作流 (按步骤执行审批/驳回/撤销)
	Run func(s *FlowxBenchmarkTestSuite, instID string)

	// 3. After: 期望结果声明式校验
	WantStatus       model.InstanceStatus // 期望流程实例终态
	WantPassedEdges  []Edge               // 期望高亮流转通过的边 (实线)
	WantSkippedEdges []Edge               // 期望因条件未命中跳过的边 (虚线)
	WantBlockedEdges []Edge               // 期望未流转/阻断截断的边 (灰线)
}

// ProcessFixture 快照定义与前端画布数据缓存
type ProcessFixture struct {
	Def       model.ProcessDefinition
	RawCanvas map[string]any
}

// FlowxBenchmarkTestSuite FlowX 真实常用流程模式基准测试套件
type FlowxBenchmarkTestSuite struct {
	suite.Suite
	ctx       context.Context
	fixtures  map[string]*ProcessFixture
	engine    flowx.Engine
	storage   *memory.Storage
	converter compiler.IConverter
}

// SetupSuite 全局初始化：一次性解析并编译所有真实 LogicFlow 数据，保障编译幂等与零重复开销
func (s *FlowxBenchmarkTestSuite) SetupSuite() {
	s.ctx = context.Background()
	s.fixtures = make(map[string]*ProcessFixture)
	s.converter = compiler.NewConverter()

	// 载入并编译前端示范模板库 4 大标准流程
	s.loadFixture("linear_approval", "testdata/linear_approval.json", "基础串行审批流")
	s.loadFixture("standard_branch", "testdata/standard_branch.json", "条件分支审批流")
	s.loadFixture("flowx_or_branch", "testdata/flowx_or_branch.json", "多人竞争审批流")
	s.loadFixture("flowx_comprehensive", "testdata/flowx_comprehensive.json", "多技术线协同与并行发布流")
}

// SetupTest 每个场景用例执行前隔离重置纯内存存储，确保测试无状态污染
func (s *FlowxBenchmarkTestSuite) SetupTest() {
	s.storage = memory.NewStorage()
	s.engine = flowx.NewEngine(s.storage)

	// 注册通用自动化 Worker，支撑自动化部署与配置同步节点自动完成推进
	s.engine.RegisterWorker("automation", func(ctx context.Context, tc *flowx.TaskContext) (*flowx.TaskResult, error) {
		return &flowx.TaskResult{
			Variables: map[string]any{"auto_executed": true},
		}, nil
	})
}

func (s *FlowxBenchmarkTestSuite) loadFixture(key, jsonPath, name string) {
	rawBytes, err := os.ReadFile(jsonPath)
	s.Require().NoError(err, "读取基准测试 LogicFlow 数据失败: %s", jsonPath)

	var rawCanvas map[string]any
	err = json.Unmarshal(rawBytes, &rawCanvas)
	s.Require().NoError(err, "反序列化 LogicFlow 画布失败: %s", jsonPath)

	graph, err := flowdsl.Parse(rawBytes)
	s.Require().NoError(err, "解析 LogicFlow 拓扑图失败: %s", jsonPath)

	def, err := s.converter.Convert(key, name, graph)
	s.Require().NoError(err, "编译 LogicFlow 为 FlowX 定义快照失败: %s", jsonPath)

	s.fixtures[key] = &ProcessFixture{
		Def:       def,
		RawCanvas: rawCanvas,
	}
}

// ExecuteTableDriven 表驱动用例统一执行器
func (s *FlowxBenchmarkTestSuite) ExecuteTableDriven(testCases []TestCase) {
	for _, tc := range testCases {
		s.Run(tc.Name, func() {
			// 1. 发起流程实例 (默认使用标准发起人 UserAlice: alice)
			instID := s.startInstance(tc.Template, UserAlice, tc.Variables)

			// 2. 执行前置检查 (Before)
			if tc.Before != nil {
				tc.Before(s, instID)
			}

			// 3. 驱动流转动作 (Run)
			if tc.Run != nil {
				tc.Run(s, instID)
			}

			// 4. 统一断言终态 (After)
			s.AssertInstanceStatus(instID, tc.WantStatus)

			// 5. 统一断言高亮通过连线 (实线)
			for _, edge := range tc.WantPassedEdges {
				s.AssertEdgePassed(tc.Template, instID, edge.From, edge.To)
			}

			// 6. 统一断言跳过连线 (虚线)
			for _, edge := range tc.WantSkippedEdges {
				s.AssertEdgeSkipped(tc.Template, instID, edge.From, edge.To)
			}

			// 7. 统一断言未流转/阻断连线 (灰线)
			for _, edge := range tc.WantBlockedEdges {
				s.AssertEdgeNotTraversed(tc.Template, instID, edge.From, edge.To)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 操作辅助函数
// ─────────────────────────────────────────────────────────────────────────────

func (s *FlowxBenchmarkTestSuite) startInstance(templateKey string, starter string, vars map[string]any) string {
	fixture, exists := s.fixtures[templateKey]
	s.Require().True(exists, "未定义的流程模板夹具: %s", templateKey)

	instID, err := s.engine.Start(s.ctx, flowx.StartReq{
		Def:       fixture.Def,
		Key:       fixture.Def.Key,
		Starter:   starter,
		Variables: vars,
	})
	s.Require().NoError(err, "流程实例启动失败")
	return instID
}

// PassNode 按展示名称和处理人通过审批节点
func (s *FlowxBenchmarkTestSuite) PassNode(instID string, nodeName string, operator string) {
	tasks, err := s.engine.ListPendingTasksByInstances(s.ctx, []string{instID})
	s.Require().NoError(err)

	// 优先匹配节点名且指派人等于 operator 的任务；若未指定则匹配该节点名
	targetTask, found := lo.Find(tasks, func(tk *model.Task) bool {
		return tk.NodeName == nodeName && (tk.Assignee == operator || operator == "")
	})
	if !found {
		targetTask, found = lo.Find(tasks, func(tk *model.Task) bool { return tk.NodeName == nodeName })
	}
	s.Require().True(found, "未找到待审批节点: %s (操作人: %s, 当前活跃: %v)", nodeName, operator, lo.Map(tasks, func(t *model.Task, _ int) string { return t.NodeName }))

	err = s.engine.Pass(s.ctx, flowx.PassReq{
		TaskID:   targetTask.ID,
		Operator: operator,
	})
	s.Require().NoError(err, "节点审批通过失败: %s (操作人: %s)", nodeName, operator)
}

// RejectNodeTerminate 按展示名称直接驳回终止流程
func (s *FlowxBenchmarkTestSuite) RejectNodeTerminate(instID string, nodeName string, operator string, comment string) {
	tasks, err := s.engine.ListPendingTasksByInstances(s.ctx, []string{instID})
	s.Require().NoError(err)

	targetTask, found := lo.Find(tasks, func(tk *model.Task) bool {
		return tk.NodeName == nodeName && (tk.Assignee == operator || operator == "")
	})
	if !found {
		targetTask, found = lo.Find(tasks, func(tk *model.Task) bool { return tk.NodeName == nodeName })
	}
	s.Require().True(found, "未找到待审批节点: %s (操作人: %s, 当前活跃: %v)", nodeName, operator, lo.Map(tasks, func(t *model.Task, _ int) string { return t.NodeName }))

	err = s.engine.Reject(s.ctx, flowx.RejectReq{
		TaskID:    targetTask.ID,
		Operator:  operator,
		Comment:   comment,
		Terminate: true,
	})
	s.Require().NoError(err, "节点驳回终止失败: %s (操作人: %s)", nodeName, operator)
}

// Revoke 发起人撤回流程实例
func (s *FlowxBenchmarkTestSuite) Revoke(instID string, operator string, reason string) {
	err := s.engine.Revoke(s.ctx, flowx.RevokeReq{
		InstanceID: instID,
		Operator:   operator,
		Comment:    reason,
	})
	s.Require().NoError(err, "提单人主动撤单失败")
}

// ─────────────────────────────────────────────────────────────────────────────
// 断言辅助函数
// ─────────────────────────────────────────────────────────────────────────────

func (s *FlowxBenchmarkTestSuite) AssertInstanceStatus(instID string, expected model.InstanceStatus) {
	inst, err := s.engine.GetInstance(s.ctx, instID)
	s.Require().NoError(err)
	s.Equal(expected, inst.Status, "流程实例状态与预期不符")
}

func (s *FlowxBenchmarkTestSuite) AssertActiveTasksCount(instID string, expected int) {
	tasks, err := s.engine.ListPendingTasksByInstances(s.ctx, []string{instID})
	s.Require().NoError(err)
	s.Len(tasks, expected, "待办任务数与预期不符")
}

func (s *FlowxBenchmarkTestSuite) AssertEdgePassed(templateKey, instID, source, target string) {
	canvas := s.getDecoratedCanvas(templateKey, instID)
	edge := s.findCanvasEdge(canvas, source, target)
	s.Require().NotNil(edge, "画布中未找到连线: %s -> %s", source, target)

	props, _ := edge["properties"].(map[string]any)
	s.True(props != nil && props["is_pass"] == true, "连线 %s -> %s 应被高亮通过 (is_pass 需为 true)", source, target)
}

func (s *FlowxBenchmarkTestSuite) AssertEdgeSkipped(templateKey, instID, source, target string) {
	canvas := s.getDecoratedCanvas(templateKey, instID)
	edge := s.findCanvasEdge(canvas, source, target)
	s.Require().NotNil(edge, "画布中未找到连线: %s -> %s", source, target)

	props, _ := edge["properties"].(map[string]any)
	s.True(props != nil && props["is_skipped"] == true, "连线 %s -> %s 应被投影为跳过虚线 (is_skipped 需为 true)", source, target)
}

func (s *FlowxBenchmarkTestSuite) AssertEdgeNotTraversed(templateKey, instID, source, target string) {
	canvas := s.getDecoratedCanvas(templateKey, instID)
	edge := s.findCanvasEdge(canvas, source, target)
	s.Require().NotNil(edge, "画布中未找到连线: %s -> %s", source, target)

	props, _ := edge["properties"].(map[string]any)
	if props != nil {
		s.False(props["is_pass"] == true, "连线 %s -> %s 未流转，is_pass 不能为 true", source, target)
		s.False(props["is_skipped"] == true, "连线 %s -> %s 未流转，is_skipped 不能为 true", source, target)
	}
}

func (s *FlowxBenchmarkTestSuite) getDecoratedCanvas(templateKey, instID string) map[string]any {
	fixture := s.fixtures[templateKey]
	s.Require().NotNil(fixture)

	view, err := s.engine.GetExecutionView(s.ctx, instID)
	s.Require().NoError(err)

	canvas, err := flowdsl.HighlightCanvas(fixture.RawCanvas, flowdsl.ExecutionTrace{
		TraversedEdges: view.TraversedEdges,
		NodeStatusMap:  view.NodeStatusMap,
	})
	s.Require().NoError(err)
	return canvas
}

func (s *FlowxBenchmarkTestSuite) findCanvasEdge(canvas map[string]any, source, target string) map[string]any {
	edges, ok := canvas["edges"].([]any)
	if !ok {
		return nil
	}
	for _, raw := range edges {
		e, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		sNode, _ := e["sourceNodeId"].(string)
		tNode, _ := e["targetNodeId"].(string)
		if sNode == source && tNode == target {
			return e
		}
	}
	return nil
}

func TestFlowxBenchmarkTestSuite(t *testing.T) {
	suite.Run(t, new(FlowxBenchmarkTestSuite))
}
