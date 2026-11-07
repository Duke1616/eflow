package easyflow

import (
	"context"
	"fmt"

	"github.com/Bunny3th/easy-workflow/workflow/engine"
	"github.com/Bunny3th/easy-workflow/workflow/model"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/repository"
	"github.com/samber/lo"
	"golang.org/x/sync/errgroup"
)

// Service easy-workflow 专有流程引擎服务接口（提供基于 EasyFlow 存储体系的待办、时间线、流转记录与底层查询）
type Service interface {
	// ListTodoTasks 查看指定用户的待办任务列表，支持按时间排序和分页检索
	ListTodoTasks(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) ([]domain.Instance, int64, error)
	// ListAllTodoTasks 分页查询经过 PBAC AccessScope 约束的待办任务，并返回使用相同条件计算的总数。
	ListAllTodoTasks(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) ([]domain.Instance, int64, error)

	// TaskRecord 获取工单流转的任务节点变更和审批历史全量记录
	TaskRecord(ctx context.Context, processInstId, offset, limit int) ([]domain.TaskRecord, int64, error)
	// AccessibleTaskRecord 获取经过工单 AccessScope 约束的流转记录。
	AccessibleTaskRecord(ctx context.Context, processInstId, offset, limit int) ([]domain.TaskRecord, int64, error)
	// AccessibleTaskTimeline 将同一节点执行批次的任务聚合为单条时间线事件。
	AccessibleTaskTimeline(ctx context.Context, processInstId, offset, limit int) ([]TaskTimelineGroup, int64, error)
	// IsReject 判断指定任务是否在先前被驳回过
	IsReject(ctx context.Context, taskId int) (bool, error)
	// GetTasksByCurrentNodeId 获取指定流程实例及当前活动节点下的所有挂起任务
	GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]domain.TaskRecord, error)
	// Pass 审批通过当前待办的流程节点，推进流程至下一层级
	Pass(ctx context.Context, taskId int, comment string) error
	// ListPendingStepsOfMyTask 列出由当前用户发起的指定流程实例列表中当前仍待处理的步骤信息
	ListPendingStepsOfMyTask(ctx context.Context, processInstIds []int, starter string) ([]domain.Instance, error)
	// GetAutomationTask 获取流程实例中等待自动触发执行的脚本或系统节点任务
	GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (domain.TaskRecord, error)
	// GetTicketIdByVariable 从流程实例的绑定参数中检索关联的业务工单唯一 ID 标识
	GetTicketIdByVariable(ctx context.Context, processInstId int) (string, error)
	// GetInstanceByID 根据实例 ID 获取流程实例基本详情信息（含历史实例归档）
	GetInstanceByID(ctx context.Context, processInstId int) (domain.Instance, error)
	// GetProcessDefineByVersion 获取指定流程 ID 与特定版本下锁定的完整流程图 DSL 定义
	GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error)
}

// TaskTimelineGroup 是时间线展示单元，保留 Members 用于前端按需展开人员处理明细。
type TaskTimelineGroup struct {
	domain.TaskTimelineGroup
	Members []domain.TaskRecord
}

type service struct {
	repo repository.IEngineRepository
}

// NewService 初始化 easy-workflow 流程引擎专有查询与操作服务
func NewService(repo repository.IEngineRepository) Service {
	return &service{
		repo: repo,
	}
}

func (s *service) GetInstanceByID(ctx context.Context, processInstId int) (domain.Instance, error) {
	return s.repo.GetInstanceByID(ctx, processInstId)
}

func (s *service) GetProcessDefineByVersion(ctx context.Context, processID, version int) (model.Process, error) {
	return s.repo.GetProcessDefineByVersion(ctx, processID, version)
}

func (s *service) GetTasksByCurrentNodeId(ctx context.Context, processInstId int, currentNodeId string) ([]domain.TaskRecord, error) {
	tasks, err := s.repo.GetTasksByCurrentNodeId(ctx, processInstId, currentNodeId)
	if err != nil {
		return nil, err
	}
	return lo.Map(tasks, func(t model.Task, _ int) domain.TaskRecord {
		return toDomainTaskRecord(t)
	}), nil
}

func (s *service) GetTicketIdByVariable(ctx context.Context, processInstId int) (string, error) {
	return s.repo.GetTicketIdByVariable(ctx, processInstId)
}

func (s *service) GetAutomationTask(ctx context.Context, currentNodeId string, processInstId int) (domain.TaskRecord, error) {
	task, err := s.repo.GetAutomationTask(ctx, currentNodeId, processInstId)
	if err != nil {
		return domain.TaskRecord{}, err
	}
	return toDomainTaskRecord(task), nil
}

func (s *service) ListPendingStepsOfMyTask(ctx context.Context, processInstIds []int, starter string) (
	[]domain.Instance, error) {
	return s.repo.ListTasksByProcInstIds(ctx, processInstIds, starter)
}

