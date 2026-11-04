package storage

import (
	"context"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

// Decorator 通用存储装饰器基类，内嵌组合 IStorage，子类只需覆写关注的方法即可实现 AOP 切面增强
type Decorator struct {
	target IStorage
}

// NewStorageDecorator 创建通用存储装饰器包装实例
func NewStorageDecorator(target IStorage) *Decorator {
	return &Decorator{target: target}
}

func (d *Decorator) Target() IStorage {
	return d.target
}

func (d *Decorator) SaveDefinition(ctx context.Context, def model.ProcessDefinition) error {
	return d.target.SaveDefinition(ctx, def)
}

func (d *Decorator) GetDefinition(ctx context.Context, key string, version int) (model.ProcessDefinition, error) {
	return d.target.GetDefinition(ctx, key, version)
}

func (d *Decorator) SaveInstance(ctx context.Context, inst *model.Instance) error {
	return d.target.SaveInstance(ctx, inst)
}

func (d *Decorator) GetInstance(ctx context.Context, id string) (*model.Instance, error) {
	return d.target.GetInstance(ctx, id)
}

func (d *Decorator) GetInstanceForUpdate(ctx context.Context, id string) (*model.Instance, error) {
	return d.target.GetInstanceForUpdate(ctx, id)
}

func (d *Decorator) SaveTasks(ctx context.Context, tasks []*model.Task) error {
	return d.target.SaveTasks(ctx, tasks)
}

func (d *Decorator) GetTask(ctx context.Context, id string) (*model.Task, error) {
	return d.target.GetTask(ctx, id)
}

func (d *Decorator) GetTaskForUpdate(ctx context.Context, id string) (*model.Task, error) {
	return d.target.GetTaskForUpdate(ctx, id)
}

func (d *Decorator) ListTasksByInstance(ctx context.Context, instID string) ([]*model.Task, error) {
	return d.target.ListTasksByInstance(ctx, instID)
}

func (d *Decorator) UpdateTask(ctx context.Context, task *model.Task) error {
	return d.target.UpdateTask(ctx, task)
}

func (d *Decorator) UpdateTasksStatus(ctx context.Context, taskIDs []string, status model.TaskStatus) error {
	return d.target.UpdateTasksStatus(ctx, taskIDs, status)
}

// 编译期静态断言确保完全实现 IStorage 契约
var _ IStorage = (*Decorator)(nil)
