package easyflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/database"
	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	compiler "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"
)

// adapter easyflow (easy-workflow) 历史遗留工作流引擎适配器实现
type adapter struct {
	repo repository.IEngineRepository
}

const (
	// maxScanLimit 时间线聚合与审批历史扫描的安全上限
	maxScanLimit = 1000
)

// NewEngine 实例化面向 easyflow 的引擎实现
func NewEngine(repo repository.IEngineRepository) engine.IDriver {
	return &adapter{
		repo: repo,
	}
}

func (a *adapter) EngineType() domain.EngineType {
	return domain.EngineTypeEasyFlow
}

func (a *adapter) Deploy(ctx context.Context, rawDef any) (domain.DeployInfo, error) {
	process, ok := rawDef.(*model.Process)
	if !ok {
		return domain.DeployInfo{}, errors.New("easyflow 流程定义编译产物类型错误")
	}

	bs, err := json.Marshal(process)
	if err != nil {
		return domain.DeployInfo{}, fmt.Errorf("序列化 easyflow 流程定义失败: %w", err)
	}

	processID, err := easyEngine.ProcessSave(string(bs), process.ProcessName)
	if err != nil {
		return domain.DeployInfo{}, fmt.Errorf("保存 easyflow 流程定义失败: %w", err)
	}

	version, err := a.repo.GetLatestProcessVersion(ctx, processID)
	if err != nil {
		return domain.DeployInfo{}, fmt.Errorf("获取最新 easyflow 流程版本失败: %w", err)
	}

	return domain.DeployInfo{
		ProcessID: processID,
		Version:   version,
	}, nil
}

func (a *adapter) Start(ctx context.Context, req domain.ProcessLaunchReq) (domain.ProcessLaunchResult, error) {
	if req.ProcessId <= 0 {
		return domain.ProcessLaunchResult{}, errors.New("easyflow 流程定义 ProcessId 不能为空")
	}

	rawVars := toEasyVariables(ensureEssentialVariables(req.Variables, req.Starter, req.TicketId))
	instID, err := easyEngine.InstanceStart(req.ProcessId, "业务申请", req.WorkflowName, rawVars)
	if err != nil {
		return domain.ProcessLaunchResult{}, fmt.Errorf("easyflow 启动流程实例失败: %w", err)
	}

	return domain.ProcessLaunchResult{
		EngineType: domain.EngineTypeEasyFlow,
		InstanceID: domain.NewID(instID),
	}, nil
}

func (a *adapter) Pass(ctx context.Context, req domain.TaskPassReq) error {
	taskID := req.TaskId.Int()
	if taskID <= 0 {
		return fmt.Errorf("%w: easyflow 任务 ID 无效: %s", engine.ErrInvalidParameter, req.TaskId)
	}

	if err := easyEngine.TaskPass(taskID, req.Comment, toEasyVariables(req.Variables), false); err != nil {
		return fmt.Errorf("easyflow 推进任务通过失败 [taskId=%d]: %w", taskID, err)
	}
	return nil
}

func (a *adapter) Reject(ctx context.Context, req domain.TaskRejectReq) error {
	taskID := req.TaskId.Int()
	if taskID <= 0 {
		return fmt.Errorf("%w: easyflow 任务 ID 无效: %s", engine.ErrInvalidParameter, req.TaskId)
	}

	if req.TargetNodeId != "" {
		if err := easyEngine.TaskFreeRejectToUpstreamNode(taskID, req.TargetNodeId, req.Comment, ""); err != nil {
			return fmt.Errorf("easyflow 驳回任务至目标节点失败 [taskId=%d, targetNode=%s]: %w", taskID, req.TargetNodeId, err)
		}
		return nil
	}
	if err := easyEngine.TaskReject(taskID, req.Comment, ""); err != nil {
		return fmt.Errorf("easyflow 驳回任务失败 [taskId=%d]: %w", taskID, err)
	}
	return nil
}

func (a *adapter) GetRejectTargets(ctx context.Context, taskID domain.ID) ([]domain.RejectTarget, error) {
	// easyflow 遗留引擎兼容实现：提供流程起点作为通用驳回目标
	return []domain.RejectTarget{
		{
			NodeID:    "Start",
			NodeName:  "发起人 (重新填报)",
			NodeType:  "START",
			IsStarter: true,
		},
	}, nil
}

