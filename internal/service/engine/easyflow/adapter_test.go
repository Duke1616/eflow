package easyflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/database"
	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	compiler "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
	repoMocks "github.com/Duke1616/eflow/internal/repository/mocks"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/glebarez/sqlite"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"
)

func TestToEasyVariables(t *testing.T) {
	testCases := []struct {
		name       string
		variables  map[string]any
		starter    string
		ticketId   int64
		assertFunc func(t *testing.T, rawJSON string)
	}{
		{
			name: "正常字典映射并且包含starter与ticket_id",
			variables: map[string]any{
				"amount": 100,
				"reason": "办公用品采购",
			},
			starter:  "ZhangSan",
			ticketId: 1001,
			assertFunc: func(t *testing.T, rawJSON string) {
				var vars []model.Variable
				err := json.Unmarshal([]byte(rawJSON), &vars)
				require.NoError(t, err)

				m := make(map[string]string)
				for _, v := range vars {
					m[v.Key] = v.Value
				}
				assert.Equal(t, "100", m["amount"])
				assert.Equal(t, "办公用品采购", m["reason"])
				assert.Equal(t, "ZhangSan", m["starter"])
				assert.Equal(t, "1001", m["ticket_id"])
			},
		},
		{
			name: "变量中已存在starter时不重复追加",
			variables: map[string]any{
				"starter": "custom_user",
			},
			starter:  "ZhangSan",
			ticketId: 0,
			assertFunc: func(t *testing.T, rawJSON string) {
				var vars []model.Variable
				err := json.Unmarshal([]byte(rawJSON), &vars)
				require.NoError(t, err)

				starterCount := 0
				for _, v := range vars {
					if v.Key == "starter" {
						starterCount++
						assert.Equal(t, "custom_user", v.Value)
					}
				}
				assert.Equal(t, 1, starterCount)
			},
		},
		{
			name:      "全空时返回空切片JSON",
			variables: nil,
			starter:   "",
			ticketId:  0,
			assertFunc: func(t *testing.T, rawJSON string) {
				assert.Equal(t, "[]", rawJSON)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			vars := ensureEssentialVariables(tc.variables, tc.starter, tc.ticketId)
			raw := toEasyVariables(vars)
			tc.assertFunc(t, raw)
		})
	}
}

