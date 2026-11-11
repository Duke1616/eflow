package easyflow

import (
	"context"
	"testing"

	"github.com/Bunny3th/easy-workflow/workflow/model"
	repoMocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCalculateTraversedEdges_GatewayConvergenceBlocking(t *testing.T) {
	// 复现用户遇到的实际线上发版拓扑场景：
	// start -> submitter -> split_gw -> [backend(skip), frontend(pending), algo(pending)] -> join_gw -> [product, leader]
	const (
		nodeStart     = "node_start"
		nodeSubmitter = "node_submitter"
		nodeSplitGW   = "node_split_gw"
		nodeBackend   = "node_backend"
		nodeFrontend  = "node_frontend"
		nodeAlgo      = "node_algo"
		nodeJoinGW    = "node_join_gw"
		nodeProduct   = "node_product"
		nodeLeader    = "node_leader"
	)

	processDef := model.Process{
		ProcessName: "测试流程",
		Nodes: []model.Node{
			{
				NodeID:   nodeStart,
				NodeType: model.RootNode,
			},
			{
				NodeID:      nodeSubmitter,
				NodeType:    model.TaskNode,
				PrevNodeIDs: []string{nodeStart},
			},
			{
				NodeID:      nodeSplitGW,
				NodeType:    model.GateWayNode,
				PrevNodeIDs: []string{nodeSubmitter},
				GWConfig: model.HybridGateway{
					WaitForAllPrevNode: 1,
				},
			},
			{
				NodeID:      nodeBackend,
				NodeType:    model.TaskNode,
				PrevNodeIDs: []string{nodeSplitGW},
			},
			{
				NodeID:      nodeFrontend,
				NodeType:    model.TaskNode,
				PrevNodeIDs: []string{nodeSplitGW},
			},
			{
				NodeID:      nodeAlgo,
				NodeType:    model.TaskNode,
				PrevNodeIDs: []string{nodeSplitGW},
			},
			{
				NodeID:      nodeJoinGW,
				NodeType:    model.GateWayNode,
				PrevNodeIDs: []string{nodeBackend, nodeFrontend, nodeAlgo},
				GWConfig: model.HybridGateway{
					WaitForAllPrevNode: 1, // 并行/包容汇聚，必须等待所有前驱
				},
			},
			{
				NodeID:      nodeProduct,
				NodeType:    model.TaskNode,
				PrevNodeIDs: []string{nodeJoinGW},
			},
			{
				NodeID:      nodeLeader,
				NodeType:    model.TaskNode,
				PrevNodeIDs: []string{nodeJoinGW},
			},
		},
	}

	testCases := []struct {
		name       string
		tasks      []model.Task
		assertFunc func(t *testing.T, edges map[string][]string)
	}{
		{
			name: "后端被跳过但前端与算法仍在审批中，汇聚网关下游绝不能提前点亮",
			tasks: []model.Task{
				{
					NodeID:     nodeSubmitter,
					Status:     1,
					IsFinished: 1,
					BatchCode:  "b1",
					PrevNodeID: nodeStart,
				},
				{
					NodeID:     nodeBackend,
					Status:     5, // 条件不满足跳过
					IsFinished: 1,
					BatchCode:  "b2",
					PrevNodeID: nodeSplitGW,
				},
				{
					NodeID:     nodeFrontend,
					Status:     0, // 审批中
					IsFinished: 0,
					BatchCode:  "b2",
					PrevNodeID: nodeSplitGW,
				},
				{
					NodeID:     nodeAlgo,
					Status:     0, // 审批中
					IsFinished: 0,
					BatchCode:  "b2",
					PrevNodeID: nodeSplitGW,
				},
			},
			assertFunc: func(t *testing.T, edges map[string][]string) {
				// 1. 上游与分支路径应正常流转
				assert.Contains(t, edges[nodeStart], nodeSubmitter)
				assert.Contains(t, edges[nodeSubmitter], nodeSplitGW)
				assert.Contains(t, edges[nodeSplitGW], nodeFrontend)
				assert.Contains(t, edges[nodeSplitGW], nodeAlgo)

				// 2. 后端跳过流转可以到达汇聚网关
				assert.Contains(t, edges[nodeBackend], nodeJoinGW)

				// 3. 关键断言：汇聚网关未完成汇聚，下游的产品和领导绝不能被穿透点亮！
				assert.NotContains(t, edges, nodeJoinGW, "汇聚网关未满足汇聚条件，不能存在流出边")
				assert.NotContains(t, edges[nodeJoinGW], nodeProduct)
				assert.NotContains(t, edges[nodeJoinGW], nodeLeader)
			},
		},
		{
			name: "前端与算法均审批通过且底层已生成产品与领导任务，汇聚网关下游正常点亮",
			tasks: []model.Task{
				{
					NodeID:     nodeSubmitter,
					Status:     1,
					IsFinished: 1,
					BatchCode:  "b1",
					PrevNodeID: nodeStart,
				},
				{
					NodeID:     nodeBackend,
					Status:     5, // 跳过
					IsFinished: 1,
					BatchCode:  "b2",
					PrevNodeID: nodeSplitGW,
				},
				{
					NodeID:     nodeFrontend,
					Status:     1, // 通过
					IsFinished: 1,
					BatchCode:  "b2",
					PrevNodeID: nodeSplitGW,
				},
				{
					NodeID:     nodeAlgo,
					Status:     1, // 通过
					IsFinished: 1,
					BatchCode:  "b2",
					PrevNodeID: nodeSplitGW,
				},
				{
					NodeID:     nodeProduct,
					Status:     0, // 已生成待办
					IsFinished: 0,
					BatchCode:  "b3",
					PrevNodeID: nodeJoinGW,
				},
				{
					NodeID:     nodeLeader,
					Status:     0, // 已生成待办
					IsFinished: 0,
					BatchCode:  "b3",
					PrevNodeID: nodeJoinGW,
				},
			},
			assertFunc: func(t *testing.T, edges map[string][]string) {
				// 此时汇聚网关下游由已生成的任务关联点亮
				assert.Contains(t, edges[nodeJoinGW], nodeProduct)
				assert.Contains(t, edges[nodeJoinGW], nodeLeader)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			repo := repoMocks.NewMockIEngineRepository(ctrl)
			repo.EXPECT().
				GetProcessDefineByVersion(gomock.Any(), 101, 1).
				Return(processDef, nil)

			edges, err := calculateTraversedEdges(context.Background(), repo, tc.tasks, 101, 1)
			require.NoError(t, err)
			tc.assertFunc(t, edges)
		})
	}
}