// toEasyVariables 纯函数：将变量字典单向序列化为 easy-workflow 所需的 []model.Variable 格式
func toEasyVariables(variables domain.Variables) string {
	if len(variables) == 0 {
		return "[]"
	}
	vars := lo.MapToSlice(variables, func(k string, v any) model.Variable {
		return model.Variable{Key: k, Value: fmt.Sprint(v)}
	})
	bs, err := json.Marshal(vars)
	if err != nil {
		return "[]"
	}
	return string(bs)
}

// ensureEssentialVariables 保证 easyflow 所需的核心元变量（$starter 与 ticket_id）始终注入
func ensureEssentialVariables(vars domain.Variables, starter string, ticketID int64) domain.Variables {
	result := vars.Clone()
	if result == nil {
		result = domain.NewVariables()
	}
	if starter != "" && !domain.VarStarter.Has(result) {
		domain.VarStarter.Set(&result, starter)
	}
	if ticketID > 0 && !domain.VarTicketID.Has(result) {
		domain.VarTicketID.Set(&result, ticketID)
	}
	return result
}

func (a *adapter) GetTaskContext(ctx context.Context, taskID domain.ID) (domain.TaskContext, error) {
	id := taskID.Int()
	if id <= 0 {
		return domain.TaskContext{}, fmt.Errorf("easyflow 任务 ID 非法: %s", taskID)
	}

	task, err := easyEngine.GetTaskInfo(id)
	if err != nil {
		return domain.TaskContext{}, fmt.Errorf("查询 easyflow 任务明细失败: %w", err)
	}

	inst, err := a.repo.GetInstanceByID(ctx, task.ProcInstID)
	if err != nil {
		return domain.TaskContext{}, fmt.Errorf("查询 easyflow 流程实例详情失败 [instanceID=%d]: %w", task.ProcInstID, err)
	}

	return domain.TaskContext{
		EngineType:        domain.EngineTypeEasyFlow,
		TaskID:            taskID,
		ProcessInstanceID: domain.NewID(task.ProcInstID),
		ProcessID:         task.ProcID,
		ProcessVersion:    inst.ProcVersion,
		NodeID:            task.NodeID,
		NodeName:          task.NodeName,
		Assignee:          task.UserID,
		IsFinished:        task.IsFinished == 1,
		Comment:           task.Comment,
	}, nil
}

func (a *adapter) GetTimeline(ctx context.Context, instanceID domain.ID) ([]domain.TimelineNode, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, fmt.Errorf("%w: easyflow 流程实例 ID 无效: %s", engine.ErrInvalidParameter, instanceID)
	}

	groups, err := a.repo.ListAccessibleTaskTimelineGroups(ctx, procInstID, 0, maxScanLimit)
	if err != nil {
		return nil, fmt.Errorf("easyflow 查询时间线聚合批次失败: %w", err)
	}

	if len(groups) == 0 {
		return nil, nil
	}

	members, err := a.repo.ListAccessibleTaskTimelineMembers(ctx, procInstID, groups)
	if err != nil {
		return nil, fmt.Errorf("easyflow 查询时间线成员明细失败: %w", err)
	}

	memberMap := lo.GroupBy(members, func(t model.Task) string {
		return fmt.Sprintf("%s:%s", t.NodeID, t.BatchCode)
	})

	nodes := make([]domain.TimelineNode, 0, len(groups))
	for _, group := range groups {
		key := fmt.Sprintf("%s:%s", group.NodeID, group.BatchCode)
		taskMembers := memberMap[key]
		if len(taskMembers) == 0 {
			nodes = append(nodes, domain.TimelineNode{
				NodeID:    group.NodeID,
				NodeName:  group.NodeName,
				Status:    "PENDING",
				StartTime: group.OccurredAt,
			})
			continue
		}

		for _, m := range taskMembers {
			nodes = append(nodes, domain.TimelineNode{
				NodeID:    m.NodeID,
				NodeName:  m.NodeName,
				Operator:  m.UserID,
				Status:    strconv.Itoa(m.Status),
				Comment:   m.Comment,
				EndTime:   formatTimeString(m.FinishedTime),
				StartTime: group.OccurredAt,
			})
		}
	}

	return nodes, nil
}

func formatTimeString(t *database.LocalTime) string {
	if t == nil {
		return ""
	}
	return t.String()
}