func TestAdapter_Revoke(t *testing.T) {
	testCases := []struct {
		name       string
		req        domain.RevokeReq
		setupMock  func(m *repoMocks.MockIEngineRepository)
		wantErr    bool
		errContain string
	}{
		{
			name: "非法流程实例ID报错",
			req: domain.RevokeReq{
				InstanceID: domain.NewID(0),
			},
			setupMock:  func(m *repoMocks.MockIEngineRepository) {},
			wantErr:    true,
			errContain: "easyflow 流程实例 ID 非法",
		},
		{
			name: "实例不存在时幂等返回成功",
			req: domain.RevokeReq{
				InstanceID: domain.NewID(9999),
				Operator:   "test_user",
			},
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 9999).Return(domain.Instance{}, gorm.ErrRecordNotFound)
			},
			wantErr: false,
		},
		{
			name: "实例ID为0的空对象时幂等返回成功",
			req: domain.RevokeReq{
				InstanceID: domain.NewID(8888),
				Operator:   "test_user",
			},
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 8888).Return(domain.Instance{ProcInstID: 0, ProcID: 0}, nil)
			},
			wantErr: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repoMocks.NewMockIEngineRepository(ctrl)
			tc.setupMock(mockRepo)

			engineAdapter := NewEngine(mockRepo)
			err := engineAdapter.Revoke(context.Background(), tc.req)

			if tc.wantErr {
				require.Error(t, err)
				if tc.errContain != "" {
					assert.Contains(t, err.Error(), tc.errContain)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestAdapter_GetActiveReject 覆盖驳回横幅的展示与消隐时机。
// easyflow 的 BatchCode 是引擎用 SELECT UUID() 生成的随机串，不具轮次顺序语义，
// 因此"最近一次驳回"必须按办结时间锁定，闭环必须按自增 TaskID 判定先后。
func TestAdapter_GetActiveReject(t *testing.T) {
	localTime := func(s string) *database.LocalTime {
		parsed, err := time.Parse("2006-01-02 15:04:05", s)
		require.NoError(t, err)
		v := database.LocalTime(parsed)
		return &v
	}

	testCases := []struct {
		name        string
		tasks       []model.Task
		wantShow    bool
		wantNode    string
		wantComment string
	}{
		{
			name: "并行双节点自由驳回_两人都待办时意见可见",
			tasks: []model.Task{
				{TaskID: 1, NodeID: "backend", NodeName: "后端审批", UserID: "bob", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:00:00")},
				{TaskID: 2, NodeID: "algo", NodeName: "算法审批", UserID: "dave", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:01:00")},
				{TaskID: 3, NodeID: "product", NodeName: "产品审批", UserID: "carol", BatchCode: "aaaaaaaa-1111", Status: 2, IsFinished: 1, Comment: "算法侧口径不对，请重算", FinishedTime: localTime("2026-11-12 10:02:00")},
				{TaskID: 4, NodeID: "backend", NodeName: "后端审批", UserID: "bob", BatchCode: "bbbbbbbb-2222", Status: 0, IsFinished: 0},
				{TaskID: 5, NodeID: "algo", NodeName: "算法审批", UserID: "dave", BatchCode: "bbbbbbbb-2222", Status: 0, IsFinished: 0},
			},
			wantShow: true, wantNode: "product", wantComment: "算法侧口径不对，请重算",
		},
		{
			name: "并行双节点自由驳回_一人提交不得抹掉另一人的意见",
			tasks: []model.Task{
				{TaskID: 1, NodeID: "backend", NodeName: "后端审批", UserID: "bob", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:00:00")},
				{TaskID: 2, NodeID: "algo", NodeName: "算法审批", UserID: "dave", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:01:00")},
				{TaskID: 3, NodeID: "product", NodeName: "产品审批", UserID: "carol", BatchCode: "aaaaaaaa-1111", Status: 2, IsFinished: 1, Comment: "算法侧口径不对，请重算", FinishedTime: localTime("2026-11-12 10:02:00")},
				{TaskID: 4, NodeID: "backend", NodeName: "后端审批", UserID: "bob", BatchCode: "bbbbbbbb-2222", Status: 1, IsFinished: 1, Comment: "同意", FinishedTime: localTime("2026-11-12 11:00:00")},
				{TaskID: 5, NodeID: "algo", NodeName: "算法审批", UserID: "dave", BatchCode: "bbbbbbbb-2222", Status: 0, IsFinished: 0},
			},
			wantShow: true, wantNode: "product", wantComment: "算法侧口径不对，请重算",
		},
		{
			name: "两轮驳回_UUID字典序与时间序相反时仍取最近一轮",
			tasks: []model.Task{
				{TaskID: 10, NodeID: "leader", NodeName: "主管审批", UserID: "bob", BatchCode: "ffffffff-9999", Status: 2, IsFinished: 1, Comment: "第一轮驳回", FinishedTime: localTime("2026-11-12 09:00:00")},
				{TaskID: 20, NodeID: "product", NodeName: "产品审批", UserID: "carol", BatchCode: "00000000-0000", Status: 2, IsFinished: 1, Comment: "第二轮驳回", FinishedTime: localTime("2026-11-12 12:00:00")},
				{TaskID: 21, NodeID: "leader", NodeName: "主管审批", UserID: "bob", BatchCode: "11111111-1111", Status: 0, IsFinished: 0},
			},
			wantShow: true, wantNode: "product", wantComment: "第二轮驳回",
		},
		{
			name: "单目标驳回_已走回驳回节点则横幅消隐",
			tasks: []model.Task{
				{TaskID: 1, NodeID: "submitter", NodeName: "提单人录入", UserID: "alice", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:00:00")},
				{TaskID: 2, NodeID: "auditor", NodeName: "主管审批", UserID: "bob", BatchCode: "aaaaaaaa-1111", Status: 2, IsFinished: 1, Comment: "资料不全", FinishedTime: localTime("2026-11-12 10:02:00")},
				{TaskID: 3, NodeID: "submitter", NodeName: "提单人录入", UserID: "alice", BatchCode: "bbbbbbbb-2222", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 11:00:00")},
				{TaskID: 4, NodeID: "auditor", NodeName: "主管审批", UserID: "bob", BatchCode: "bbbbbbbb-2222", Status: 0, IsFinished: 0},
			},
			wantShow: false,
		},
		{
			name: "从未发生驳回_返回空快照",
			tasks: []model.Task{
				{TaskID: 1, NodeID: "submitter", NodeName: "提单人录入", UserID: "alice", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:00:00")},
				{TaskID: 2, NodeID: "auditor", NodeName: "主管审批", UserID: "bob", BatchCode: "aaaaaaaa-1111", Status: 0, IsFinished: 0},
			},
			wantShow: false,
		},
		{
			name: "驳回后流程已终结_无待办则横幅消隐",
			tasks: []model.Task{
				{TaskID: 1, NodeID: "submitter", NodeName: "提单人录入", UserID: "alice", BatchCode: "aaaaaaaa-1111", Status: 1, IsFinished: 1, FinishedTime: localTime("2026-11-12 10:00:00")},
				{TaskID: 2, NodeID: "auditor", NodeName: "主管审批", UserID: "bob", BatchCode: "aaaaaaaa-1111", Status: 2, IsFinished: 1, Comment: "资质不合规，驳回终止", FinishedTime: localTime("2026-11-12 10:02:00")},
			},
			wantShow: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repoMocks.NewMockIEngineRepository(ctrl)
			mockRepo.EXPECT().ListAccessibleTaskRecord(gomock.Any(), 100, 0, 1000).Return(tc.tasks, nil)
			mockRepo.EXPECT().CountAccessibleTaskRecord(gomock.Any(), 100).Return(int64(len(tc.tasks)), nil)

			snap, err := NewEngine(mockRepo).GetActiveReject(context.Background(), domain.NewID(100))
			require.NoError(t, err)

			if !tc.wantShow {
				assert.Nil(t, snap)
				return
			}
			require.NotNil(t, snap)
			assert.Equal(t, tc.wantNode, snap.NodeID)
			assert.Equal(t, tc.wantComment, snap.Comment)
		})
	}
}

// TestAdapter_GetPriorComments 验证 easyflow 侧把过滤、排序与截断交给 SQL，
// 驱动只按仓储给定的 limit 取数并做字段映射与时间格式化，不再全量拉取任务链
func TestAdapter_GetPriorComments(t *testing.T) {
	localTime := func(s string) *database.LocalTime {
		parsed, err := time.Parse("2006-01-02 15:04:05", s)
		require.NoError(t, err)
		v := database.LocalTime(parsed)
		return &v
	}

	testCases := []struct {
		name         string
		instID       domain.ID
		limit        int
		nodes        []model.Node // 实例锁定版本的定义拓扑，缺省为单个普通审批节点
		wantExclude  []string     // 期望下推给 SQL 的机器节点排除集
		tasks        []model.Task
		total        int64
		listErr      error
		countErr     error
		wantErr      bool
		wantErrIs    error
		wantTotal    int64
		wantComments []string
		wantTimes    []string
		wantFirst    *domain.PriorComment
	}{
		{
			name:   "按仓储返回序映射字段并格式化办结时间",
			instID: domain.NewID(100),
			limit:  5,
			tasks: []model.Task{
				{TaskID: 21, NodeID: "product", NodeName: "产品审批", UserID: "bob", Status: 2, Comment: "口径不对请重算", FinishedTime: localTime("2026-11-11 10:00:00")},
				{TaskID: 20, NodeID: "backend", NodeName: "后端审批", UserID: "alice", Status: 1, Comment: "构建通过", FinishedTime: localTime("2026-11-09 09:00:00")},
			},
			total:        8,
			wantTotal:    8,
			wantComments: []string{"口径不对请重算", "构建通过"},
			wantTimes:    []string{"2026-11-11 10:00:00", "2026-11-09 09:00:00"},
			wantFirst: &domain.PriorComment{
				NodeID: "product", NodeName: "产品审批",
				ApprovedBy: "bob", Status: 2,
				Comment: "口径不对请重算", OccurredAt: "2026-11-11 10:00:00",
			},
		},
		{
			name:   "办结时间缺失时归一为空串",
			instID: domain.NewID(100),
			limit:  5,
			tasks: []model.Task{
				{TaskID: 30, NodeID: "node", NodeName: "环节", UserID: "alice", Status: 1, Comment: "同意", FinishedTime: nil},
			},
			total:        1,
			wantTotal:    1,
			wantComments: []string{"同意"},
			wantTimes:    []string{""},
		},
		{
			name:         "无可读意见时返回空摘要",
			instID:       domain.NewID(100),
			limit:        5,
			tasks:        []model.Task{},
			total:        0,
			wantTotal:    0,
			wantComments: []string{},
		},
		{
			name:    "列表查询失败时携带根因返回",
			instID:  domain.NewID(100),
			limit:   5,
			listErr: errors.New("db down"),
			wantErr: true,
		},
		{
			name:     "计数查询失败时携带根因返回",
			instID:   domain.NewID(100),
			limit:    5,
			countErr: errors.New("db down"),
			wantErr:  true,
		},
		{
			name:      "非法流程实例ID直接拒绝_不触达仓储",
			instID:    domain.NewID(0),
			limit:     5,
			wantErr:   true,
			wantErrIs: engine.ErrInvalidParameter,
		},
		{
			name:   "抄送群聊自动化与系统代理节点全部下推排除_审批与起点保留",
			instID: domain.NewID(100),
			limit:  5,
			nodes: []model.Node{
				// 起点节点无 StartEvents：驳回回退后退化成普通任务节点，承载提单人真实意见，不得排除
				{NodeID: "start", NodeName: "提单", NodeType: model.RootNode,
					UserIDs: []string{compiler.UserStarter}, NodeEndEvents: []string{compiler.EventStart}},
				{NodeID: "leader", NodeName: "主管审批", NodeType: model.TaskNode,
					UserIDs: []string{"alice"}, NodeStartEvents: []string{compiler.EventNotify}},
				// 抄送解析到真实处理人，靠处理人哨兵识别不了，只能靠事件标记
				{NodeID: "cc", NodeName: "抄送知会", NodeType: model.TaskNode,
					UserIDs: []string{"carol"}, NodeStartEvents: []string{compiler.EventCarbonCopy}},
				{NodeID: "chat", NodeName: "群通知", NodeType: model.TaskNode,
					UserIDs: []string{compiler.ChatGroupApproval}, NodeStartEvents: []string{compiler.EventChatGroup}},
				{NodeID: "auto", NodeName: "自动化作业", NodeType: model.TaskNode,
					UserIDs: []string{compiler.AutomationApproval}, NodeStartEvents: []string{compiler.EventAutomation}},
				// 系统代理节点挂的是普通 EventNotify，只有占位处理人能识别
				{NodeID: "proxy", NodeName: "系统代理流转", NodeType: model.TaskNode,
					UserIDs: []string{compiler.SysAutoUser}, NodeStartEvents: []string{compiler.EventNotify}},
				{NodeID: "end", NodeName: "结束", NodeType: model.EndNode,
					NodeStartEvents: []string{compiler.EventNotify}},
			},
			wantExclude: []string{"cc", "chat", "auto", "proxy"},
			tasks: []model.Task{
				{TaskID: 41, NodeID: "leader", NodeName: "主管审批", UserID: "alice", Status: 1, Comment: "同意", FinishedTime: localTime("2026-11-11 10:00:00")},
				{TaskID: 40, NodeID: "start", NodeName: "提单", UserID: "bob", Status: 1, Comment: "补充了预算明细", FinishedTime: localTime("2026-11-10 09:00:00")},
			},
			total:        2,
			wantTotal:    2,
			wantComments: []string{"同意", "补充了预算明细"},
		},
		{
			name:   "未能识别的节点默认按人工保留_宁可漏排也不误删真实意见",
			instID: domain.NewID(100),
			limit:  5,
			nodes: []model.Node{
				{NodeID: "legacy", NodeName: "历史环节", NodeType: model.TaskNode},
			},
			wantExclude: []string{},
			tasks: []model.Task{
				{TaskID: 50, NodeID: "legacy", NodeName: "历史环节", UserID: "alice", Status: 1, Comment: "老工单意见", FinishedTime: localTime("2026-11-11 10:00:00")},
			},
			total:        1,
			wantTotal:    1,
			wantComments: []string{"老工单意见"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repoMocks.NewMockIEngineRepository(ctrl)
			if tc.instID.Int() > 0 {
				// 定义必须按实例锁定的版本回源：期望值写死 (7, 2)，若改用最新版即失配
				mockRepo.EXPECT().GetInstanceByID(gomock.Any(), tc.instID.Int()).
					Return(domain.Instance{ProcInstID: tc.instID.Int(), ProcID: 7, ProcVersion: 2}, nil)
				nodes := tc.nodes
				if nodes == nil {
					nodes = []model.Node{{
						NodeID: "node", NodeName: "环节", NodeType: model.TaskNode,
						UserIDs: []string{"alice"}, NodeStartEvents: []string{compiler.EventNotify},
					}}
				}
				mockRepo.EXPECT().GetProcessDefineByVersion(gomock.Any(), 7, 2).
					Return(model.Process{ProcessName: "请假流程", Nodes: nodes}, nil)

				wantExclude := tc.wantExclude
				if wantExclude == nil {
					wantExclude = []string{}
				}
				// limit 与机器节点排除集都必须原样下推：断言"不再按 1000 全量扫描"且人机过滤在 SQL 层完成
				mockRepo.EXPECT().ListAccessiblePriorComments(gomock.Any(), tc.instID.Int(), tc.limit, wantExclude).
					Return(tc.tasks, tc.listErr)
				mockRepo.EXPECT().CountAccessiblePriorComments(gomock.Any(), tc.instID.Int(), wantExclude).
					Return(tc.total, tc.countErr)
			}

			comments, total, err := NewEngine(mockRepo).GetPriorComments(context.Background(), tc.instID, tc.limit)
			if tc.wantErr {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				} else {
					assert.Contains(t, err.Error(), "easyflow 查询前序审批意见失败")
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantTotal, total)
			assert.Equal(t, tc.wantComments, lo.Map(comments, func(c domain.PriorComment, _ int) string { return c.Comment }))
			if tc.wantTimes != nil {
				assert.Equal(t, tc.wantTimes, lo.Map(comments, func(c domain.PriorComment, _ int) string { return c.OccurredAt }))
			}
			if tc.wantFirst != nil {
				require.NotEmpty(t, comments)
				assert.Equal(t, *tc.wantFirst, comments[0])
			}
		})
	}
}

// TestStarterRejectTarget 驳回目标必须取自流程定义中真实的起始节点：
// 起始节点 ID 由画布 DSL 生成，假定字面量 "Start" 会让自由驳回指向不存在的节点
func TestStarterRejectTarget(t *testing.T) {
	starterTarget := func(nodeID string) domain.RejectTarget {
		return domain.RejectTarget{NodeID: nodeID, NodeName: "发起人 (重新填报)", NodeType: "START", IsStarter: true}
	}

	testCases := []struct {
		name       string
		nodes      easyEngine.ProcNodes
		wantTarget domain.RejectTarget
		wantErr    string
	}{
		{
			name: "画布生成的起始节点ID",
			nodes: easyEngine.ProcNodes{
				"start_9f3k": {NodeID: "start_9f3k", NodeName: "提单", NodeType: model.RootNode},
				"approve_1":  {NodeID: "approve_1", NodeName: "主管审批", NodeType: model.TaskNode},
				"gw_1":       {NodeID: "gw_1", NodeName: "条件分支", NodeType: model.GateWayNode},
				"end_1":      {NodeID: "end_1", NodeName: "结束", NodeType: model.EndNode},
			},
			wantTarget: starterTarget("start_9f3k"),
		},
		{
			name: "历史定义以字面量Start为起始节点仍兼容",
			nodes: easyEngine.ProcNodes{
				"Start": {NodeID: "Start", NodeName: "提单", NodeType: model.RootNode},
			},
			wantTarget: starterTarget("Start"),
		},
		{
			name: "缺少起始节点时报错",
			nodes: easyEngine.ProcNodes{
				"approve_1": {NodeID: "approve_1", NodeName: "主管审批", NodeType: model.TaskNode},
			},
			wantErr: "流程定义中不存在起始节点",
		},
		{
			name:    "空节点集报错",
			nodes:   easyEngine.ProcNodes{},
			wantErr: "流程定义中不存在起始节点",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			target, err := starterRejectTarget(tc.nodes)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.wantTarget, target)
		})
	}
}

// TestAdapter_GetRejectTargets 端到端验证退回发起人目标的解析链路：
// 引擎任务明细 → 流程实例 → 流程节点缓存 → 真实起始节点。
// engine.DB 与 ProcCache 是包级全局变量，测试前后保存恢复，避免污染其他用例
func TestAdapter_GetRejectTargets(t *testing.T) {
	oldDB, oldCache := easyEngine.DB, easyEngine.ProcCache
	t.Cleanup(func() { easyEngine.DB, easyEngine.ProcCache = oldDB, oldCache })

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	// :memory: 库随连接私有，多连接会各自看到空库
	sqlDB.SetMaxOpenConns(1)

	// 时间列声明 DATETIME 且铺数据时置 NULL：LocalTime.Scan 只接受 time.Time，
	// 本用例只关心任务归属与节点拓扑，不触达时间字段
	taskColumns := `proc_id INTEGER, proc_inst_id INTEGER, business_id TEXT, starter TEXT,
		node_id TEXT, node_name TEXT, prev_node_id TEXT, is_cosigned INTEGER, batch_code TEXT,
		user_id TEXT, status INTEGER, is_finished INTEGER, comment TEXT,
		proc_inst_create_time DATETIME, create_time DATETIME, finished_time DATETIME`
	for _, ddl := range []string{
		"CREATE TABLE proc_task (id INTEGER PRIMARY KEY," + taskColumns + ")",
		"CREATE TABLE hist_proc_task (id INTEGER PRIMARY KEY, task_id INTEGER," + taskColumns + ")",
		"CREATE TABLE proc_def (id INTEGER PRIMARY KEY, name TEXT)",
	} {
		require.NoError(t, db.Exec(ddl).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO proc_def VALUES (7, '请假流程')").Error)
	// 在办任务与归档任务各覆盖 GetTaskInfo CTE 的一条分支
	require.NoError(t, db.Exec(`INSERT INTO proc_task (id, proc_id, proc_inst_id, node_id, node_name, user_id, status, is_finished)
		VALUES (5, 7, 100, 'approve_1', '主管审批', 'bob', 0, 0)`).Error)
	require.NoError(t, db.Exec(`INSERT INTO hist_proc_task (id, task_id, proc_id, proc_inst_id, node_id, node_name, user_id, status, is_finished)
		VALUES (9, 9, 7, 100, 'approve_1', '主管审批', 'bob', 1, 1)`).Error)

	easyEngine.DB = db
	easyEngine.ProcCache = map[int]easyEngine.ProcNodes{
		7: {
			"start_9f3k": {NodeID: "start_9f3k", NodeName: "提单", NodeType: model.RootNode},
			"approve_1":  {NodeID: "approve_1", NodeName: "主管审批", NodeType: model.TaskNode},
		},
		// 缺失起始节点的异常定义，覆盖解析失败分支
		8: {
			"approve_1": {NodeID: "approve_1", NodeName: "主管审批", NodeType: model.TaskNode},
		},
	}

	canvasTarget := domain.RejectTarget{NodeID: "start_9f3k", NodeName: "发起人 (重新填报)", NodeType: "START", IsStarter: true}

	testCases := []struct {
		name       string
		taskID     domain.ID
		setupMock  func(m *repoMocks.MockIEngineRepository)
		want       []domain.RejectTarget
		wantErrIs  error
		wantErrStr string
	}{
		{
			name:   "在办任务返回画布生成的起始节点ID",
			taskID: domain.NewID(5),
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 100).
					Return(domain.Instance{ProcInstID: 100, ProcID: 7, ProcVersion: 2}, nil)
			},
			want: []domain.RejectTarget{canvasTarget},
		},
		{
			name:   "归档任务经CTE同样解析出真实起始节点",
			taskID: domain.NewID(9),
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 100).
					Return(domain.Instance{ProcInstID: 100, ProcID: 7, ProcVersion: 2}, nil)
			},
			want: []domain.RejectTarget{canvasTarget},
		},
		{
			name:      "非法任务ID直接拒绝_不触达引擎与仓储",
			taskID:    domain.NewID(0),
			setupMock: func(m *repoMocks.MockIEngineRepository) {},
			wantErrIs: engine.ErrInvalidParameter,
		},
		{
			name:       "任务不存在时携带根因返回",
			taskID:     domain.NewID(404),
			setupMock:  func(m *repoMocks.MockIEngineRepository) {},
			wantErrStr: "查询 easyflow 任务明细失败",
		},
		{
			name:   "实例查询失败时携带根因返回",
			taskID: domain.NewID(5),
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 100).Return(domain.Instance{}, errors.New("db down"))
			},
			wantErrStr: "查询 easyflow 流程实例失败",
		},
		{
			name:   "流程定义缺失起始节点时报错",
			taskID: domain.NewID(5),
			setupMock: func(m *repoMocks.MockIEngineRepository) {
				m.EXPECT().GetInstanceByID(gomock.Any(), 100).
					Return(domain.Instance{ProcInstID: 100, ProcID: 8, ProcVersion: 1}, nil)
			},
			wantErrStr: "解析 easyflow 驳回起点失败",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := repoMocks.NewMockIEngineRepository(ctrl)
			tc.setupMock(mockRepo)

			targets, err := NewEngine(mockRepo).GetRejectTargets(context.Background(), tc.taskID)
			if tc.wantErrIs != nil || tc.wantErrStr != "" {
				require.Error(t, err)
				if tc.wantErrIs != nil {
					assert.ErrorIs(t, err, tc.wantErrIs)
				}
				if tc.wantErrStr != "" {
					assert.Contains(t, err.Error(), tc.wantErrStr)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, targets)
		})
	}
}