func (s *service) IsReject(ctx context.Context, taskId int) (bool, error) {
	total, err := s.repo.CountReject(ctx, taskId)
	if err != nil {
		return false, fmt.Errorf("统计 easyflow 任务驳回记录失败 [taskId=%d]: %w", taskId, err)
	}
	return total >= 1, nil
}

func (s *service) Pass(ctx context.Context, taskId int, comment string) error {
	return engine.TaskPass(taskId, comment, "", false)
}

func (s *service) TaskRecord(ctx context.Context, processInstId, offset, limit int) ([]domain.TaskRecord, int64, error) {
	var (
		eg      errgroup.Group
		records []model.Task
		total   int64
	)
	eg.Go(func() error {
		var err error
		records, err = s.repo.ListTaskRecord(ctx, processInstId, offset, limit)
		return err
	})

	eg.Go(func() error {
		var err error
		total, err = s.repo.CountTaskRecord(ctx, processInstId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, total, err
	}
	return lo.Map(records, func(t model.Task, _ int) domain.TaskRecord {
		return toDomainTaskRecord(t)
	}), total, nil
}

func (s *service) AccessibleTaskRecord(ctx context.Context, processInstId, offset, limit int) ([]domain.TaskRecord, int64, error) {
	var (
		records []model.Task
		total   int64
		eg      errgroup.Group
	)
	eg.Go(func() error {
		var err error
		records, err = s.repo.ListAccessibleTaskRecord(ctx, processInstId, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.CountAccessibleTaskRecord(ctx, processInstId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, total, err
	}
	return lo.Map(records, func(t model.Task, _ int) domain.TaskRecord {
		return toDomainTaskRecord(t)
	}), total, nil
}

func (s *service) AccessibleTaskTimeline(ctx context.Context, processInstId, offset, limit int) ([]TaskTimelineGroup, int64, error) {
	var (
		eg     errgroup.Group
		groups []domain.TaskTimelineGroup
		total  int64
	)
	eg.Go(func() error {
		var err error
		groups, err = s.repo.ListAccessibleTaskTimelineGroups(ctx, processInstId, offset, limit)
		return err
	})
	eg.Go(func() error {
		var err error
		total, err = s.repo.CountAccessibleTaskTimelineGroups(ctx, processInstId)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, 0, err
	}

	members, err := s.repo.ListAccessibleTaskTimelineMembers(ctx, processInstId, groups)
	if err != nil {
		return nil, 0, err
	}
	return attachTimelineMembers(groups, members), total, nil
}

func attachTimelineMembers(groups []domain.TaskTimelineGroup, members []model.Task) []TaskTimelineGroup {
	// 按节点与批次对任务明细进行聚合分组
	byKey := lo.GroupBy(members, func(member model.Task) string {
		return timelineGroupKey(member.NodeID, member.BatchCode)
	})

	// 声明式组装各时间线批次及其对应的任务明细
	return lo.Map(groups, func(group domain.TaskTimelineGroup, _ int) TaskTimelineGroup {
		key := timelineGroupKey(group.NodeID, group.BatchCode)
		taskList := lo.Map(byKey[key], func(task model.Task, _ int) domain.TaskRecord {
			return toDomainTaskRecord(task)
		})
		return TaskTimelineGroup{
			TaskTimelineGroup: group,
			Members:           taskList,
		}
	})
}

func timelineGroupKey(nodeID, batchCode string) string {
	return nodeID + ":batch:" + batchCode
}

func (s *service) ListTodoTasks(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) (
	[]domain.Instance, int64, error) {
	return s.listTodoTasks(ctx, userId, processName, sortByAse, offset, limit, false)
}

// ListAllTodoTasks 分页返回当前租户全部待办中经 PBAC 决策允许访问的任务及总数。
func (s *service) ListAllTodoTasks(ctx context.Context, userId, processName string, sortByAse bool, offset, limit int) ([]domain.Instance, int64, error) {
	return s.listTodoTasks(ctx, userId, processName, sortByAse, offset, limit, true)
}

func (s *service) listTodoTasks(
	ctx context.Context,
	userId, processName string,
	sortByAse bool,
	offset, limit int,
	applyAccessScope bool,
) ([]domain.Instance, int64, error) {
	var (
		eg    errgroup.Group
		ts    []domain.Instance
		total int64
	)
	eg.Go(func() error {
		var err error
		if applyAccessScope {
			ts, err = s.repo.TodoAllList(ctx, userId, processName, sortByAse, offset, limit)
		} else {
			ts, err = s.repo.TodoList(ctx, userId, processName, sortByAse, offset, limit)
		}
		return err
	})

	eg.Go(func() error {
		var err error
		if applyAccessScope {
			total, err = s.repo.CountAllTodo(ctx, userId, processName)
		} else {
			total, err = s.repo.CountTodo(ctx, userId, processName)
		}
		return err
	})
	if err := eg.Wait(); err != nil {
		return ts, total, err
	}
	return ts, total, nil
}

func toDomainTaskRecord(task model.Task) domain.TaskRecord {
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
	}
}
