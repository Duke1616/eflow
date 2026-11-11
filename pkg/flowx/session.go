package flowx

import (
	"context"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/kernel"
	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
	"github.com/samber/lo"
)

// flowSession 流程单次状态机推进生命周期内的工作单元 (Unit of Work)
// 职责收敛：统一协调底层持久化存储与内存实体快照，避免单次流转内脏读与重复查库，不侵入具体节点业务
type flowSession struct {
	storage storage.IStorage
	inst    *model.Instance
	plan    *kernel.ExecutionPlan
	tasks   []*model.Task
	dirty   bool // 流程实例运行态是否发生过修改，用于 Unit of Work 延迟收口合并写
}

func newFlowSession(store storage.IStorage, inst *model.Instance, plan *kernel.ExecutionPlan, tasks []*model.Task) *flowSession {
	return &flowSession{
		storage: store,
		inst:    inst,
		plan:    plan,
		tasks:   tasks,
		dirty:   false,
	}
}

// Instance 获取当前流程实例指针（直接操作原地状态）
func (s *flowSession) Instance() *model.Instance {
	return s.inst
}

// Plan 获取当前流程执行拓扑计划
func (s *flowSession) Plan() *kernel.ExecutionPlan {
	return s.plan
}

// AllTasks 获取当前会话维护的全量任务快照
func (s *flowSession) AllTasks() []*model.Task {
	return s.tasks
}

// GetTask 从会话内存快照中获取指定任务
func (s *flowSession) GetTask(taskID string) (*model.Task, bool) {
	return lo.Find(s.tasks, func(t *model.Task) bool {
		return t.ID == taskID
	})
}

// MaxBatchCode 获取当前会话快照中任务的最大批次号 (默认兜底为 1)
func (s *flowSession) MaxBatchCode() int {
	if len(s.tasks) == 0 {
		return 1
	}
	maxTask := lo.MaxBy(s.tasks, func(a, b *model.Task) bool {
		return a.BatchCode > b.BatchCode
	})
	if maxTask.BatchCode > 1 {
		return maxTask.BatchCode
	}
	return 1
}

// SaveTasks 批量持久化新任务，并同步追加至会话内存快照
func (s *flowSession) SaveTasks(ctx context.Context, newTasks []*model.Task) error {
	if len(newTasks) == 0 {
		return nil
	}
	if err := s.storage.SaveTasks(ctx, newTasks); err != nil {
		return err
	}
	s.tasks = append(s.tasks, newTasks...)
	return nil
}

// UpdateTask 更新单条任务，并同步刷新会话内存状态
func (s *flowSession) UpdateTask(ctx context.Context, task *model.Task) error {
	if err := s.storage.UpdateTask(ctx, task); err != nil {
		return err
	}
	for i, t := range s.tasks {
		if t.ID == task.ID {
			s.tasks[i] = task
			break
		}
	}
	return nil
}

// UpdateTasksStatus 批量更新任务状态，并同步刷新内存时间与状态
func (s *flowSession) UpdateTasksStatus(ctx context.Context, taskIDs []string, status model.TaskStatus) error {
	if len(taskIDs) == 0 {
		return nil
	}
	if err := s.storage.UpdateTasksStatus(ctx, taskIDs, status); err != nil {
		return err
	}
	idMap := lo.SliceToMap(taskIDs, func(id string) (string, struct{}) {
		return id, struct{}{}
	})
	now := time.Now()
	for i, t := range s.tasks {
		if _, exists := idMap[t.ID]; exists {
			s.tasks[i].Status = status
			s.tasks[i].FinishTime = now
		}
	}
	return nil
}

// SaveInstance 标记当前流程实例运行态已脏（延迟物理更新，彻底杜绝递归穿透造成的多次 DB UPDATE 与 Binlog 放大）
func (s *flowSession) SaveInstance(ctx context.Context) error {
	s.dirty = true
	return nil
}

// Flush 将会话中发生状态变更的流程实例原子提交至底层持久化存储（Unit of Work 统一收口）
func (s *flowSession) Flush(ctx context.Context) error {
	if !s.dirty {
		return nil
	}
	s.dirty = false
	return s.storage.SaveInstance(ctx, s.inst)
}
