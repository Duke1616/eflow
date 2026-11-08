package flowx

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// flowxEngine flowx 现代工作流引擎实现
type flowxEngine struct {
	engine     flowx.Engine
	ticketRepo repository.TicketRepository
}

// NewEngine 实例化 flowx 引擎实现
func NewEngine(engine flowx.Engine, ticketRepo repository.TicketRepository) engine.IEngine {
	return &flowxEngine{
		engine:     engine,
		ticketRepo: ticketRepo,
	}
}

func (a *flowxEngine) EngineType() domain.EngineType {
	return domain.EngineTypeFlowx
}

func (a *flowxEngine) Deploy(ctx context.Context, _ domain.EngineType, rawDef any) (domain.DeployInfo, error) {
	def, ok := rawDef.(flowxModel.ProcessDefinition)
	if !ok {
		return domain.DeployInfo{}, errors.New("flowx 流程定义编译产物类型错误")
	}

	if a.engine == nil {
		return domain.DeployInfo{}, errors.New("flowx 流程引擎未初始化")
	}

	deployedDef, err := a.engine.Deploy(ctx, def)
	if err != nil {
		return domain.DeployInfo{}, fmt.Errorf("部署流程至 flowx 失败: %w", err)
	}

	return domain.DeployInfo{
		Version: deployedDef.Version,
	}, nil
}

func (a *flowxEngine) Start(ctx context.Context, req domain.ProcessLaunchReq) (domain.ProcessLaunchResult, error) {
	if a.engine == nil {
		return domain.ProcessLaunchResult{}, errors.New("flowx 流程引擎未初始化")
	}

	procKey := domain.FormatFlowxProcessKey(req.WorkflowId)
	instID := a.engine.GenInstanceID(procKey)

	// 预分配实例 ID 并提前物理落盘绑定，彻底消除生命周期事件广播与工单外键落盘之间的时序竞态
	if req.TicketId > 0 && a.ticketRepo != nil {
		if err := a.ticketRepo.RegisterProcess(ctx, req.TicketId, domain.EngineTypeFlowx, domain.NewID(instID)); err != nil {
			return domain.ProcessLaunchResult{}, fmt.Errorf("前置绑定工单流程引擎实例失败: %w", err)
		}
	}

	_, err := a.engine.Start(ctx, flowx.StartReq{
		InstanceID:  instID,
		Key:         procKey,
		Version:     req.Version,
		Starter:     req.Starter,
		Variables:   req.Variables,
		BusinessKey: strconv.FormatInt(req.TicketId, 10),
	})
	if err != nil {
		return domain.ProcessLaunchResult{}, fmt.Errorf("flowx 启动流程实例失败: %w", err)
	}

	return domain.ProcessLaunchResult{
		EngineType: domain.EngineTypeFlowx,
		InstanceID: domain.NewID(instID),
	}, nil
}

func (a *flowxEngine) Pass(ctx context.Context, req domain.TaskPassReq) error {
	if a.engine == nil {
		return errors.New("flowx 流程引擎未初始化")
	}

	return a.engine.Pass(ctx, flowx.PassReq{
		TaskID:    req.TaskId.String(),
		Operator:  req.Operator,
		Comment:   req.Comment,
		Variables: req.Variables,
	})
}

func (a *flowxEngine) Reject(ctx context.Context, req domain.TaskRejectReq) error {
	if a.engine == nil {
		return errors.New("flowx 流程引擎未初始化")
	}

	return a.engine.Reject(ctx, flowx.RejectReq{
		TaskID:       req.TaskId.String(),
		Operator:     req.Operator,
		Comment:      req.Comment,
		TargetNodeID: req.TargetNodeId,
	})
}

func (a *flowxEngine) GetTaskContext(ctx context.Context, _ domain.EngineType, taskID domain.ID) (domain.TaskContext, error) {
	if a.engine == nil {
		return domain.TaskContext{}, errors.New("flowx 流程引擎未初始化")
	}

	task, err := a.engine.GetTask(ctx, taskID.String())
	if err != nil {
		return domain.TaskContext{}, fmt.Errorf("flowx 查询任务详情失败: %w", err)
	}

	inst, err := a.engine.GetInstance(ctx, task.InstanceID)
	if err != nil {
		return domain.TaskContext{}, fmt.Errorf("flowx 查询流程实例详情失败 [instanceID=%s]: %w", task.InstanceID, err)
	}

	wfID := domain.ParseWorkflowIDFromProcessKey(inst.ProcKey)

	return domain.TaskContext{
		EngineType:        domain.EngineTypeFlowx,
		TaskID:            taskID,
		WorkflowID:        wfID,
		ProcessKey:        inst.ProcKey,
		ProcessID:         int(wfID), // 兼顾历史仅读取 ProcessID 的场景
		ProcessInstanceID: domain.NewID(task.InstanceID),
		ProcessVersion:    inst.Version,
		NodeID:            task.NodeID,
		NodeName:          task.NodeName,
		Assignee:          task.Assignee,
		IsFinished:        task.IsTerminal(),
		Comment:           task.Comment,
	}, nil
}

