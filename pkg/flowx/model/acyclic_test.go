package model

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func startNode(id string) NodeSpec { return NodeSpec{ID: id, Name: "发起", Type: NodeStart} }
func endNode(id string) NodeSpec   { return NodeSpec{ID: id, Name: "办结", Type: NodeEnd} }
func approvalNode(id, name string) NodeSpec {
	return NodeSpec{ID: id, Name: name, Type: NodeApproval, SignRule: SignOneVote, Assignees: []string{"alice"}}
}

// buildDef 按给定节点顺序与边顺序组装定义，顺序即报错路径的确定性来源
func buildDef(nodes []NodeSpec, edges [][2]string) ProcessDefinition {
	specs := make([]EdgeSpec, 0, len(edges))
	for _, e := range edges {
		specs = append(specs, EdgeSpec{Source: e[0], Target: e[1]})
	}
	return ProcessDefinition{Key: "cycle_probe", Name: "环检测探针", Version: 1, Nodes: nodes, Edges: specs}
}

func TestProcessDefinition_ValidateAcyclic(t *testing.T) {
	testCases := []struct {
		name string
		def  ProcessDefinition
		// wantCyclePath 非空表示必须拒绝，且报错须含该环路路径
		wantCyclePath string
		wantErrPart   string
	}{
		{
			name: "双审批节点成环_即工单永不办结的原始缺陷画布",
			def: buildDef(
				[]NodeSpec{startNode("s"), approvalNode("a", "审批A"), approvalNode("b", "审批B"), endNode("e")},
				[][2]string{{"s", "a"}, {"a", "b"}, {"b", "a"}, {"b", "e"}},
			),
			wantCyclePath: "审批A → 审批B → 审批A",
		},
		{
			name: "三节点长环",
			def: buildDef(
				[]NodeSpec{startNode("s"), approvalNode("a", "一审"), approvalNode("b", "二审"),
					approvalNode("c", "三审"), endNode("e")},
				[][2]string{{"s", "a"}, {"a", "b"}, {"b", "c"}, {"c", "a"}, {"c", "e"}},
			),
			wantCyclePath: "一审 → 二审 → 三审 → 一审",
		},
		{
			name: "并行网关成环",
			def: buildDef(
				[]NodeSpec{startNode("s"), {ID: "gw1", Name: "网关一", Type: NodeGatewayParallel},
					{ID: "gw2", Name: "网关二", Type: NodeGatewayParallel}, endNode("e")},
				[][2]string{{"s", "gw1"}, {"gw1", "gw2"}, {"gw2", "gw1"}, {"gw2", "e"}},
			),
			wantCyclePath: "网关一 → 网关二 → 网关一",
		},
		{
			name: "抄送瞬时穿透节点成环",
			def: buildDef(
				[]NodeSpec{startNode("s"), {ID: "c1", Name: "抄送一", Type: NodeCarbonCopy},
					{ID: "c2", Name: "抄送二", Type: NodeCarbonCopy}, endNode("e")},
				[][2]string{{"s", "c1"}, {"c1", "c2"}, {"c2", "c1"}, {"c2", "e"}},
			),
			wantCyclePath: "抄送一 → 抄送二 → 抄送一",
		},
		{
			name: "环上游存在合法前缀_只报残留子图内的环",
			def: buildDef(
				[]NodeSpec{startNode("s"), approvalNode("x", "前置审批"), approvalNode("a", "审批A"),
					approvalNode("b", "审批B"), endNode("e")},
				[][2]string{{"s", "x"}, {"x", "a"}, {"a", "b"}, {"b", "a"}, {"b", "e"}},
			),
			wantCyclePath: "审批A → 审批B → 审批A",
		},
		{
			name: "残留子图首节点是无出边的尾巴_反向游走不得断头",
			def: buildDef(
				// 故意把 end 排在环成员之前，使其成为残留子图的首个节点
				[]NodeSpec{startNode("s"), endNode("e"), approvalNode("a", "审批A"), approvalNode("b", "审批B")},
				[][2]string{{"s", "a"}, {"a", "b"}, {"b", "a"}, {"b", "e"}},
			),
			wantCyclePath: "审批B → 审批A → 审批B",
		},
		{
			name: "两个独立环_按定义顺序取首个保证报错确定",
			def: buildDef(
				[]NodeSpec{startNode("s"), approvalNode("a", "甲一"), approvalNode("b", "甲二"),
					approvalNode("c", "乙一"), approvalNode("d", "乙二"), endNode("e")},
				[][2]string{{"s", "a"}, {"a", "b"}, {"b", "a"}, {"s", "c"}, {"c", "d"}, {"d", "c"},
					{"b", "e"}, {"d", "e"}},
			),
			wantCyclePath: "甲一 → 甲二 → 甲一",
		},
		{
			name: "节点无展示名时回落节点ID",
			def: buildDef(
				[]NodeSpec{startNode("s"), {ID: "a", Type: NodeApproval}, {ID: "b", Type: NodeApproval}, endNode("e")},
				[][2]string{{"s", "a"}, {"a", "b"}, {"b", "a"}, {"b", "e"}},
			),
			wantCyclePath: "a → b → a",
		},
		{
			name: "自环仍由前置校验优先拦截_报错口径不变",
			def: buildDef(
				[]NodeSpec{startNode("s"), approvalNode("a", "审批A"), endNode("e")},
				[][2]string{{"s", "a"}, {"a", "a"}, {"a", "e"}},
			),
			wantErrPart: "存在无效的自环连接",
		},
		{
			name: "合法线性链不得误伤",
			def: buildDef(
				[]NodeSpec{startNode("s"), approvalNode("a", "一审"), approvalNode("b", "二审"), endNode("e")},
				[][2]string{{"s", "a"}, {"a", "b"}, {"b", "e"}},
			),
		},
		{
			name: "菱形并行汇聚不得误伤_汇聚节点入度大于1但不是环",
			def: buildDef(
				[]NodeSpec{startNode("s"), {ID: "fork", Name: "分支", Type: NodeGatewayParallel},
					approvalNode("a", "甲审"), approvalNode("b", "乙审"),
					{ID: "join", Name: "汇聚", Type: NodeGatewayParallel}, endNode("e")},
				[][2]string{{"s", "fork"}, {"fork", "a"}, {"fork", "b"}, {"a", "join"}, {"b", "join"}, {"join", "e"}},
			),
		},
		{
			name: "排他网关条件分支不得误伤",
			def: buildDef(
				[]NodeSpec{startNode("s"), {ID: "gw", Name: "分支", Type: NodeGatewayExclusive},
					approvalNode("a", "甲审"), approvalNode("b", "乙审"), endNode("e")},
				[][2]string{{"s", "gw"}, {"gw", "a"}, {"gw", "b"}, {"a", "e"}, {"b", "e"}},
			),
		},
		{
			name: "多结束节点不得误伤",
			def: buildDef(
				[]NodeSpec{startNode("s"), {ID: "gw", Name: "分支", Type: NodeGatewayInclusive},
					approvalNode("a", "甲审"), approvalNode("b", "乙审"), endNode("e1"), endNode("e2")},
				[][2]string{{"s", "gw"}, {"gw", "a"}, {"gw", "b"}, {"a", "e1"}, {"b", "e2"}},
			),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.def.Validate()
			switch {
			case tc.wantCyclePath != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantCyclePath)
				assert.Contains(t, err.Error(), "工作流必须是有向无环图（DAG）")
			case tc.wantErrPart != "":
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErrPart)
			default:
				assert.NoError(t, err)
			}
		})
	}
}

