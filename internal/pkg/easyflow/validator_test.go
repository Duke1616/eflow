package easyflow

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidator_TableDriven 采用表驱动统一测试流程图校验器的所有正向与反向规则
func TestValidator_TableDriven(t *testing.T) {
	testCases := []struct {
		name      string
		workflow  Workflow
		wantErr   bool
		wantErrKw string // 预期的错误关键词（当 wantErr 为 true 时生效）
	}{
		// ─────────────────────────────────────────────────────────────────────
		// 一、正向合法场景（预期无错误）
		// ─────────────────────────────────────────────────────────────────────
		{
			name: "合法流程: 基础串行流程 (Start -> User -> End)",
			workflow: Workflow{
				Name: "基础串行",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start-1", "type": "start"},
						{"id": "user-1", "type": "user", "properties": map[string]interface{}{"name": "审批", "rule": "appoint", "approved": []string{"admin"}}},
						{"id": "end-1", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start-1", "targetNodeId": "user-1"},
						{"id": "e2", "sourceNodeId": "user-1", "targetNodeId": "end-1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "合法流程: 并行网关标准成对闭合 (Fork -> [A, B] -> Join -> End)",
			workflow: Workflow{
				Name: "并行成对闭合",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start-1", "type": "start"},
						{"id": "fork-1", "type": "parallel"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "技术审批", "rule": "appoint", "approved": []string{"tech"}}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "财务审批", "rule": "appoint", "approved": []string{"finance"}}},
						{"id": "join-1", "type": "parallel"},
						{"id": "end-1", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start-1", "targetNodeId": "fork-1"},
						{"id": "e2", "sourceNodeId": "fork-1", "targetNodeId": "user-a"},
						{"id": "e3", "sourceNodeId": "fork-1", "targetNodeId": "user-b"},
						{"id": "e4", "sourceNodeId": "user-a", "targetNodeId": "join-1"},
						{"id": "e5", "sourceNodeId": "user-b", "targetNodeId": "join-1"},
						{"id": "e6", "sourceNodeId": "join-1", "targetNodeId": "end-1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "合法流程: selective + condition 子网关 (各子分支均配置表达式且汇聚)",
			workflow: Workflow{
				Name: "条件并行合法流程",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start-1", "type": "start"},
						{"id": "selective-1", "type": "selective"},
						{"id": "cond-a", "type": "condition"},
						{"id": "cond-b", "type": "condition"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "后端", "rule": "appoint", "approved": []string{"yangwz"}}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "前端", "rule": "appoint", "approved": []string{"luankz"}}},
						{"id": "join-1", "type": "parallel"},
						{"id": "end-1", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start-1", "targetNodeId": "selective-1"},
						{"id": "e2", "sourceNodeId": "selective-1", "targetNodeId": "cond-a"},
						{"id": "e3", "sourceNodeId": "selective-1", "targetNodeId": "cond-b"},
						{"id": "e4", "sourceNodeId": "cond-a", "targetNodeId": "user-a", "properties": map[string]interface{}{"expression": "$env = 'backend'"}},
						{"id": "e5", "sourceNodeId": "cond-b", "targetNodeId": "user-b", "properties": map[string]interface{}{"expression": "$env = 'frontend'"}},
						{"id": "e6", "sourceNodeId": "user-a", "targetNodeId": "join-1"},
						{"id": "e7", "sourceNodeId": "user-b", "targetNodeId": "join-1"},
						{"id": "e8", "sourceNodeId": "join-1", "targetNodeId": "end-1"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "合法流程: 排他条件分支分别流向不同终点 (非汇聚死锁模式)",
			workflow: Workflow{
				Name: "排他分支流向终点",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start-1", "type": "start"},
						{"id": "user-1", "type": "user", "properties": map[string]interface{}{"name": "发起", "rule": "appoint", "approved": []string{"admin"}}},
						{"id": "cond-1", "type": "condition"},
						{"id": "user-pass", "type": "user", "properties": map[string]interface{}{"name": "快速通过", "rule": "appoint", "approved": []string{"leader"}}},
						{"id": "user-audit", "type": "user", "properties": map[string]interface{}{"name": "普通审核", "rule": "appoint", "approved": []string{"auditor"}}},
						{"id": "end-1", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start-1", "targetNodeId": "user-1"},
						{"id": "e2", "sourceNodeId": "user-1", "targetNodeId": "cond-1"},
						{"id": "e3", "sourceNodeId": "cond-1", "targetNodeId": "user-pass", "properties": map[string]interface{}{"expression": "$score >= 90"}},
						{"id": "e4", "sourceNodeId": "cond-1", "targetNodeId": "user-audit", "properties": map[string]interface{}{"expression": "$score < 90"}},
						{"id": "e5", "sourceNodeId": "user-pass", "targetNodeId": "end-1"},
						{"id": "e6", "sourceNodeId": "user-audit", "targetNodeId": "end-1"},
					},
				},
			},
			wantErr: false,
		},

		// ─────────────────────────────────────────────────────────────────────
		// 二、条件网关表达式规则测试
		// ─────────────────────────────────────────────────────────────────────
		{
			name: "条件网关规则: 多分支出边未配置表达式报错",
			workflow: Workflow{
				Name: "多分支缺表达式",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "cond", "type": "condition"},
						{"id": "user1", "type": "user", "properties": map[string]interface{}{"name": "用户1"}},
						{"id": "user2", "type": "user", "properties": map[string]interface{}{"name": "用户2"}},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "cond"},
						{"id": "e2", "sourceNodeId": "cond", "targetNodeId": "user1", "properties": map[string]interface{}{"expression": "$a=1"}},
						{"id": "e3", "sourceNodeId": "cond", "targetNodeId": "user2"}, // 缺少 expression
						{"id": "e4", "sourceNodeId": "user1", "targetNodeId": "end"},
						{"id": "e5", "sourceNodeId": "user2", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "未配置条件表达式",
		},
		{
			name: "条件网关规则: 单出边（如 selective 子网关）未配置表达式报错",
			workflow: Workflow{
				Name: "单出边条件子网关缺表达式",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "sel", "type": "selective"},
						{"id": "cond-a", "type": "condition"},
						{"id": "cond-b", "type": "condition"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "后端"}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "前端"}},
						{"id": "join", "type": "parallel"},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "sel"},
						{"id": "e2", "sourceNodeId": "sel", "targetNodeId": "cond-a"},
						{"id": "e3", "sourceNodeId": "sel", "targetNodeId": "cond-b"},
						{"id": "e4", "sourceNodeId": "cond-a", "targetNodeId": "user-a", "properties": map[string]interface{}{"expression": "$env='backend'"}},
						{"id": "e5", "sourceNodeId": "cond-b", "targetNodeId": "user-b"}, // 单出边，未配表达式
						{"id": "e6", "sourceNodeId": "user-a", "targetNodeId": "join"},
						{"id": "e7", "sourceNodeId": "user-b", "targetNodeId": "join"},
						{"id": "e8", "sourceNodeId": "join", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "未配置条件表达式",
		},

		// ─────────────────────────────────────────────────────────────────────
		// 三、并行网关 Fork-Join 成对闭合规则测试
		// ─────────────────────────────────────────────────────────────────────
		{
			name: "并行网关规则: 并行分叉未汇聚直接流向结束节点报错",
			workflow: Workflow{
				Name: "并行未汇聚直接流向End",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "fork", "type": "parallel"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "审批A"}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "审批B"}},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "fork"},
						{"id": "e2", "sourceNodeId": "fork", "targetNodeId": "user-a"},
						{"id": "e3", "sourceNodeId": "fork", "targetNodeId": "user-b"},
						{"id": "e4", "sourceNodeId": "user-a", "targetNodeId": "end"}, // 直接流向 End
						{"id": "e5", "sourceNodeId": "user-b", "targetNodeId": "end"}, // 直接流向 End
					},
				},
			},
			wantErr:   true,
			wantErrKw: "成对闭合",
		},
		{
			name: "并行网关规则: 并行分叉分支未汇聚至同一个聚合网关报错",
			workflow: Workflow{
				Name: "并行分支分散汇聚",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "fork", "type": "parallel"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "分支1"}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "分支2"}},
						{"id": "join-a", "type": "parallel"},
						{"id": "join-b", "type": "parallel"},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "fork"},
						{"id": "e2", "sourceNodeId": "fork", "targetNodeId": "user-a"},
						{"id": "e3", "sourceNodeId": "fork", "targetNodeId": "user-b"},
						{"id": "e4", "sourceNodeId": "user-a", "targetNodeId": "join-a"},
						{"id": "e5", "sourceNodeId": "user-b", "targetNodeId": "join-b"}, // 分散到不同聚合点
						{"id": "e6", "sourceNodeId": "join-a", "targetNodeId": "end"},
						{"id": "e7", "sourceNodeId": "join-b", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "未汇聚到同一个聚合网关",
		},

		// ─────────────────────────────────────────────────────────────────────
		// 四、基础元素与拓扑结构校验测试
		// ─────────────────────────────────────────────────────────────────────
		{
			name: "元素规则: 不受系统支持的节点类型报错",
			workflow: Workflow{
				Name: "未知类型",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n1", "type": "start"},
						{"id": "n2", "type": "invalid_node_type"},
						{"id": "n3", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "n1", "targetNodeId": "n2"},
						{"id": "e2", "sourceNodeId": "n2", "targetNodeId": "n3"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "不受系统支持",
		},
		{
			name: "元素规则: 连线起点不存在报错",
			workflow: Workflow{
				Name: "起点不存在",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n1", "type": "start"},
						{"id": "n2", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "non_exist_src", "targetNodeId": "n2"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "起点节点 [non_exist_src] 不存在",
		},
		{
			name: "元素规则: 连线终点不存在报错",
			workflow: Workflow{
				Name: "终点不存在",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n1", "type": "start"},
						{"id": "n2", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "n1", "targetNodeId": "non_exist_dst"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "目标节点 [non_exist_dst] 不存在",
		},
		{
			name: "元素规则: 节点自环连线报错",
			workflow: Workflow{
				Name: "自环连线",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n1", "type": "start"},
						{"id": "n2", "type": "user", "properties": map[string]interface{}{"name": "审批节点"}},
						{"id": "n3", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "n1", "targetNodeId": "n2"},
						{"id": "e2", "sourceNodeId": "n2", "targetNodeId": "n2"}, // 自环
						{"id": "e3", "sourceNodeId": "n2", "targetNodeId": "n3"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "自环",
		},
		{
			name: "容错规则: 节点间存在重复连线时自动幂等去重并成功转换",
			workflow: Workflow{
				Name: "重复连线自动去重",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n1", "type": "start"},
						{"id": "n2", "type": "user", "properties": map[string]interface{}{"name": "审批节点"}},
						{"id": "n3", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "n1", "targetNodeId": "n2"},
						{"id": "e2", "sourceNodeId": "n1", "targetNodeId": "n2"}, // 重复连线，自动去重
						{"id": "e3", "sourceNodeId": "n2", "targetNodeId": "n3"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "边界规则: 缺少开始节点报错",
			workflow: Workflow{
				Name: "缺少开始节点",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "n2", "type": "user", "properties": map[string]interface{}{"name": "审批"}},
						{"id": "n3", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "n2", "targetNodeId": "n3"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "缺少开始节点",
		},
		{
			name: "边界规则: 存在多个开始节点报错",
			workflow: Workflow{
				Name: "多开始节点",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start-a", "type": "start"},
						{"id": "start-b", "type": "start"},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start-a", "targetNodeId": "end"},
						{"id": "e2", "sourceNodeId": "start-b", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "多个开始节点",
		},
		{
			name: "边界规则: 开始节点有前置入边报错",
			workflow: Workflow{
				Name: "开始节点有入边",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "user", "type": "user", "properties": map[string]interface{}{"name": "审批"}},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "user"},
						{"id": "e2", "sourceNodeId": "user", "targetNodeId": "end"},
						{"id": "e3", "sourceNodeId": "user", "targetNodeId": "start"}, // 倒流入边
					},
				},
			},
			wantErr:   true,
			wantErrKw: "开始节点",
		},
		{
			name: "边界规则: 缺少结束节点报错",
			workflow: Workflow{
				Name: "缺少结束节点",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "user", "type": "user", "properties": map[string]interface{}{"name": "审批"}},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "user"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "缺少结束节点",
		},
		{
			name: "边界规则: 结束节点有后置出边报错",
			workflow: Workflow{
				Name: "结束节点有出边",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "end-1", "type": "end"},
						{"id": "user-extra", "type": "user", "properties": map[string]interface{}{"name": "多余审批"}},
						{"id": "end-2", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "end-1"},
						{"id": "e2", "sourceNodeId": "end-1", "targetNodeId": "user-extra"}, // 出边
						{"id": "e3", "sourceNodeId": "user-extra", "targetNodeId": "end-2"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "结束节点",
		},
		{
			name: "图结构规则: 回路死循环检测报错（提取环路路径）",
			workflow: Workflow{
				Name: "回路死循环",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "userA", "type": "user", "properties": map[string]interface{}{"name": "节点A"}},
						{"id": "userB", "type": "user", "properties": map[string]interface{}{"name": "节点B"}},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "userA"},
						{"id": "e2", "sourceNodeId": "userA", "targetNodeId": "userB"},
						{"id": "e3", "sourceNodeId": "userB", "targetNodeId": "userA"}, // 循环 A <-> B
						{"id": "e4", "sourceNodeId": "userB", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "死循环环路",
		},
		{
			name: "连通性规则: 游离孤岛节点（正向无法到达）报错",
			workflow: Workflow{
				Name: "游离孤岛",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "user-main", "type": "user", "properties": map[string]interface{}{"name": "主线审批"}},
						{"id": "user-island", "type": "user", "properties": map[string]interface{}{"name": "孤岛节点"}},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "user-main"},
						{"id": "e2", "sourceNodeId": "user-main", "targetNodeId": "end"},
						{"id": "e3", "sourceNodeId": "user-island", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "无法从开始节点到达",
		},
		{
			name: "连通性规则: 死胡同节点（无法流向结束节点）报错",
			workflow: Workflow{
				Name: "死胡同",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "fork", "type": "parallel"},
						{"id": "user-ok", "type": "user", "properties": map[string]interface{}{"name": "正常审批"}},
						{"id": "user-dead", "type": "user", "properties": map[string]interface{}{"name": "断头审批"}},
						{"id": "join", "type": "parallel"},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "fork"},
						{"id": "e2", "sourceNodeId": "fork", "targetNodeId": "user-ok"},
						{"id": "e3", "sourceNodeId": "fork", "targetNodeId": "user-dead"},
						{"id": "e4", "sourceNodeId": "user-ok", "targetNodeId": "join"},
						// user-dead 悬空未汇聚到 join 也未流向 end
						{"id": "e5", "sourceNodeId": "join", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "无法流转到任何结束节点",
		},
		{
			name: "死锁防护规则: 单条件网关多分支汇聚至并行网关导致死锁报错",
			workflow: Workflow{
				Name: "条件并行汇聚死锁",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "cond", "type": "condition"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "分支A"}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "分支B"}},
						{"id": "join", "type": "parallel"},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "cond"},
						{"id": "e2", "sourceNodeId": "cond", "targetNodeId": "user-a", "properties": map[string]interface{}{"expression": "$env = 'a'"}},
						{"id": "e3", "sourceNodeId": "cond", "targetNodeId": "user-b", "properties": map[string]interface{}{"expression": "$env = 'b'"}},
						{"id": "e4", "sourceNodeId": "user-a", "targetNodeId": "join"},
						{"id": "e5", "sourceNodeId": "user-b", "targetNodeId": "join"},
						{"id": "e6", "sourceNodeId": "join", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "无效拓扑",
		},
		{
			name: "Selective规则: selective出边直连非condition节点报错",
			workflow: Workflow{
				Name: "selective直连非condition",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "sel", "type": "selective"},
						{"id": "user-a", "type": "user", "properties": map[string]interface{}{"name": "审批1"}},
						{"id": "user-b", "type": "user", "properties": map[string]interface{}{"name": "审批2"}},
						{"id": "join", "type": "parallel"},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "sel"},
						{"id": "e2", "sourceNodeId": "sel", "targetNodeId": "user-a"},
						{"id": "e3", "sourceNodeId": "sel", "targetNodeId": "user-b"},
						{"id": "e4", "sourceNodeId": "user-a", "targetNodeId": "join"},
						{"id": "e5", "sourceNodeId": "user-b", "targetNodeId": "join"},
						{"id": "e6", "sourceNodeId": "join", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "直接连接了非 condition 节点",
		},
		{
			name: "Selective规则: selective出边少于2条报错",
			workflow: Workflow{
				Name: "selective仅1条出边",
				FlowData: LogicFlow{
					Nodes: []map[string]interface{}{
						{"id": "start", "type": "start"},
						{"id": "sel", "type": "selective"},
						{"id": "cond", "type": "condition"},
						{"id": "user", "type": "user", "properties": map[string]interface{}{"name": "审批"}},
						{"id": "end", "type": "end"},
					},
					Edges: []map[string]interface{}{
						{"id": "e1", "sourceNodeId": "start", "targetNodeId": "sel"},
						{"id": "e2", "sourceNodeId": "sel", "targetNodeId": "cond"},
						{"id": "e3", "sourceNodeId": "cond", "targetNodeId": "user", "properties": map[string]interface{}{"expression": "$x=1"}},
						{"id": "e4", "sourceNodeId": "user", "targetNodeId": "end"},
					},
				},
			},
			wantErr:   true,
			wantErrKw: "只有 1 条出边，至少需要 2 条",
		},
	}

	converter := NewDefaultConverterWithHandlers()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := converter.Convert(tc.workflow)
			if tc.wantErr {
				require.Error(t, err, "用例 [%s] 应当拦截报错", tc.name)
				assert.True(t, strings.Contains(err.Error(), tc.wantErrKw),
					"用例 [%s] 返回错误应该包含 [%s]，实际错误: %v", tc.name, tc.wantErrKw, err)
			} else {
				require.NoError(t, err, "用例 [%s] 应当通过校验，实际报错: %v", tc.name, err)
			}
		})
	}
}