func (a *flowxEngine) GetTimeline(ctx context.Context, _ domain.EngineType, instanceID domain.ID) ([]domain.TimelineNode, error) {
	if a.engine == nil {
		return nil, errors.New("flowx 流程引擎未初始化")
	}

	events, err := a.engine.GetTimeline(ctx, instanceID.String())
	if err != nil {
		return nil, fmt.Errorf("flowx 查询流转时间线失败: %w", err)
	}

	// 转换为统一领域模型 domain.TimelineNode
	nodes := lo.FlatMap(events, func(evt flowxModel.TimelineEvent, _ int) []domain.TimelineNode {
		if len(evt.Tasks) == 0 {
			return []domain.TimelineNode{{
				NodeID:    evt.NodeID,
				NodeName:  evt.NodeName,
				Batch:     evt.BatchCode,
				Status:    lo.Ternary(evt.IsFinished, "COMPLETED", "RUNNING"),
				StartTime: evt.OccurredAt.Format(time.RFC3339),
			}}
		}

		return lo.Map(evt.Tasks, func(t *flowxModel.Task, _ int) domain.TimelineNode {
			endTimeStr := ""
			if !t.FinishTime.IsZero() {
				endTimeStr = t.FinishTime.Format(time.RFC3339)
			}
			return domain.TimelineNode{
				NodeID:    evt.NodeID,
				NodeName:  evt.NodeName,
				Batch:     evt.BatchCode,
				Operator:  t.Assignee,
				Status:    string(t.Status),
				Comment:   t.Comment,
				StartTime: t.CreateTime.Format(time.RFC3339),
				EndTime:   endTimeStr,
			}
		})
	})

	return nodes, nil
}

func (a *flowxEngine) GetInstanceExecutionView(ctx context.Context, _ domain.EngineType, instanceID domain.ID, _ uint8) (domain.InstanceExecutionView, error) {
	if a.engine == nil {
		return domain.InstanceExecutionView{}, errors.New("flowx 流程引擎未初始化")
	}

	instID := instanceID.String()
	if instID == "" {
		return domain.InstanceExecutionView{}, fmt.Errorf("%w: 流程实例 ID 不能为空", engine.ErrInvalidParameter)
	}

	view, err := a.engine.GetExecutionView(ctx, instID)
	if err != nil {
		return domain.InstanceExecutionView{}, fmt.Errorf("flowx 获取执行拓扑视图失败: %w", err)
	}

	wfID := domain.ParseWorkflowIDFromProcessKey(view.ProcessKey)

	return domain.InstanceExecutionView{
		WorkflowID:     wfID,
		ProcessKey:     view.ProcessKey,
		ProcessID:      int(wfID), // 兼顾下游仅读取 ProcessID 的场景
		ProcessVersion: view.ProcessVersion,
		NodeStatusMap:  view.NodeStatusMap,
		TraversedEdges: view.TraversedEdges,
		PendingUsers:   view.PendingUsers,
	}, nil
}

func (a *flowxEngine) Revoke(ctx context.Context, req domain.RevokeReq) error {
	if a.engine == nil {
		return errors.New("flowx 流程引擎未初始化")
	}
	return a.engine.Revoke(ctx, flowx.RevokeReq{
		InstanceID:    req.InstanceID.String(),
		Operator:      req.Operator,
		BackToStarter: false,
	})
}

func (a *flowxEngine) Transfer(ctx context.Context, req domain.TaskTransferReq) error {
	if a.engine == nil {
		return errors.New("flowx 流程引擎未初始化")
	}
	return a.engine.Transfer(ctx, req.TaskId.String(), req.TargetUsers)
}

