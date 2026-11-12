package flowx

import (
	"context"
	"fmt"
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/samber/lo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// autoCycleDefinition 构造两个同类自动节点互相成环的定义。
// 静态校验刻意只拦自环（防环由 kernel 遍历原语自身保证），故此类定义可以正常部署，
// 但瞬时穿透递归没有天然停止点，必须由调度层的深度护栏收敛为业务错误
func autoCycleDefinition(key string, nodeType model.NodeType) model.ProcessDefinition {
	b := NewProcess(key, "自动节点成环").Start("s", "发起")
	switch nodeType {
	case model.NodeCarbonCopy:
		b = b.CarbonCopy("c1", "自动一", "alice").CarbonCopy("c2", "自动二", "bob")
	case model.NodeChatGroup:
		b = b.ChatGroup("c1", "自动一").ChatGroup("c2", "自动二")
	case model.NodeTask:
		b = b.Task("c1", "自动一", "noop").Task("c2", "自动二", "noop")
	}
	return b.End("e", "办结").
		Edge("s", "c1").
		Edge("c1", "c2").
		Edge("c2", "c1").
		Edge("c2", "e").
		Build()
}

func TestApplyStepDecision_AutoNodeCycleFailsFastNotCrash(t *testing.T) {
	testCases := []struct {
		name     string
		nodeType model.NodeType
	}{
		{name: "抄送节点成环", nodeType: model.NodeCarbonCopy},
		{name: "群通知节点成环", nodeType: model.NodeChatGroup},
		{name: "同步自动化任务成环", nodeType: model.NodeTask},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			engine := NewEngine(memory.NewStorage())
			engine.RegisterWorker("noop", func(context.Context, *TaskContext) (*TaskResult, error) {
				return &TaskResult{}, nil
			})

			def := autoCycleDefinition("cycle_"+string(tc.nodeType), tc.nodeType)
			require.NoError(t, def.Validate(), "此类定义可部署，护栏必须在运行期生效")

			// 修复前此处为 runtime fatal error: stack overflow，recover 兜不住，整个进程直接死
			_, err := engine.Start(ctx, StartReq{Def: def, Starter: "alice", Variables: model.Variables{}})
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrPenetrationTooDeep)
		})
	}
}

func TestApplyStepDecision_LongAutoChainWithinLimitStillFinishes(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(memory.NewStorage())

	// 合法长链不得被护栏误伤：30 个抄送节点串行穿透远低于上限
	b := NewProcess("long_chain", "长自动链").Start("s", "发起")
	const chainLen = 30
	for i := 0; i < chainLen; i++ {
		b = b.CarbonCopy(fmt.Sprintf("cc%02d", i), fmt.Sprintf("抄送%d", i), "alice")
	}
	b = b.End("e", "办结").Edge("s", "cc00")
	for i := 0; i < chainLen-1; i++ {
		b = b.Edge(fmt.Sprintf("cc%02d", i), fmt.Sprintf("cc%02d", i+1))
	}
	def := b.Edge(fmt.Sprintf("cc%02d", chainLen-1), "e").Build()

	instID, err := engine.Start(ctx, StartReq{Def: def, Starter: "bob", Variables: model.Variables{}})
	require.NoError(t, err)
	inst, err := engine.GetInstance(ctx, instID)
	require.NoError(t, err)
	assert.Equal(t, model.InstanceFinished, inst.Status)
}

func TestApplyStepDecision_ChainBeyondLimitFailsFast(t *testing.T) {
	ctx := context.Background()
	engine := NewEngine(memory.NewStorage())

	chainLen := maxPenetrationDepth + 5
	b := NewProcess("over_limit", "超限长链").Start("s", "发起")
	for i := 0; i < chainLen; i++ {
		b = b.CarbonCopy(fmt.Sprintf("cc%03d", i), fmt.Sprintf("抄送%d", i), "alice")
	}
	b = b.End("e", "办结").Edge("s", "cc000")
	for i := 0; i < chainLen-1; i++ {
		b = b.Edge(fmt.Sprintf("cc%03d", i), fmt.Sprintf("cc%03d", i+1))
	}
	def := b.Edge(fmt.Sprintf("cc%03d", chainLen-1), "e").Build()

	_, err := engine.Start(ctx, StartReq{Def: def, Starter: "bob", Variables: model.Variables{}})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPenetrationTooDeep)
}

// ccJoinDefinition 构造「人工审批 + 抄送」双分支汇聚流程，抄送处理人可为空
func ccJoinDefinition(key string, gatewayType model.NodeType, ccAssignees ...string) model.ProcessDefinition {
	b := NewProcess(key, "抄送汇聚").Start("s", "发起")
	switch gatewayType {
	case model.NodeGatewayParallel:
		b = b.ParallelGateway("split", "并行分流").ParallelGateway("join", "并行汇聚")
	case model.NodeGatewayInclusive:
		b = b.InclusiveGateway("split", "包容分流").InclusiveGateway("join", "包容汇聚")
	}
	return b.Approval("a1", "人工审批", model.SignOneVote, "alice").
		CarbonCopy("cc", "抄送知会", ccAssignees...).
		End("e", "办结").
		Edge("s", "split").
		Edge("split", "a1").
		Edge("split", "cc").
		Edge("a1", "join").
		Edge("cc", "join").
		Edge("join", "e").
		Build()
}

