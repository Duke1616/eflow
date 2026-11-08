package easyflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Bunny3th/easy-workflow/workflow/database"
	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
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
func NewEngine(repo repository.IEngineRepository) engine.IEngine {
	return &adapter{
		repo: repo,
	}
}

func (a *adapter) EngineType() domain.EngineType {
	return domain.EngineTypeEasyFlow
}

func (a *adapter) Deploy(ctx context.Context, _ domain.EngineType, rawDef any) (domain.DeployInfo, error) {
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

func (a *adapter) GetTaskContext(ctx context.Context, _ domain.EngineType, taskID domain.ID) (domain.TaskContext, error) {
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

func (a *adapter) GetTimeline(ctx context.Context, _ domain.EngineType, instanceID domain.ID) ([]domain.TimelineNode, error) {
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

func (a *adapter) GetInstanceExecutionView(ctx context.Context, _ domain.EngineType, instanceID domain.ID, status uint8) (domain.InstanceExecutionView, error) {
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

	edgeMap, err := calculateTraversedEdges(ctx, a.repo, tasks, procInstID, inst.ProcID, inst.ProcVersion, status)
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

func (a *adapter) GetAutomationTask(ctx context.Context, _ domain.EngineType, instanceID domain.ID, nodeID string) (domain.TaskRecord, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return domain.TaskRecord{}, fmt.Errorf("easyflow 流程实例 ID 非法: %s", instanceID)
	}

	task, err := a.repo.GetAutomationTask(ctx, nodeID, procInstID)
	if err != nil {
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

func (a *adapter) GetTasksByCurrentNode(ctx context.Context, _ domain.EngineType, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error) {
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

func (a *adapter) GetTaskRecords(ctx context.Context, _ domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error) {
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

func (a *adapter) GetTaskTimeline(ctx context.Context, _ domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error) {
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