// TestProcessDefinition_ValidateAcyclicLongChain 长自动链不得被环检测或深度护栏误伤
func TestProcessDefinition_ValidateAcyclicLongChain(t *testing.T) {
	const chainLen = 200
	nodes := []NodeSpec{startNode("s")}
	edges := [][2]string{}
	prev := "s"
	for i := 0; i < chainLen; i++ {
		id := fmt.Sprintf("cc%03d", i)
		nodes = append(nodes, NodeSpec{ID: id, Name: id, Type: NodeCarbonCopy})
		edges = append(edges, [2]string{prev, id})
		prev = id
	}
	nodes = append(nodes, endNode("e"))
	edges = append(edges, [2]string{prev, "e"})

	assert.NoError(t, buildDef(nodes, edges).Validate())
}

// TestProcessDefinition_CyclePathDeterministic 环路报错路径必须与 map 遍历顺序无关
func TestProcessDefinition_CyclePathDeterministic(t *testing.T) {
	def := buildDef(
		[]NodeSpec{startNode("s"), approvalNode("a", "甲一"), approvalNode("b", "甲二"),
			approvalNode("c", "乙一"), approvalNode("d", "乙二"), endNode("e")},
		[][2]string{{"s", "a"}, {"a", "b"}, {"b", "a"}, {"s", "c"}, {"c", "d"}, {"d", "c"},
			{"b", "e"}, {"d", "e"}},
	)

	first := def.Validate()
	require.Error(t, first)
	for i := 0; i < 100; i++ {
		require.Equal(t, first.Error(), def.Validate().Error(), "第 %d 次报错路径漂移", i)
	}
}
