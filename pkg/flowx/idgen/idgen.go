package idgen

import (
	"fmt"
	"time"
)

// IDGenerator 工作流实例与任务物理 ID 生成契约接口，支持分布式扩展与定制规则
type IDGenerator interface {
	// GenInstanceID 生成流程实例唯一 ID
	GenInstanceID(procKey string) string
	// GenTaskID 生成审批任务唯一 ID
	GenTaskID(nodeID, assignee string, batchCode, seq int) string
}

// defaultIDGenerator 默认的 ID 生成策略，兼顾可读性与唯一性
type defaultIDGenerator struct{}

// NewDefaultIDGenerator 实例化默认 ID 生成器
func NewDefaultIDGenerator() IDGenerator {
	return &defaultIDGenerator{}
}

func (g *defaultIDGenerator) GenInstanceID(procKey string) string {
	return fmt.Sprintf("inst_%s_%d", procKey, time.Now().UnixNano())
}

func (g *defaultIDGenerator) GenTaskID(nodeID, assignee string, batchCode, seq int) string {
	return fmt.Sprintf("task_%s_%s_b%d_%d", nodeID, assignee, batchCode, seq)
}