func (a *adapter) GetInstanceExecutionView(ctx context.Context, instanceID domain.ID) (domain.InstanceExecutionView, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return domain.InstanceExecutionView{}, fmt.Errorf("%w: easyflow 流程实例 ID 无效: %s", engine.ErrInvalidParameter, instanceID)
	}

	inst, err := a.repo.GetInstanceByID(ctx, procInstID)
	if err != nil {
		return domain.InstanceExecutionView{}, fmt.Errorf("easyflow 获取实例详情失败: %w", err)
	}

	tasks, err := a.repo.ListTaskRecord(ctx, procInstID, 0, maxScanLimit)
	if err != nil {
		return domain.InstanceExecutionView{}, fmt.Errorf("easyflow 查询审批记录失败: %w", err)
	}

	nodeTasks := lo.GroupBy(tasks, func(task model.Task) string {
		return task.NodeID
	})
	nodeStatusMap := lo.MapValues(nodeTasks, func(group []model.Task, _ string) int {
		if lo.SomeBy(group, func(t model.Task) bool { return t.Status == 1 || t.Status == 3 }) {
			return 1
		}
		if lo.SomeBy(group, func(t model.Task) bool { return t.Status == 2 }) {
			return 2
		}
		if lo.SomeBy(group, func(t model.Task) bool { return t.Status == 0 && t.IsFinished == 0 }) {
			return 0
		}
		if lo.EveryBy(group, func(t model.Task) bool { return t.Status == 5 }) {
			return 5
		}
		return int(group[len(group)-1].Status)
	})

	pendingUsers := lo.Uniq(lo.FilterMap(tasks, func(src model.Task, _ int) (string, bool) {
		if src.Status == 0 && src.IsFinished == 0 && src.UserID != "" {
			return src.UserID, true
		}
		return "", false
	}))

	edgeMap, err := calculateTraversedEdges(ctx, a.repo, tasks, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return domain.InstanceExecutionView{}, fmt.Errorf("easyflow 计算已流转边失败: %w", err)
	}

	return domain.InstanceExecutionView{
		EngineType:     domain.EngineTypeEasyFlow,
		ProcessID:      inst.ProcID,
		ProcessVersion: inst.ProcVersion,
		NodeStatusMap:  nodeStatusMap,
		TraversedEdges: edgeMap,
		PendingUsers:   pendingUsers,
	}, nil
}

func (a *adapter) Revoke(ctx context.Context, req domain.RevokeReq) error {
	procInstID := req.InstanceID.Int()
	if procInstID <= 0 {
		return fmt.Errorf("%w: easyflow 流程实例 ID 非法: %s", engine.ErrInvalidParameter, req.InstanceID)
	}

	// NOTE: 容错防卫。当工单此前在启动阶段失败时，easyflow 底层事务已回滚，proc_inst 表并无此实例记录。
	// 若底层流程实例不存在或未就绪，无需也不可能在底层引擎执行归档流转，直接幂等返回成功，
	// 允许上层撤回服务继续闭环将工单终结为已撤回状态，避免用户卡死在 500 无法撤回。
	inst, err := a.repo.GetInstanceByID(ctx, procInstID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
	} else if inst.ProcInstID == 0 && inst.ProcID == 0 {
		return nil
	}

	err = easyEngine.InstanceRevoke(procInstID, req.Force, req.Operator)
	if err != nil {
		// 历史/异常数据兜底：若底层引擎返回“未找到对应流程定义”，说明实例不存在或流程定义丢失
		if strings.Contains(err.Error(), "未找到对应流程定义") {
			return nil
		}
		return err
	}
	return nil
}

func (a *adapter) Transfer(ctx context.Context, req domain.TaskTransferReq) error {
	procTaskID := req.TaskId.Int()
	if procTaskID <= 0 {
		return fmt.Errorf("%w: easyflow 任务 ID 非法: %s", engine.ErrInvalidParameter, req.TaskId)
	}
	_, err := a.repo.Transfer(ctx, procTaskID, req.TargetUsers)
	return err
}

