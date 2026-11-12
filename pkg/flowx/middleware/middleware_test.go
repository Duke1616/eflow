package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestCompose(t *testing.T) {
	testCases := []struct {
		name     string
		mws      []Middleware
		expected []string
	}{
		{
			name: "按洋葱圈顺序执行",
			mws: []Middleware{
				func(next Handler) Handler {
					return func(ctx context.Context, actCtx *ActionContext) error {
						actCtx.Operator += "->mw1_before"
						err := next(ctx, actCtx)
						actCtx.Operator += "->mw1_after"
						return err
					}
				},
				func(next Handler) Handler {
					return func(ctx context.Context, actCtx *ActionContext) error {
						actCtx.Operator += "->mw2_before"
						err := next(ctx, actCtx)
						actCtx.Operator += "->mw2_after"
						return err
					}
				},
			},
			expected: []string{"start", "mw1_before", "mw2_before", "core", "mw2_after", "mw1_after"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actCtx := &ActionContext{Operator: "start"}
			core := func(ctx context.Context, actCtx *ActionContext) error {
				actCtx.Operator += "->core"
				return nil
			}

			chain := Compose(tc.mws, core)
			err := chain(context.Background(), actCtx)
			assert.NoError(t, err)
			assert.Equal(t, "start->mw1_before->mw2_before->core->mw2_after->mw1_after", actCtx.Operator)
		})
	}
}

func TestRecovery(t *testing.T) {
	testCases := []struct {
		name        string
		panicVal    any
		expectError string
	}{
		{
			name:        "捕获普通字符串 panic 并通过 elog 兜底",
			panicVal:    "runtime exception",
			expectError: "flowx execution panic: runtime exception",
		},
		{
			name:        "捕获 error 类型 panic 并通过 elog 兜底",
			panicVal:    errors.New("db broken"),
			expectError: "flowx execution panic: db broken",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var capturedPanic any
			recMw := Recovery(func(ctx context.Context, actCtx *ActionContext, r any, stack []byte) {
				capturedPanic = r
			})

			core := func(ctx context.Context, actCtx *ActionContext) error {
				panic(tc.panicVal)
			}

			chain := Compose([]Middleware{recMw}, core)
			err := chain(context.Background(), &ActionContext{ActionType: ActionPass, InstanceID: "inst_1"})
			assert.Error(t, err)
			assert.Contains(t, err.Error(), tc.expectError)
			assert.Equal(t, tc.panicVal, capturedPanic)
		})
	}
}

func TestLogger_DefaultElog(t *testing.T) {
	// 测试开箱即用的 elog 日志中间件
	logMw := Logger()

	core := func(ctx context.Context, actCtx *ActionContext) error {
		time.Sleep(5 * time.Millisecond)
		return nil
	}

	chain := Compose([]Middleware{logMw}, core)
	err := chain(context.Background(), &ActionContext{
		ActionType: ActionPass,
		InstanceID: "inst_1001",
		TaskID:     "task_2001",
		Operator:   "alex",
	})
	assert.NoError(t, err)

	// 测试执行出错时的日志捕获
	failCore := func(ctx context.Context, actCtx *ActionContext) error {
		return errors.New("approval rejected by policy")
	}
	failChain := Compose([]Middleware{logMw}, failCore)
	err = failChain(context.Background(), &ActionContext{
		ActionType: ActionPass,
		InstanceID: "inst_1001",
		TaskID:     "task_2001",
		Operator:   "alex",
	})
	assert.Error(t, err)
}

func TestCustomLogger(t *testing.T) {
	var loggedCost time.Duration
	var loggedAction ActionType

	logMw := CustomLogger(func(ctx context.Context, actCtx *ActionContext, cost time.Duration, err error) {
		loggedCost = cost
		loggedAction = actCtx.ActionType
	})

	core := func(ctx context.Context, actCtx *ActionContext) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	}

	chain := Compose([]Middleware{logMw}, core)
	err := chain(context.Background(), &ActionContext{ActionType: ActionPass})
	assert.NoError(t, err)
	assert.Equal(t, ActionPass, loggedAction)
	assert.GreaterOrEqual(t, loggedCost, 10*time.Millisecond)
}

// TestAuditOrder_LoggerOutsideRecovery 固化 ioc.InitFlowxEngine 的装配顺序：Logger 在外、Recovery 在内。
// core panic 时 Recovery 把它转成 error，外层 Logger 仍记得下这次流转；反过来装配则 Logger 被 panic 直接跳过，
// 审计记录丢失。用同一 panic 做顺序差分，证明这个先后不是随手写的
func TestAuditOrder_LoggerOutsideRecovery(t *testing.T) {
	testCases := []struct {
		name        string
		loggerOuter bool
		wantAudited bool
	}{
		{name: "Logger外层_Recovery内层_panic仍留审计", loggerOuter: true, wantAudited: true},
		{name: "Recovery外层_Logger内层_panic跳过审计", loggerOuter: false, wantAudited: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			audited := false
			var auditErr error
			logMw := CustomLogger(func(_ context.Context, _ *ActionContext, _ time.Duration, err error) {
				audited = true
				auditErr = err
			})
			recMw := Recovery()

			mws := []Middleware{recMw, logMw}
			if tc.loggerOuter {
				mws = []Middleware{logMw, recMw}
			}

			core := func(context.Context, *ActionContext) error { panic("worker 炸了") }
			err := Compose(mws, core)(context.Background(), &ActionContext{ActionType: ActionPass})

			assert.Error(t, err, "两种顺序都必须把 panic 收成 error，不得炸穿主协程")
			assert.Equal(t, tc.wantAudited, audited)
			if tc.wantAudited {
				assert.ErrorContains(t, auditErr, "flowx execution panic")
			}
		})
	}
}
