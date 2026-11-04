package kernel

import (
	"context"
	"fmt"
	"sync"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"golang.org/x/sync/singleflight"
)

// PlanCache 流程只读执行拓扑的高速内存缓存，大幅消除高频重复查询与 JSON 反序列化开销
type PlanCache struct {
	plans sync.Map // key: "proc_key:version" -> *ExecutionPlan
	group singleflight.Group
}

// NewPlanCache 实例化执行拓扑高速缓存
func NewPlanCache() *PlanCache {
	return &PlanCache{}
}

// GetOrLoad 读取缓存中的执行计划，未命中时通过 singleflight 安全回源加载
func (c *PlanCache) GetOrLoad(ctx context.Context, key string, version int, loader func(ctx context.Context) (model.ProcessDefinition, error)) (*ExecutionPlan, error) {
	cacheKey := fmt.Sprintf("%s:%d", key, version)
	if val, ok := c.plans.Load(cacheKey); ok {
		return val.(*ExecutionPlan), nil
	}

	val, err, _ := c.group.Do(cacheKey, func() (any, error) {
		if existing, ok := c.plans.Load(cacheKey); ok {
			return existing.(*ExecutionPlan), nil
		}
		def, err := loader(ctx)
		if err != nil {
			return nil, err
		}
		plan := NewExecutionPlan(def)
		c.plans.Store(cacheKey, plan)
		return plan, nil
	})
	if err != nil {
		return nil, err
	}
	return val.(*ExecutionPlan), nil
}

// Put 主动预热存入执行计划缓存
func (c *PlanCache) Put(def model.ProcessDefinition) {
	cacheKey := fmt.Sprintf("%s:%d", def.Key, def.Version)
	c.plans.Store(cacheKey, NewExecutionPlan(def))
}