func TestCarbonCopy_EmptyAssigneeStillSatisfiesJoinBarrier(t *testing.T) {
	testCases := []struct {
		name        string
		gatewayType model.NodeType
		ccAssignees []string
	}{
		{name: "并行汇聚_抄送无处理人", gatewayType: model.NodeGatewayParallel, ccAssignees: nil},
		{name: "并行汇聚_抄送有处理人", gatewayType: model.NodeGatewayParallel, ccAssignees: []string{"carol"}},
		{name: "包容汇聚_抄送无处理人", gatewayType: model.NodeGatewayInclusive, ccAssignees: nil},
		{name: "包容汇聚_抄送有处理人", gatewayType: model.NodeGatewayInclusive, ccAssignees: []string{"carol"}},
		{name: "并行汇聚_抄送处理人全空白", gatewayType: model.NodeGatewayParallel, ccAssignees: []string{"", "   "}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			store := memory.NewStorage()
			engine := NewEngine(store)

			def := ccJoinDefinition("cc_join", tc.gatewayType, tc.ccAssignees...)
			instID, err := engine.Start(ctx, StartReq{Def: def, Starter: "bob", Variables: model.Variables{}})
			require.NoError(t, err)

			tasks, err := store.ListTasksByInstance(ctx, instID)
			require.NoError(t, err)
			aliceTask, ok := lo.Find(tasks, func(x *model.Task) bool { return x.Assignee == "alice" && x.IsPending() })
			require.True(t, ok, "人工审批分支必须已派发待办")
			require.NoError(t, engine.Pass(ctx, PassReq{TaskID: aliceTask.ID, Operator: "alice", Comment: "同意"}))

			inst, err := store.GetInstance(ctx, instID)
			require.NoError(t, err)
			// 修复前抄送无处理人时不落任何 Task，汇聚分支 HasTask 恒为 false，
			// 实例永久卡在 Running 且无任何在途待办、无任何报错
			assert.Equal(t, model.InstanceFinished, inst.Status, "抄送分支不得让汇聚屏障永久阻断")

			all, err := store.ListTasksByInstance(ctx, instID)
			require.NoError(t, err)
			assert.Empty(t, lo.Filter(all, func(x *model.Task, _ int) bool { return x.IsPending() }))
		})
	}
}

func TestCarbonCopy_EmptyAssigneePlaceholderIsHonest(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	engine := NewEngine(store)

	def := ccJoinDefinition("cc_placeholder", model.NodeGatewayParallel)
	instID, err := engine.Start(ctx, StartReq{Def: def, Starter: "bob", Variables: model.Variables{}})
	require.NoError(t, err)

	tasks, err := store.ListTasksByInstance(ctx, instID)
	require.NoError(t, err)
	ccTasks := lo.Filter(tasks, func(x *model.Task, _ int) bool { return x.NodeID == "cc" })
	require.Len(t, ccTasks, 1, "空处理人抄送必须落且只落一条占位记录")

	placeholder := ccTasks[0]
	assert.Equal(t, model.TaskPassed, placeholder.Status, "须与有处理人时的抄送完成态同口径：汇聚仲裁按 TaskPassed 计票，记为 Skipped 会再次死锁")
	assert.Equal(t, "system", placeholder.Assignee)
	assert.Contains(t, placeholder.Comment, "未解析到处理人")
	assert.False(t, placeholder.FinishTime.IsZero())
}

func TestCarbonCopy_AllBranchesEmptyStillFinishes(t *testing.T) {
	ctx := context.Background()
	store := memory.NewStorage()
	engine := NewEngine(store)

	// 全分支皆空抄送：若占位记为 Skipped，汇聚策略的「至少一条真实通过」防卫会再次死锁，
	// 故占位必须与正常抄送完成同为通过态
	def := NewProcess("cc_all_empty", "全空抄送").
		Start("s", "发起").
		ParallelGateway("split", "并行分流").
		CarbonCopy("cc1", "抄送一").
		CarbonCopy("cc2", "抄送二").
		ParallelGateway("join", "并行汇聚").
		End("e", "办结").
		Edge("s", "split").
		Edge("split", "cc1").
		Edge("split", "cc2").
		Edge("cc1", "join").
		Edge("cc2", "join").
		Edge("join", "e").
		Build()

	instID, err := engine.Start(ctx, StartReq{Def: def, Starter: "bob", Variables: model.Variables{}})
	require.NoError(t, err)
	inst, err := store.GetInstance(ctx, instID)
	require.NoError(t, err)
	assert.Equal(t, model.InstanceFinished, inst.Status)
}
