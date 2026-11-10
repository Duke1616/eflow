package flowx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Duke1616/eflow/pkg/flowx/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapWithRetry(t *testing.T) {
	t.Run("策略为空或最大次数<=1_直接执行不重试", func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		rawWorker := func(_ context.Context, _ *TaskContext) (*TaskResult, error) {
			calls++
			return &TaskResult{}, nil
		}

		w := wrapWithRetry(rawWorker, nil)
		res, err := w(ctx, &TaskContext{})
		require.NoError(t, err)
		assert.NotNil(t, res)
		assert.Equal(t, 1, calls)
	})

	t.Run("重试并在达到上限前成功", func(t *testing.T) {
		ctx := context.Background()
		var attempts []int
		rawWorker := func(_ context.Context, tc *TaskContext) (*TaskResult, error) {
			attempts = append(attempts, tc.Attempt)
			if tc.Attempt < 3 {
				return nil, errors.New("网络抖动")
			}
			return &TaskResult{Variables: map[string]any{"done": true}}, nil
		}

		policy := &model.RetryPolicy{
			MaxAttempts:     3,
			InitialInterval: 10 * time.Millisecond,
			BackoffFactor:   2.0,
			MaxInterval:     100 * time.Millisecond,
		}

		w := wrapWithRetry(rawWorker, policy)
		res, err := w(ctx, &TaskContext{})
		require.NoError(t, err)
		assert.Equal(t, true, res.Variables["done"])
		assert.Equal(t, []int{1, 2, 3}, attempts)
	})

	t.Run("重试耗尽_向外冒泡最终错误", func(t *testing.T) {
		ctx := context.Background()
		calls := 0
		rawWorker := func(_ context.Context, _ *TaskContext) (*TaskResult, error) {
			calls++
			return nil, errors.New("持续失败")
		}

		policy := &model.RetryPolicy{
			MaxAttempts:     2,
			InitialInterval: 5 * time.Millisecond,
		}

		w := wrapWithRetry(rawWorker, policy)
		_, err := w(ctx, &TaskContext{})
		assert.ErrorContains(t, err, "持续失败")
		assert.Equal(t, 2, calls)
	})

	t.Run("Context被取消_立即退出中断重试", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		rawWorker := func(_ context.Context, _ *TaskContext) (*TaskResult, error) {
			return nil, errors.New("fail")
		}

		policy := &model.RetryPolicy{
			MaxAttempts:     5,
			InitialInterval: 100 * time.Millisecond,
		}

		go func() {
			time.Sleep(15 * time.Millisecond)
			cancel()
		}()

		w := wrapWithRetry(rawWorker, policy)
		_, err := w(ctx, &TaskContext{})
		assert.ErrorIs(t, err, context.Canceled)
	})
}
