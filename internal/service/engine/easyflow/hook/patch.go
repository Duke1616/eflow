package hook

import (
	"context"

	"github.com/Bunny3th/easy-workflow/workflow/database"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	easyflowCompiler "github.com/Duke1616/eflow/internal/pkg/compiler/easyflow"
	"github.com/Duke1616/eflow/internal/repository"
)

// easyflowPatcher 封装 easyflow 引擎内部专有的代理节点与分支跳过补偿逻辑
type easyflowPatcher interface {
	// getProxyPrevNodeID 获取代理节点的前置节点标识
	getProxyPrevNodeID(ctx context.Context, processInstId int, prevNodeID string) (string, error)
	// getProxyNodeByProcessInstId 查询流程实例当前存在的代理节点标识
	getProxyNodeByProcessInstId(ctx context.Context, processInstId int) (string, error)
	// getProxyTaskByProcessInstId 查询流程实例当前的代理节点任务
	getProxyTaskByProcessInstId(ctx context.Context, processInstId int) (model.Task, error)
	// deleteProxyNodeByNodeId 清理指定代理节点记录
	deleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error
	// updateTaskPrevNodeID 修正任务的前置节点关联标识
	updateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error
	// updateIsFinishedByPreNodeId 根据前置节点更新任务完成状态与流转意见
	updateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error
	// createSkippedTask 创建分支条件不满足时的跳过任务占位记录
	createSkippedTask(ctx context.Context, processInstId int, nodeId, prevNodeId, comment string, status uint8) error
}

type defaultPatcher struct {
	repo repository.IEngineRepository
}

func newPatcher(repo repository.IEngineRepository) easyflowPatcher {
	return &defaultPatcher{repo: repo}
}

func (p *defaultPatcher) getProxyPrevNodeID(ctx context.Context, processInstId int, prevNodeID string) (string, error) {
	procTask, err := p.repo.GetProxyNodeID(ctx, processInstId, prevNodeID)
	return procTask.PrevNodeID, err
}

func (p *defaultPatcher) getProxyNodeByProcessInstId(ctx context.Context, processInstId int) (string, error) {
	procTask, err := p.repo.GetProxyNodeByProcessInstId(ctx, processInstId)
	return procTask.NodeID, err
}

func (p *defaultPatcher) getProxyTaskByProcessInstId(ctx context.Context, processInstId int) (model.Task, error) {
	return p.repo.GetProxyNodeByProcessInstId(ctx, processInstId)
}

func (p *defaultPatcher) deleteProxyNodeByNodeId(ctx context.Context, processInstId int, nodeId string) error {
	return p.repo.DeleteProxyNodeByNodeId(ctx, processInstId, nodeId)
}

func (p *defaultPatcher) updateTaskPrevNodeID(ctx context.Context, taskId int, prevNodeId string) error {
	return p.repo.UpdateTaskPrevNodeID(ctx, taskId, prevNodeId)
}

func (p *defaultPatcher) updateIsFinishedByPreNodeId(ctx context.Context, processInstId int, nodeId string, status int, comment string) error {
	return p.repo.UpdateIsFinishedByPreNodeId(ctx, processInstId, nodeId, status, comment)
}

func (p *defaultPatcher) createSkippedTask(ctx context.Context, processInstId int, nodeId, prevNodeId, comment string, status uint8) error {
	inst, err := p.repo.GetInstanceByID(ctx, processInstId)
	if err != nil {
		return err
	}

	now := database.LTime.Now()
	var createTime *database.LocalTime
	if inst.CreateTime != nil {
		t := database.LocalTime(*inst.CreateTime)
		createTime = &t
	}

	task := model.Task{
		ProcInstID:         processInstId,
		ProcID:             inst.ProcID,
		BusinessID:         inst.BusinessID,
		Starter:            inst.Starter,
		NodeID:             nodeId,
		NodeName:           easyflowCompiler.SysProxyNodeName,
		PrevNodeID:         prevNodeId,
		UserID:             "sys_skipped",
		Status:             int(status),
		IsFinished:         1,
		Comment:            comment,
		CreateTime:         &now,
		FinishedTime:       &now,
		ProcInstCreateTime: createTime,
	}

	return p.repo.CreateSkippedTask(ctx, task)
}
