package easyflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	easyEngine "github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/samber/lo"
)

// adapter easyflow (easy-workflow) 历史遗留工作流引擎适配器实现
type adapter struct {
	repo repository.IEngineRepository
}

// NewAdapter 实例化面向 easyflow 的驱动适配器
func NewAdapter(repo repository.IEngineRepository) engine.IEngineAdapter {
	return &adapter{
		repo: repo,
	}
}

func (a *adapter) EngineType() domain.EngineType {
	return domain.EngineTypeEasyFlow
}

func (a *adapter) Deploy(ctx context.Context, rawDef any) (engine.DeployInfo, error) {
	process, ok := rawDef.(*model.Process)
	if !ok {
		return engine.DeployInfo{}, errors.New("easyflow 流程定义编译产物类型错误")
	}

	bs, err := json.Marshal(process)
	if err != nil {
		return engine.DeployInfo{}, fmt.Errorf("序列化 easyflow 流程定义失败: %w", err)
	}

	processID, err := easyEngine.ProcessSave(string(bs), process.ProcessName)
	if err != nil {
		return engine.DeployInfo{}, fmt.Errorf("保存 easyflow 流程定义失败: %w", err)
	}

	version, err := a.repo.GetLatestProcessVersion(ctx, processID)
	if err != nil {
		return engine.DeployInfo{}, fmt.Errorf("获取最新 easyflow 流程版本失败: %w", err)
	}

	return engine.DeployInfo{
		ProcessID: processID,
		Version:   version,
	}, nil
}

func (a *adapter) Start(ctx context.Context, req engine.ProcessLaunchReq) (engine.ProcessLaunchResult, error) {
	if req.ProcessId <= 0 {
		return engine.ProcessLaunchResult{}, errors.New("easyflow 流程定义 ProcessId 不能为空")
	}

	var rawVars string
	if len(req.Variables) > 0 {
		bs, err := json.Marshal(req.Variables)
		if err == nil {
			rawVars = string(bs)
		}
	}
	if rawVars == "" {
		rawVars = "{}"
	}

	instID, err := easyEngine.InstanceStart(req.ProcessId, "业务申请", req.WorkflowName, rawVars)
	if err != nil {
		return engine.ProcessLaunchResult{}, fmt.Errorf("easyflow 启动流程实例失败: %w", err)
	}

	return engine.ProcessLaunchResult{
		InstanceId:  instID,
		InstanceKey: strconv.Itoa(instID),
	}, nil
}

func (a *adapter) Pass(ctx context.Context, req engine.TaskPassReq) error {
	taskID := req.TaskId.Int()
	if taskID <= 0 {
		return fmt.Errorf("easyflow 任务 ID 无效: %s", req.TaskId)
	}

	return easyEngine.TaskPass(taskID, req.Comment, "", false)
}

func (a *adapter) Reject(ctx context.Context, req engine.TaskRejectReq) error {
	taskID := req.TaskId.Int()
	if taskID <= 0 {
		return fmt.Errorf("easyflow 任务 ID 无效: %s", req.TaskId)
	}

	return easyEngine.TaskReject(taskID, req.Comment, "")
}

func (a *adapter) GetTaskContext(ctx context.Context, taskID engine.ID) (engine.TaskContext, error) {
	id := taskID.Int()
	if id <= 0 {
		return engine.TaskContext{}, fmt.Errorf("easyflow 任务 ID 非法: %s", taskID)
	}

	task, err := easyEngine.GetTaskInfo(id)
	if err != nil {
		return engine.TaskContext{}, fmt.Errorf("查询 easyflow 任务明细失败: %w", err)
	}

	inst, err := a.repo.GetInstanceByID(ctx, task.ProcInstID)
	var procVersion int
	if err == nil {
		procVersion = inst.ProcVersion
	}

	return engine.TaskContext{
		EngineType:        domain.EngineTypeEasyFlow,
		TaskID:            taskID,
		ProcessInstanceID: engine.NewID(task.ProcInstID),
		ProcessID:         task.ProcID,
		ProcessVersion:    procVersion,
		NodeID:            task.NodeID,
		NodeName:          task.NodeName,
		Assignee:          task.UserID,
		IsFinished:        task.IsFinished == 1,
		Comment:           task.Comment,
	}, nil
}

func (a *adapter) GetTimeline(ctx context.Context, instanceID engine.ID) ([]engine.TimelineNode, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return nil, fmt.Errorf("easyflow 流程实例 ID 无效: %s", instanceID)
	}

	groups, err := a.repo.ListAccessibleTaskTimelineGroups(ctx, procInstID, 0, 1000)
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

	nodes := make([]engine.TimelineNode, 0, len(groups))
	for _, group := range groups {
		key := fmt.Sprintf("%s:%s", group.NodeID, group.BatchCode)
		taskMembers := memberMap[key]
		if len(taskMembers) == 0 {
			nodes = append(nodes, engine.TimelineNode{
				NodeID:    group.NodeID,
				NodeName:  group.NodeName,
				Status:    "PENDING",
				StartTime: group.OccurredAt.String(),
			})
			continue
		}

		for _, m := range taskMembers {
			endTimeStr := ""
			if m.FinishedTime != nil {
				endTimeStr = m.FinishedTime.String()
			}
			nodes = append(nodes, engine.TimelineNode{
				NodeID:    m.NodeID,
				NodeName:  m.NodeName,
				Operator:  m.UserID,
				Status:    strconv.Itoa(int(m.Status)),
				Comment:   m.Comment,
				EndTime:   endTimeStr,
				StartTime: group.OccurredAt.String(),
			})
		}
	}

	return nodes, nil
}

func (a *adapter) GetInstanceExecutionView(ctx context.Context, instanceID engine.ID, status uint8) (engine.InstanceExecutionView, error) {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return engine.InstanceExecutionView{}, fmt.Errorf("easyflow 流程实例 ID 无效: %s", instanceID)
	}

	inst, err := a.repo.GetInstanceByID(ctx, procInstID)
	if err != nil {
		return engine.InstanceExecutionView{}, fmt.Errorf("easyflow 获取实例详情失败: %w", err)
	}

	tasks, err := a.repo.ListTaskRecord(ctx, procInstID, 0, 1000)
	if err != nil {
		return engine.InstanceExecutionView{}, fmt.Errorf("easyflow 查询审批记录失败: %w", err)
	}

	nodeStatusMap := lo.Associate(tasks, func(task model.Task) (string, int) {
		return task.NodeID, int(task.Status)
	})

	edgeMap, err := calculateTraversedEdges(ctx, a.repo, tasks, procInstID, inst.ProcID, status)
	if err != nil {
		return engine.InstanceExecutionView{}, fmt.Errorf("easyflow 计算已流转边失败: %w", err)
	}

	return engine.InstanceExecutionView{
		ProcessID:      inst.ProcID,
		ProcessVersion: inst.ProcVersion,
		NodeStatusMap:  nodeStatusMap,
		TraversedEdges: edgeMap,
	}, nil
}

func (a *adapter) Revoke(ctx context.Context, instanceID engine.ID, force bool, operator string) error {
	procInstID := instanceID.Int()
	if procInstID <= 0 {
		return fmt.Errorf("easyflow 流程实例 ID 非法: %s", instanceID)
	}
	return easyEngine.InstanceRevoke(procInstID, force, operator)
}