func (a *adapter) GetAutomationTask(ctx context.Context, instanceID domain.ID, nodeID string) (domain.TaskRecord, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return domain.TaskRecord{}, fmt.Errorf("easyflow 流程实例 ID 非法: %s", instanceID)
	}

	task, err := a.repo.GetAutomationTask(ctx, nodeID, procInstID)
	if err != nil {
		// 统一收敛为引擎层哨兵错误，屏蔽底层 ORM 差异，供上层基于 errors.Is 精准识别"节点任务已不存在"
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return domain.TaskRecord{}, fmt.Errorf("%w: easyflow 自动化节点任务不存在 [node_id=%s]", engine.ErrTaskNotFound, nodeID)
		}
		return domain.TaskRecord{}, err
	}

	return domain.TaskRecord{
		TaskID:       domain.NewID(task.TaskID),
		NodeID:       task.NodeID,
		PrevNodeID:   task.PrevNodeID,
		BatchCode:    task.BatchCode,
		NodeName:     task.NodeName,
		ApprovedBy:   task.UserID,
		IsCosigned:   task.IsCosigned,
		Status:       task.Status,
		Comment:      task.Comment,
		IsFinished:   task.IsFinished,
		FinishedTime: task.FinishedTime,
	}, nil
}

func (a *adapter) GetTasksByCurrentNode(ctx context.Context, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, fmt.Errorf("easyflow 流程实例 ID 非法: %s", instanceID)
	}

	tasks, err := a.repo.GetTasksByCurrentNodeId(ctx, procInstID, nodeID)
	if err != nil {
		return nil, err
	}

	return lo.Map(tasks, func(t model.Task, _ int) domain.TaskRecord {
		return domain.TaskRecord{
			TaskID:       domain.NewID(t.TaskID),
			NodeID:       t.NodeID,
			PrevNodeID:   t.PrevNodeID,
			BatchCode:    t.BatchCode,
			NodeName:     t.NodeName,
			ApprovedBy:   t.UserID,
			IsCosigned:   t.IsCosigned,
			Status:       t.Status,
			Comment:      t.Comment,
			IsFinished:   t.IsFinished,
			FinishedTime: t.FinishedTime,
		}
	}), nil
}

func (a *adapter) ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error) {
	var (
		eg    errgroup.Group
		insts []domain.Instance
		total int64
	)

	eg.Go(func() error {
		var err error
		if req.IsAdmin {
			insts, err = a.repo.TodoAllList(ctx, req.Username, req.ProcessName, req.SortByAsc, req.Offset, req.Limit)
		} else {
			insts, err = a.repo.TodoList(ctx, req.Username, req.ProcessName, req.SortByAsc, req.Offset, req.Limit)
		}
		return err
	})

	eg.Go(func() error {
		var err error
		if req.IsAdmin {
			total, err = a.repo.CountAllTodo(ctx, req.Username, req.ProcessName)
		} else {
			total, err = a.repo.CountTodo(ctx, req.Username, req.ProcessName)
		}
		return err
	})

	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	tasks := lo.Map(insts, func(inst domain.Instance, _ int) domain.TodoTask {
		var createTime time.Time
		if inst.CreateTime != nil {
			createTime = *inst.CreateTime
		}
		return domain.TodoTask{
			EngineType: domain.EngineTypeEasyFlow,
			TaskID:     domain.NewID(inst.TaskID),
			InstanceID: domain.NewID(inst.ProcInstID),
			NodeID:     inst.CurrentNodeID,
			NodeName:   inst.CurrentNodeName,
			ApprovedBy: inst.ApprovedBy,
			CreateTime: createTime,
		}
	})

	return tasks, total, nil
}

func (a *adapter) ListPendingSteps(ctx context.Context, processes []domain.Process) ([]domain.TodoTask, error) {
	procInstIDs := lo.FilterMap(processes, func(p domain.Process, _ int) (int, bool) {
		val := p.InstanceID.Int()
		return val, val > 0
	})
	if len(procInstIDs) == 0 {
		return nil, nil
	}

	insts, err := a.repo.ListTasksByProcInstIds(ctx, procInstIDs, "")
	if err != nil {
		return nil, err
	}

	return lo.Map(insts, func(inst domain.Instance, _ int) domain.TodoTask {
		var createTime time.Time
		if inst.CreateTime != nil {
			createTime = *inst.CreateTime
		}
		return domain.TodoTask{
			EngineType: domain.EngineTypeEasyFlow,
			TaskID:     domain.NewID(inst.TaskID),
			InstanceID: domain.NewID(inst.ProcInstID),
			NodeID:     inst.CurrentNodeID,
			NodeName:   inst.CurrentNodeName,
			ApprovedBy: inst.ApprovedBy,
			CreateTime: createTime,
		}
	}), nil
}

