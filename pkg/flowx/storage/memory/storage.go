package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
)

// Storage 纯内存存储实现（用于轻量调试与单测，零 DB 依赖，并发安全）
type Storage struct {
	mu          sync.RWMutex
	definitions map[string]model.ProcessDefinition // key:version -> Def
	instances   map[string]*model.Instance
	tasks       map[string]*model.Task
	instTasks   map[string][]*model.Task // instID -> tasks
}

// 编译期严格校验是否完整实现了 IStorage 接口
var _ storage.IStorage = (*Storage)(nil)

// NewStorage 创建内存存储实例
func NewStorage() *Storage {
	return &Storage{
		definitions: make(map[string]model.ProcessDefinition),
		instances:   make(map[string]*model.Instance),
		tasks:       make(map[string]*model.Task),
		instTasks:   make(map[string][]*model.Task),
	}
}

func (m *Storage) SaveDefinition(ctx context.Context, def model.ProcessDefinition) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := fmt.Sprintf("%s:%d", def.Key, def.Version)
	m.definitions[key] = def
	return nil
}

func (m *Storage) GetDefinition(ctx context.Context, key string, version int) (model.ProcessDefinition, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	k := fmt.Sprintf("%s:%d", key, version)
	def, ok := m.definitions[k]
	if !ok {
		return model.ProcessDefinition{}, fmt.Errorf("%w: %s", storage.ErrDefinitionNotFound, k)
	}
	return def, nil
}

// GetLatestVersion 根据流程业务唯一标识查询已发布的最大版本号，尚未发布过任何版本时返回 0, nil
func (m *Storage) GetLatestVersion(ctx context.Context, key string) (int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	maxVersion := 0
	for _, def := range m.definitions {
		if def.Key == key && def.Version > maxVersion {
			maxVersion = def.Version
		}
	}
	return maxVersion, nil
}

func (m *Storage) SaveInstance(ctx context.Context, inst *model.Instance) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *inst
	m.instances[inst.ID] = &cp
	return nil
}

func (m *Storage) GetInstance(ctx context.Context, id string) (*model.Instance, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	inst, ok := m.instances[id]
	if !ok {
		return nil, storage.ErrInstanceNotFound
	}
	cp := *inst
	return &cp, nil
}

func (m *Storage) GetInstanceForUpdate(ctx context.Context, id string) (*model.Instance, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	inst, ok := m.instances[id]
	if !ok {
		return nil, storage.ErrInstanceNotFound
	}
	cp := *inst
	return &cp, nil
}

func (m *Storage) SaveTasks(ctx context.Context, tasks []*model.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, t := range tasks {
		cp := *t
		m.tasks[t.ID] = &cp
		m.instTasks[t.InstanceID] = append(m.instTasks[t.InstanceID], &cp)
	}
	return nil
}

func (m *Storage) GetTask(ctx context.Context, id string) (*model.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, storage.ErrTaskNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *Storage) GetTaskForUpdate(ctx context.Context, id string) (*model.Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return nil, storage.ErrTaskNotFound
	}
	cp := *t
	return &cp, nil
}

func (m *Storage) ListTasksByInstance(ctx context.Context, instID string) ([]*model.Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := m.instTasks[instID]
	res := make([]*model.Task, len(list))
	for i, t := range list {
		cp := *t
		res[i] = &cp
	}
	return res, nil
}

func (m *Storage) ListPendingTasksByInstances(ctx context.Context, instIDs []string) ([]*model.Task, error) {
	if len(instIDs) == 0 {
		return []*model.Task{}, nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	idMap := make(map[string]struct{}, len(instIDs))
	for _, id := range instIDs {
		idMap[id] = struct{}{}
	}

	var result []*model.Task
	for _, t := range m.tasks {
		if _, ok := idMap[t.InstanceID]; ok && t.IsPending() {
			cp := *t
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (m *Storage) ListPendingTasks(ctx context.Context, query model.PendingTaskQuery) ([]*model.Task, int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matched []*model.Task
	for _, t := range m.tasks {
		if t.Status != model.TaskPending {
			continue
		}
		if query.Assignee != "" && t.Assignee != query.Assignee {
			continue
		}
		if query.ProcessName != "" && m.processNameOf(t.InstanceID) != query.ProcessName {
			continue
		}
		cp := *t
		matched = append(matched, &cp)
	}

	// 契约要求结果按创建时间有序：上层跨引擎归并分页依赖此有序性。
	// map 遍历顺序随机，缺少排序会让内存实现与 MySQL 实现行为分叉
	sort.SliceStable(matched, func(i, j int) bool {
		if query.SortByAsc {
			return matched[i].CreateTime.Before(matched[j].CreateTime)
		}
		return matched[i].CreateTime.After(matched[j].CreateTime)
	})

	total := int64(len(matched))
	if query.Offset >= len(matched) {
		return nil, total, nil
	}
	end := query.Offset + query.Limit
	if end > len(matched) || query.Limit <= 0 {
		end = len(matched)
	}

	res := make([]*model.Task, end-query.Offset)
	copy(res, matched[query.Offset:end])
	return res, total, nil
}

// processNameOf 经实例回溯其当时执行的流程定义名（调用方须已持有读锁）
func (m *Storage) processNameOf(instanceID string) string {
	inst, ok := m.instances[instanceID]
	if !ok {
		return ""
	}
	def, ok := m.definitions[fmt.Sprintf("%s:%d", inst.ProcKey, inst.Version)]
	if !ok {
		return ""
	}
	return def.Name
}

func (m *Storage) UpdateTask(ctx context.Context, task *model.Task) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[task.ID]; !ok {
		return storage.ErrTaskNotFound
	}
	cp := *task
	m.tasks[task.ID] = &cp

	for i, t := range m.instTasks[task.InstanceID] {
		if t.ID == task.ID {
			m.instTasks[task.InstanceID][i] = &cp
			break
		}
	}
	return nil
}

func (m *Storage) UpdateTasksStatus(ctx context.Context, taskIDs []string, status model.TaskStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, tid := range taskIDs {
		if t, ok := m.tasks[tid]; ok {
			t.Status = status
			if status != model.TaskPending {
				t.FinishTime = now
			}
			// 同步更新 instTasks
			for i, it := range m.instTasks[t.InstanceID] {
				if it.ID == tid {
					m.instTasks[t.InstanceID][i].Status = status
					m.instTasks[t.InstanceID][i].FinishTime = now
					break
				}
			}
		}
	}
	return nil
}