func (a *flowxEngine) GetAutomationTask(ctx context.Context, _ domain.EngineType, instanceID domain.ID, nodeID string) (domain.TaskRecord, error) {
	if a.engine == nil {
		return domain.TaskRecord{}, errors.New("flowx 流程引擎未初始化")
	}
	instID := instanceID.String()
	if instID == "" {
		return domain.TaskRecord{}, fmt.Errorf("%w: 流程实例 ID 不能为空", engine.ErrInvalidParameter)
	}

	tasks, err := a.engine.ListTasksByInstance(ctx, instID)
	if err != nil {
		return domain.TaskRecord{}, err
	}

	task, ok := lo.Find(tasks, func(t *flowxModel.Task) bool {
		return t.NodeID == nodeID && t.IsPending()
	})
	if !ok {
		return domain.TaskRecord{}, fmt.Errorf("未找到对应的自动化节点任务 [node_id=%s]", nodeID)
	}

	return domain.TaskRecord{
		TaskID:     domain.NewID(task.ID),
		NodeID:     task.NodeID,
		NodeName:   task.NodeName,
		BatchCode:  strconv.Itoa(task.BatchCode),
		Status:     int(task.Status),
	}, nil
}

func (a *flowxEngine) GetTasksByCurrentNode(ctx context.Context, _ domain.EngineType, instanceID domain.ID, nodeID string) ([]domain.TaskRecord, error) {
	if a.engine == nil {
		return nil, errors.New("flowx 流程引擎未初始化")
	}
	instID := instanceID.String()
	if instID == "" {
		return nil, fmt.Errorf("%w: 流程实例 ID 不能为空", engine.ErrInvalidParameter)
	}

	tasks, err := a.engine.ListTasksByInstance(ctx, instID)
	if err != nil {
		return nil, err
	}

	// 针对当前节点任务过滤：
	// 1. 优先提取处于待决挂起态的任务 (如普通人工审批节点)
	// 2. 若无待决任务，提取当前节点生成的任务 (适配抄送知会等瞬时穿透定态节点，确保通知事件可正常索引)
	matchedTasks := lo.Filter(tasks, func(t *flowxModel.Task, _ int) bool {
		return t.NodeID == nodeID && t.IsPending()
	})
	if len(matchedTasks) == 0 {
		matchedTasks = lo.Filter(tasks, func(t *flowxModel.Task, _ int) bool {
			return t.NodeID == nodeID
		})
	}

	return lo.Map(matchedTasks, func(t *flowxModel.Task, _ int) domain.TaskRecord {
		return domain.TaskRecord{
			TaskID:     domain.NewID(t.ID),
			NodeID:     t.NodeID,
			NodeName:   t.NodeName,
			ApprovedBy: t.Assignee,
			Status:     int(t.Status),
			IsFinished: lo.Ternary(t.IsTerminal(), 1, 0),
		}
	}), nil
}

func (a *flowxEngine) ListTodoTasks(ctx context.Context, req domain.TodoQueryReq) ([]domain.TodoTask, int64, error) {
	if a.engine == nil {
		return nil, 0, errors.New("flowx 流程引擎未初始化")
	}

	assignee := req.Username
	tasks, total, err := a.engine.ListPendingTasks(ctx, assignee, req.Offset, req.Limit)
	if err != nil {
		return nil, 0, err
	}

	todoTasks := lo.Map(tasks, func(t *flowxModel.Task, _ int) domain.TodoTask {
		return domain.TodoTask{
			EngineType: domain.EngineTypeFlowx,
			TaskID:     domain.NewID(t.ID),
			InstanceID: domain.NewID(t.InstanceID),
			NodeID:     t.NodeID,
			NodeName:   t.NodeName,
			ApprovedBy: t.Assignee,
			CreateTime: t.CreateTime,
		}
	})

	return todoTasks, total, nil
}

func (a *flowxEngine) ListPendingSteps(ctx context.Context, processes []domain.Process) ([]domain.TodoTask, error) {
	if a.engine == nil {
		return nil, errors.New("flowx 流程引擎未初始化")
	}

	instIDs := lo.FilterMap(processes, func(p domain.Process, _ int) (string, bool) {
		idStr := p.InstanceID.String()
		return idStr, idStr != ""
	})
	if len(instIDs) == 0 {
		return nil, nil
	}

	tasks, err := a.engine.ListPendingTasksByInstances(ctx, instIDs)
	if err != nil {
		return nil, fmt.Errorf("flowx 批量检索待办步骤失败: %w", err)
	}

	return lo.Map(tasks, func(t *flowxModel.Task, _ int) domain.TodoTask {
		return domain.TodoTask{
			EngineType: domain.EngineTypeFlowx,
			TaskID:     domain.NewID(t.ID),
			InstanceID: domain.NewID(t.InstanceID),
			NodeID:     t.NodeID,
			NodeName:   t.NodeName,
			ApprovedBy: t.Assignee,
			CreateTime: t.CreateTime,
		}
	}), nil
}

