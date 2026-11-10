package idgen

import (
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSnowflakeIDGenerator_Basic(t *testing.T) {
	gen := NewDefaultIDGenerator()

	instID := gen.GenInstanceID("wf_43")
	assert.False(t, strings.HasPrefix(instID, "inst_"), "不应带 inst_ 前缀")
	assert.LessOrEqual(t, len(instID), 20, "流程实例 ID 应当紧凑轻量")

	taskID := gen.GenTaskID("957e5acc-6ca1-4103-b7f5-6513d42fec02", "luankz", 1, 1)
	assert.False(t, strings.HasPrefix(taskID, "task_"), "不应带 task_ 前缀")
	assert.LessOrEqual(t, len(taskID), 20, "任务 ID 不应再拼接 36 位长 UUID 与前缀")
}

func TestSnowflakeIDGenerator_ConcurrentUniqueness(t *testing.T) {
	gen := NewDefaultIDGenerator()
	var (
		count = 2000
		wg    sync.WaitGroup
		mu    sync.Mutex
		seen  = make(map[string]struct{}, count)
	)

	wg.Add(count)
	for i := 0; i < count; i++ {
		go func(idx int) {
			defer wg.Done()
			id := gen.GenTaskID("node_test", "tester", 1, idx)

			mu.Lock()
			_, exists := seen[id]
			seen[id] = struct{}{}
			mu.Unlock()

			assert.False(t, exists, "生成的 ID 发生冲突: %s", id)
		}(i)
	}

	wg.Wait()
	require.Len(t, seen, count)
}
