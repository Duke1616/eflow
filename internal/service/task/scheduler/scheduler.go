package scheduler

import (
	"sync"
)

// Scheduler 自动化任务调度管理器接口
// NOTE: 用以控制任务的多节点分布式调度或本地多重调度互斥，防止重复下发
type Scheduler interface {
	// Add 尝试添加任务的调度锁，若已存在则返回 false，以此避免重复调度
	Add(taskId int64) bool
	// Remove 释放调度锁
	Remove(taskId int64)
}

type localScheduler struct {
	tasks sync.Map
}

// NewScheduler 创建默认的本地高性能任务调度器
func NewScheduler() Scheduler {
	return &localScheduler{
		tasks: sync.Map{},
	}
}

// Add 并发安全地加锁
func (s *localScheduler) Add(taskId int64) bool {
	_, loaded := s.tasks.LoadOrStore(taskId, struct{}{})
	return !loaded
}

// Remove 释放锁
func (s *localScheduler) Remove(taskId int64) {
	s.tasks.Delete(taskId)
}
