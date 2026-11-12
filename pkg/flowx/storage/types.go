// 用 go run 而非 PATH 里的 mockgen：reflect 模式才能按接口名过滤（-source 模式会忽略过滤参数，
// 把三个无独立消费者的细粒度接口也生成一遍），而 reflect 模式要求 mockgen 的构建工具链不低于本模块的 Go 版本
//go:generate go run go.uber.org/mock/mockgen -package=storagemocks -destination=./mocks/storage.mock.go github.com/Duke1616/eflow/pkg/flowx/storage IStorage,ITxStorage

package storage

import (
	"context"
	"errors"

	"github.com/Duke1616/eflow/pkg/flowx/model"
)

var (
	ErrInstanceNotFound   = errors.New("流程实例不存在")
	ErrTaskNotFound       = errors.New("流程任务不存在")
	ErrDefinitionNotFound = errors.New("流程定义未找到")
	// ErrTaskAlreadyHandled 乐观状态防卫命中：任务已脱离待办态，更新被拒（并发重复处理）
	ErrTaskAlreadyHandled = errors.New("任务已被处理或不处于待办状态")
)

// IDefinitionStorage 流程定义快照存储接口，负责静态流程图版本的幂等发布与读取
type IDefinitionStorage interface {
	// SaveDefinition 保存或更新流程图定义快照，以 Key + Version 为版本边界，多次发布同版本时执行幂等覆盖
	SaveDefinition(ctx context.Context, def model.ProcessDefinition) error

	// GetDefinition 根据业务唯一标识与版本号加载流程图定义，不存在时返回 ErrDefinitionNotFound
	GetDefinition(ctx context.Context, key string, version int) (model.ProcessDefinition, error)

	// GetLatestVersion 根据流程业务唯一标识查询已发布的最大版本号，尚未发布过任何版本时返回 0, nil
	GetLatestVersion(ctx context.Context, key string) (int, error)
}

// IInstanceStorage 流程实例运行态存储接口，负责实例生命周期推进与状态快照管理（聚合根实体指针）
type IInstanceStorage interface {
	// SaveInstance 保存或更新流程实例状态与变量快照，在节点转移、变量合并或流程办结时同步最新上下文
	SaveInstance(ctx context.Context, inst *model.Instance) error

	// GetInstance 根据流程实例 ID 获取当前运行态实体，不存在时返回 ErrInstanceNotFound
	GetInstance(ctx context.Context, id string) (*model.Instance, error)

	// GetInstanceForUpdate 采用行级排他锁获取流程实例（FOR UPDATE），防止并发推进时状态覆盖
	GetInstanceForUpdate(ctx context.Context, id string) (*model.Instance, error)
}

// ITaskStorage 审批与任务明细存储接口，负责高频任务流转、状态批量变更及并发锁控制（流转任务实体指针）
type ITaskStorage interface {
	// SaveTasks 批量持久化新生成的节点审批任务，保证同批次（BatchCode）任务原子写入
	SaveTasks(ctx context.Context, tasks []*model.Task) error

	// GetTask 根据任务 ID 获取单条审批任务详情，不存在时返回 ErrTaskNotFound
	GetTask(ctx context.Context, id string) (*model.Task, error)

	// GetTaskForUpdate 采用行级排他锁获取任务（FOR UPDATE），杜绝高并发重复审批与写冲突
	GetTaskForUpdate(ctx context.Context, id string) (*model.Task, error)

	// ListTasksByInstance 查询指定流程实例下的所有历史与待办任务列表，用于会签仲裁与组装审计时间线
	ListTasksByInstance(ctx context.Context, instID string) ([]*model.Task, error)

	// ListPendingTasksByInstances 批量获取多个流程实例列表中当前处于待办状态的审批任务（一条 SQL 批量 IN 查询）
	ListPendingTasksByInstances(ctx context.Context, instIDs []string) ([]*model.Task, error)

	// ListPendingTasks 按条件分页查询待办任务，返回该条件下的真实总数（不受 offset/limit 影响）。
	// 结果必须严格按创建时间有序（方向由 query.SortByAsc 决定）：上层跨引擎归并分页依赖此有序性，
	// 只有各引擎各自的前 K 条有序，才能保证全局前 K 条包含于其并集
	ListPendingTasks(ctx context.Context, query model.PendingTaskQuery) ([]*model.Task, int64, error)

	// UpdateTask 更新单条审批任务的处理结果（状态、实际处理人、审批意见及完成时间）
	UpdateTask(ctx context.Context, task *model.Task) error

	// UpdateTasksStatus 批量更新指定任务的状态，常用于或签竞争完成或驳回时原子取消剩余的兄弟待办
	UpdateTasksStatus(ctx context.Context, taskIDs []string, status model.TaskStatus) error
}

// IStorage 工作流持久化抽象契约接口，通过细粒度领域接口正交组合而成，解耦具体存储实现
type IStorage interface {
	IDefinitionStorage
	IInstanceStorage
	ITaskStorage
}

// ITxStorage 支持物理事务闭包执行的高级存储驱动契约（如 MySQL GORM 驱动）
type ITxStorage interface {
	IStorage
	// DoTx 在底层数据库物理事务内执行操作，若 fn 返回错误或发生 panic 则自动回滚，否则原子提交
	DoTx(ctx context.Context, fn func(txStorage IStorage) error) error
}
