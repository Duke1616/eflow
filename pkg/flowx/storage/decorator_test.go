package storage_test

import (
	"context"
	"testing"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/Duke1616/eflow/pkg/flowx/storage"
	"github.com/Duke1616/eflow/pkg/flowx/storage/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// LoggingStorageMock 模拟通过继承 Decorator 实现无侵入切面拦截
type LoggingStorageMock struct {
	*storage.Decorator
	InterceptedMethods []string
}

func (l *LoggingStorageMock) SaveInstance(ctx context.Context, inst *model.Instance) error {
	l.InterceptedMethods = append(l.InterceptedMethods, "SaveInstance")
	return l.Decorator.SaveInstance(ctx, inst)
}

func TestStorageDecorator(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewStorage()

	decorator := &LoggingStorageMock{
		Decorator:          storage.NewStorageDecorator(memStore),
		InterceptedMethods: make([]string, 0),
	}

	inst := &model.Instance{
		ID:          "inst_1001",
		ProcKey:     "test_proc",
		Status:      model.InstanceRunning,
		CurrentNode: "start",
	}

	err := decorator.SaveInstance(ctx, inst)
	require.NoError(t, err)
	assert.Contains(t, decorator.InterceptedMethods, "SaveInstance")

	// 验证底层存储已真实写入
	got, err := decorator.GetInstance(ctx, "inst_1001")
	require.NoError(t, err)
	assert.Equal(t, "inst_1001", got.ID)
}