func (a *flowxEngine) GetTaskRecords(ctx context.Context, _ domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskRecord, int64, error) {
	if a.engine == nil {
		return nil, 0, errors.New("flowx 流程引擎未初始化")
	}

	events, err := a.engine.GetTimeline(ctx, instanceID.String())
	if err != nil {
		return nil, 0, fmt.Errorf("flowx 查询执行记录失败: %w", err)
	}

	allTasks := lo.FlatMap(events, func(evt flowxModel.TimelineEvent, _ int) []*flowxModel.Task {
		return evt.Tasks
	})
	total := int64(len(allTasks))

	pagedTasks := paginateSlice(allTasks, offset, limit)
	records := lo.Map(pagedTasks, func(t *flowxModel.Task, _ int) domain.TaskRecord {
		endTimeStr := ""
		if !t.FinishTime.IsZero() {
			endTimeStr = t.FinishTime.Format(time.RFC3339)
		}
		return domain.TaskRecord{
			TaskID:       domain.NewID(t.ID),
			NodeID:       t.NodeID,
			BatchCode:    fmt.Sprintf("%d", t.BatchCode),
			NodeName:     t.NodeName,
			ApprovedBy:   t.Assignee,
			Status:       int(t.Status),
			Comment:      t.Comment,
			IsFinished:   lo.Ternary(t.IsTerminal(), 1, 0),
			FinishedTime: endTimeStr,
		}
	})

	return records, total, nil
}

func (a *flowxEngine) GetTaskTimeline(ctx context.Context, _ domain.EngineType, instanceID domain.ID, offset, limit int) ([]domain.TaskTimelineGroup, int64, error) {
	if a.engine == nil {
		return nil, 0, errors.New("flowx 流程引擎未初始化")
	}

	events, err := a.engine.GetTimeline(ctx, instanceID.String())
	if err != nil {
		return nil, 0, fmt.Errorf("flowx 查询时间线失败: %w", err)
	}
	total := int64(len(events))

	// 对齐 EasyFlow 审批时间线标准：按发生时间倒序呈现（最新发生节点排在最前）
	slices.SortFunc(events, func(a, b flowxModel.TimelineEvent) int {
		if !a.OccurredAt.Equal(b.OccurredAt) {
			if a.OccurredAt.After(b.OccurredAt) {
				return -1
			}
			return 1
		}
		if n := cmp.Compare(b.BatchCode, a.BatchCode); n != 0 {
			return n
		}
		return cmp.Compare(b.NodeID, a.NodeID)
	})

	pagedEvents := paginateSlice(events, offset, limit)
	groups := lo.Map(pagedEvents, func(evt flowxModel.TimelineEvent, _ int) domain.TaskTimelineGroup {
		passedCount := lo.CountBy(evt.Tasks, func(t *flowxModel.Task) bool { return t.IsPassed() })
		rejectedCount := lo.CountBy(evt.Tasks, func(t *flowxModel.Task) bool { return t.IsRejected() })
		pendingCount := lo.CountBy(evt.Tasks, func(t *flowxModel.Task) bool { return t.IsPending() })
		skippedCount := lo.CountBy(evt.Tasks, func(t *flowxModel.Task) bool { return t.Status == flowxModel.TaskSkipped })

		members := lo.Map(evt.Tasks, func(t *flowxModel.Task, _ int) domain.TaskRecord {
			return domain.TaskRecord{
				TaskID:     domain.NewID(t.ID),
				NodeID:     t.NodeID,
				BatchCode:  fmt.Sprintf("%d", t.BatchCode),
				NodeName:   t.NodeName,
				ApprovedBy: t.Assignee,
				Status:     int(t.Status),
				Comment:    t.Comment,
				IsFinished: lo.Ternary(t.IsTerminal(), 1, 0),
			}
		})

		return domain.TaskTimelineGroup{
			NodeID:        evt.NodeID,
			NodeName:      evt.NodeName,
			BatchCode:     fmt.Sprintf("%d", evt.BatchCode),
			TaskCount:     len(evt.Tasks),
			PassedCount:   passedCount,
			RejectedCount: rejectedCount,
			PendingCount:  pendingCount,
			SkippedCount:  skippedCount,
			OccurredAt:    evt.OccurredAt.Format(time.RFC3339),
			Members:       members,
		}
	})

	return groups, total, nil
}

func paginateSlice[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return nil
	}
	end := offset + limit
	if limit <= 0 || end > len(items) {
		end = len(items)
	}
	return items[offset:end]
}
