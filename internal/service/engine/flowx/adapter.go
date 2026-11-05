package flowx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/service/engine"
	"github.com/Duke1616/eflow/pkg/flowx"
	flowxModel "github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/samber/lo"
)

// adapter flowx 现代工作流引擎适配器实现
type adapter struct {
	engine flowx.Engine
}

// NewAdapter 实例化 flowx 驱动适配器
func NewAdapter(engine flowx.Engine) engine.IEngineAdapter {
	return &adapter{
		engine: engine,
	}
}

func (a *adapter) EngineType() domain.EngineType {
	return domain.EngineTypeFlowx
}

func (a *adapter) Deploy(ctx context.Context, rawDef any) (engine.DeployInfo, error) {
	def, ok := rawDef.(flowxModel.ProcessDefinition)
	if !ok {
		return engine.DeployInfo{}, errors.New("flowx 流程定义编译产物类型错误")
	}

	if a.engine == nil {
		return engine.DeployInfo{}, errors.New("flowx 流程引擎未初始化")
	}

	deployedDef, err := a.engine.Deploy(ctx, def)
	if err != nil {
		return engine.DeployInfo{}, fmt.Errorf("部署流程至 flowx 失败: %w", err)
	}

	return engine.DeployInfo{
		Version: deployedDef.Version,
	}, nil
}

func (a *adapter) Start(ctx context.Context, req engine.ProcessLaunchReq) (engine.ProcessLaunchResult, error) {
	if a.engine == nil {
		return engine.ProcessLaunchResult{}, errors.New("flowx 流程引擎未初始化")
	}

	procKey := fmt.Sprintf("wf_%d", req.WorkflowId)
	instID, err := a.engine.Start(ctx, flowx.StartReq{
		Key:       procKey,
		Starter:   req.Starter,
		Variables: req.Variables,
	})
	if err != nil {
		return engine.ProcessLaunchResult{}, fmt.Errorf("flowx 启动流程实例失败: %w", err)
	}

	return engine.ProcessLaunchResult{
		InstanceId:  int(req.TicketId),
		InstanceKey: instID,
	}, nil
}

func (a *adapter) Pass(ctx context.Context, req engine.TaskPassReq) error {
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

func (a *adapter) Reject(ctx context.Context, req engine.TaskRejectReq) error {
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

func (a *adapter) GetTaskContext(ctx context.Context, taskID engine.ID) (engine.TaskContext, error) {
	if a.engine == nil {
		return engine.TaskContext{}, errors.New("flowx 流程引擎未初始化")
	}

	task, err := a.engine.GetTask(ctx, taskID.String())
	if err != nil {
		return engine.TaskContext{}, fmt.Errorf("flowx 查询任务详情失败: %w", err)
	}

	return engine.TaskContext{
		EngineType:        domain.EngineTypeFlowx,
		TaskID:            taskID,
		ProcessInstanceID: engine.NewID(task.InstanceID),
		NodeID:            task.NodeID,
		NodeName:          task.NodeName,
		Assignee:          task.Assignee,
		IsFinished:        task.IsTerminal(),
		Comment:           task.Comment,
	}, nil
}

func (a *adapter) GetTimeline(ctx context.Context, instanceID engine.ID) ([]engine.TimelineNode, error) {
	if a.engine == nil {
		return nil, errors.New("flowx 流程引擎未初始化")
	}

	events, err := a.engine.GetTimeline(ctx, instanceID.String())
	if err != nil {
		return nil, fmt.Errorf("flowx 查询流转时间线失败: %w", err)
	}

	// 转换为统一领域模型 engine.TimelineNode
	nodes := lo.FlatMap(events, func(evt flowxModel.TimelineEvent, _ int) []engine.TimelineNode {
		if len(evt.Tasks) == 0 {
			return []engine.TimelineNode{{
				NodeID:    evt.NodeID,
				NodeName:  evt.NodeName,
				Batch:     evt.BatchCode,
				Status:    lo.Ternary(evt.IsFinished, "COMPLETED", "RUNNING"),
				StartTime: evt.OccurredAt.Format(time.RFC3339),
			}}
		}

		return lo.Map(evt.Tasks, func(t flowxModel.Task, _ int) engine.TimelineNode {
			endTimeStr := ""
			if t.FinishTime != nil {
				endTimeStr = t.FinishTime.Format(time.RFC3339)
			}
			return engine.TimelineNode{
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

func (a *adapter) GetInstanceExecutionView(ctx context.Context, instanceID engine.ID, status uint8) (engine.InstanceExecutionView, error) {
	return engine.InstanceExecutionView{
		NodeStatusMap:  make(map[string]int),
		TraversedEdges: make(map[string][]string),
	}, nil
}

func (a *adapter) Revoke(ctx context.Context, instanceID engine.ID, force bool, operator string) error {
	if a.engine == nil {
		return errors.New("flowx 流程引擎未初始化")
	}
	return a.engine.Revoke(ctx, flowx.RevokeReq{
		InstanceID:    instanceID.String(),
		Operator:      operator,
		BackToStarter: false,
	})
}
