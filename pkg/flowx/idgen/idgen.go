package idgen

import (
	"fmt"
	"hash/fnv"
	"math/bits"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	// 位数分配常量
	timestampBits = 41 // 时间戳位数（可支持约 69 年）
	hashBits      = 10 // hash 值位数（业务路由与离散度空间 0-1023）
	sequenceBits  = 12 // 序列号位数（每毫秒支持 4096 个自增并发序列）

	// 位移常量
	sequenceShift  = 0
	hashShift      = sequenceBits
	timestampShift = hashBits + sequenceBits

	// 掩码常量
	sequenceMask  = (1 << sequenceBits) - 1
	hashMask      = (1 << hashBits) - 1
	timestampMask = (1 << timestampBits) - 1

	// 基准时间 - 2024年1月1日
	epochMillis = int64(1704067200000)
)

// IDGenerator 工作流实例与任务物理 ID 生成契约接口，支持分布式扩展与定制规则
type IDGenerator interface {
	// GenInstanceID 生成流程实例唯一 ID
	GenInstanceID(procKey string) string
	// GenTaskID 生成审批任务唯一 ID
	GenTaskID(nodeID, assignee string, batchCode, seq int) string
}

// snowflakeIDGenerator 基于雪花算法的高性能轻量 ID 生成器
// 核心设计：消灭冗长的前端 UUID 与纳秒时间戳拼接，回归紧凑的毫秒+散列+自增位运算组合
type snowflakeIDGenerator struct {
	sequence int64
}

// NewDefaultIDGenerator 实例化默认轻量化 ID 生成器
func NewDefaultIDGenerator() IDGenerator {
	return &snowflakeIDGenerator{}
}

// GenInstanceID 生成流程实例紧凑唯一 ID（纯数字字符串）
func (g *snowflakeIDGenerator) GenInstanceID(procKey string) string {
	id := g.generateID(0, procKey)
	return strconv.FormatInt(id, 10)
}

// GenTaskID 生成审批任务紧凑唯一 ID（纯数字字符串）
func (g *snowflakeIDGenerator) GenTaskID(nodeID, assignee string, batchCode, seq int) string {
	key := fmt.Sprintf("%s:%s:%d:%d", nodeID, assignee, batchCode, seq)
	id := g.generateID(int64(batchCode), key)
	return strconv.FormatInt(id, 10)
}

// generateID 核心位运算组装 ID
func (g *snowflakeIDGenerator) generateID(bizID int64, key string) int64 {
	timestamp := time.Now().UnixMilli() - epochMillis
	hashVal := hashKey(bizID, key)
	sequence := atomic.AddInt64(&g.sequence, 1) - 1

	return (timestamp&timestampMask)<<timestampShift |
		(hashVal&hashMask)<<hashShift |
		(sequence & sequenceMask)
}

// hashKey 基于 FNV-1a 与雪崩混淆算法计算紧凑散列值
func hashKey(bizID int64, key string) int64 {
	combined := strconv.FormatInt(bizID, 10) + ":" + key
	h := fnv.New64a()
	_, _ = h.Write([]byte(combined))
	val := h.Sum64()

	// 引入质数与位旋转消除局部低熵碰撞
	const (
		prime1 = 11400714819323198485
		prime2 = 14029467366897019727
	)
	val ^= uint64(bizID) + prime1
	val = bits.RotateLeft64(val, 13) * prime2
	val = bits.RotateLeft64(val, 29)

	return int64(val & 0x7FFFFFFFFFFFFFFF)
}