func (a *adapter) GetTaskRecords(ctx context.Context, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, 0, fmt.Errorf("%w: easyflow 流程实例 ID 非法: %s", engine.ErrInvalidParameter, instanceID)
	}

	var (
		records []model.Task
		total   int64
		eg      errgroup.Group
	)
	eg.Go(func() error {
		var err error
		records, err = a.repo.ListAccessibleTaskRecord(ctx, procInstID, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = a.repo.CountAccessibleTaskRecord(ctx, procInstID)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	domainRecords := lo.Map(records, func(t model.Task, _ int) domain.TaskRecord {
		return domain.TaskRecord{
			TaskID:       domain.NewID(t.TaskID),
			NodeID:       t.NodeID,
			PrevNodeID:   t.PrevNodeID,
			BatchCode:    t.BatchCode,
			NodeName:     t.NodeName,
			ApprovedBy:   t.UserID,
			IsCosigned:   t.IsCosigned,
			Status:       t.Status,
			Comment:      t.Comment,
			IsFinished:   t.IsFinished,
			FinishedTime: t.FinishedTime,
		}
	})

	return domainRecords, total, nil
}

func (a *adapter) GetTaskTimeline(ctx context.Context, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, 0, fmt.Errorf("%w: easyflow 流程实例 ID 非法: %s", engine.ErrInvalidParameter, instanceID)
	}

	var (
		eg     errgroup.Group
		groups []domain.TaskTimelineGroup
		total  int64
	)
	eg.Go(func() error {
		var err error
		groups, err = a.repo.ListAccessibleTaskTimelineGroups(ctx, procInstID, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = a.repo.CountAccessibleTaskTimelineGroups(ctx, procInstID)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	if len(groups) == 0 {
		return []domain.TaskTimelineGroup{}, total, nil
	}

	members, err := a.repo.ListAccessibleTaskTimelineMembers(ctx, procInstID, groups)
	if err != nil {
		return nil, 0, err
	}

	byKey := lo.GroupBy(members, func(member model.Task) string {
		return fmt.Sprintf("%s:%s", member.NodeID, member.BatchCode)
	})

	timelineGroups := lo.Map(groups, func(g domain.TaskTimelineGroup, _ int) domain.TaskTimelineGroup {
		key := fmt.Sprintf("%s:%s", g.NodeID, g.BatchCode)
		g.Members = lo.Map(byKey[key], func(t model.Task, _ int) domain.TaskRecord {
			return domain.TaskRecord{
				TaskID:       domain.NewID(t.TaskID),
				NodeID:       t.NodeID,
				PrevNodeID:   t.PrevNodeID,
				BatchCode:    t.BatchCode,
				NodeName:     t.NodeName,
				ApprovedBy:   t.UserID,
				IsCosigned:   t.IsCosigned,
				Status:       t.Status,
				Comment:      t.Comment,
				IsFinished:   t.IsFinished,
				FinishedTime: t.FinishedTime,
			}
		})
		return g
	})

	return timelineGroups, total, nil
}

// GetActiveReject 获取流程实例当前正处于待处理/待响应状态的最新驳回上下文（若无驳回或已重新推进闭环，返回 nil）
func (a *adapter) GetActiveReject(ctx context.Context, instanceID domain.ID) (*domain.RejectSnapshot, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, nil
	}

	ts, _, err := a.GetTaskRecords(ctx, instanceID, 0, 1000)
	if err != nil {
		return nil, err
	}

	rejectTasks := lo.Filter(ts, func(t domain.TaskRecord, _ int) bool {
		return t.IsRejected()
	})
	if len(rejectTasks) == 0 {
		return nil, nil
	}

	// 按办结时间倒序锁定最近一次驳回：easyflow 的 BatchCode 是引擎用 SELECT UUID() 生成的随机串，
	// 字典序与轮次先后无关，拿它当主排序键会取到旧一轮的驳回意见
	sort.SliceStable(rejectTasks, func(i, j int) bool {
		ti, tj := fmt.Sprintf("%v", rejectTasks[i].FinishedTime), fmt.Sprintf("%v", rejectTasks[j].FinishedTime)
		if ti != tj {
			return ti > tj
		}
		return rejectTasks[i].TaskID.Int() > rejectTasks[j].TaskID.Int()
	})
	latest := rejectTasks[0]

	// 闭环判定：流程是否已重新走回驳回节点（该节点出现更晚创建的任务）
	// 不按"任一任务在驳回后通过"判定：自由驳回会同时打开多个上游重审节点，先提交的那一个
	// 会把其他尚未响应者的驳回意见一并抹掉，而这条意见可能正是写给后者的
	// 先后比较用自增 TaskID 而非 BatchCode，因为 UUID 批次码不具顺序语义
	hasClosedLoop := lo.SomeBy(ts, func(t domain.TaskRecord) bool {
		return t.NodeID == latest.NodeID && t.TaskID.Int() > latest.TaskID.Int()
	})
	if hasClosedLoop {
		return nil, nil
	}

	hasPendingTasks := lo.SomeBy(ts, func(t domain.TaskRecord) bool {
		return t.Status == 0 && t.IsFinished == 0
	})
	if !hasPendingTasks {
		return nil, nil
	}

	return &domain.RejectSnapshot{
		Operator:   latest.ApprovedBy,
		NodeID:     latest.NodeID,
		NodeName:   latest.NodeName,
		Comment:    latest.Comment,
		FinishTime: fmt.Sprintf("%v", latest.FinishedTime),
	}, nil
}

// GetPriorComments 过滤、排序与截断全部下推给 SQL，只回传前端要展开的最近 limit 条意见，
// 避免为了几条意见把整个实例的任务链拉进内存
func (a *adapter) GetPriorComments(ctx context.Context, instanceID domain.ID, limit int) ([]domain.PriorComment, int64, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, 0, fmt.Errorf("%w: easyflow 流程实例 ID 非法: %s", engine.ErrInvalidParameter, instanceID)
	}

	// 机器节点须先解析出来再下推，否则 LIMIT 与 COUNT 会被固定文案污染
	excludeNodeIDs, err := a.machineNodeIDs(ctx, procInstID)
	if err != nil {
		return nil, 0, err
	}

	var (
		tasks []model.Task
		total int64
		eg    errgroup.Group
	)
	eg.Go(func() error {
		var err error
		tasks, err = a.repo.ListAccessiblePriorComments(ctx, procInstID, limit, excludeNodeIDs)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = a.repo.CountAccessiblePriorComments(ctx, procInstID, excludeNodeIDs)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, fmt.Errorf("easyflow 查询前序审批意见失败: %w", err)
	}

	return lo.Map(tasks, func(t model.Task, _ int) domain.PriorComment {
		return domain.PriorComment{
			NodeID:     t.NodeID,
			NodeName:   t.NodeName,
			ApprovedBy: t.UserID,
			Status:     t.Status,
			Comment:    t.Comment,
			OccurredAt: t.FinishedTime.String(),
		}
	}), total, nil
}

// machineNodeIDs 识别机器节点，交由 SQL 层排除。
// 按实例锁定的版本回源定义：改版后的拓扑不能用来解释历史工单。
// 取「排除机器」而非「只留人工」：未识别的节点默认保留，宁可漏排一条系统文案，也不误删真实审批意见
func (a *adapter) machineNodeIDs(ctx context.Context, procInstID int) ([]string, error) {
	inst, err := a.repo.GetInstanceByID(ctx, procInstID)
	if err != nil {
		return nil, fmt.Errorf("easyflow 查询流程实例失败 [procInstID=%d]: %w", procInstID, err)
	}

	def, err := a.repo.GetProcessDefineByVersion(ctx, inst.ProcID, inst.ProcVersion)
	if err != nil {
		return nil, fmt.Errorf("easyflow 查询流程定义失败 [procID=%d version=%d]: %w", inst.ProcID, inst.ProcVersion, err)
	}

	return lo.FilterMap(def.Nodes, func(n model.Node, _ int) (string, bool) {
		return n.NodeID, isMachineNode(n)
	}), nil
}

// isMachineNode 机器裁决节点的意见是执行器写死的固定文案，不是人工审批意见。
// easyflow 把抄送/群聊/自动化都编译成 TaskNode，节点类型区分不了，只能靠 NodeStartEvents 标记；
// 标记随定义 JSON 落在 proc_def.resource（历史版本走 hist_proc_def），因此判定与工单锁定的拓扑一致。
// 开始节点不在此列：驳回回退到起点时它退化成普通任务节点，由提单人填写真实意见
func isMachineNode(n model.Node) bool {
	for _, ev := range n.NodeStartEvents {
		switch ev {
		case compiler.EventCarbonCopy, compiler.EventChatGroup, compiler.EventAutomation:
			return true
		}
	}
	// 系统代理节点挂的是普通 EventNotify，事件标记区分不了，只能靠占位处理人兜住
	return len(n.UserIDs) > 0 && n.UserIDs[0] == compiler.SysAutoUser
}
