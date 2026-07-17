package task

import (
	"context"
	"errors"
	"testing"
	"time"

	etaskclient "github.com/Duke1616/eflow/internal/client/etask"
	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	dispatchSvc "github.com/Duke1616/eflow/internal/service/dispatch"
	"github.com/stretchr/testify/require"
)

func TestResolveRunnerByDispatchFailsClosed(t *testing.T) {
	testCases := []struct {
		name       string
		dispatches []domain.Dispatch
		listErr    error
		runner     etaskclient.Runner
		wantMatch  bool
		wantErr    string
	}{
		{name: "没有匹配规则允许回退", dispatches: []domain.Dispatch{{
			Field: "environment", Value: "prod", RunnerId: 10,
		}}},
		{name: "规则查询失败阻断执行", listErr: errors.New("database unavailable"), wantErr: "查询自动派发规则失败"},
		{name: "匹配规则缺少执行单元", dispatches: []domain.Dispatch{{
			Field: "environment", Value: "test",
		}}, wantErr: "缺少执行单元"},
		{name: "匹配规则 Codebook 不一致", dispatches: []domain.Dispatch{{
			Field: "environment", Value: "test", RunnerId: 10,
		}}, runner: etaskclient.Runner{ID: 10, CodebookID: 99}, wantErr: "Codebook 不匹配"},
		{name: "有效匹配返回执行单元", dispatches: []domain.Dispatch{{
			Field: "environment", Value: "test", RunnerId: 10,
		}}, runner: etaskclient.Runner{ID: 10, CodebookID: 20}, wantMatch: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			svc := &taskService{
				dispatches: &dispatchServiceStub{dispatches: testCase.dispatches, err: testCase.listErr},
				runners:    &runnerCatalogStub{runner: testCase.runner},
			}
			_, matched, err := svc.resolveRunnerByDispatch(context.Background(), 1,
				easyflow.AutomationProperty{CodebookId: 20}, domain.TaskArgs{"environment": "test"})
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.wantMatch, matched)
		})
	}
}

func TestCalculateScheduledAt(t *testing.T) {
	testCases := []struct {
		name       string
		automation easyflow.AutomationProperty
		input      domain.TaskArgs
		wantDelay  time.Duration
		wantErr    string
	}{
		{name: "非定时任务立即执行"},
		{name: "手动分钟配置", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "hand", Unit: 1, Quantity: 3,
		}, wantDelay: 3 * time.Minute},
		{name: "动态字段按小时计算", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "template", TemplateField: "delay",
		}, input: domain.TaskArgs{"delay": "2"}, wantDelay: 2 * time.Hour},
		{name: "拒绝未知配置方式", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "legacy",
		}, wantErr: "不支持的定时配置方式"},
		{name: "拒绝无效动态字段", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "template", TemplateField: "delay",
		}, input: domain.TaskArgs{"delay": "abc"}, wantErr: "必须是有效整数"},
		{name: "拒绝未知时间单位", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "hand", Unit: 9, Quantity: 1,
		}, wantErr: "不支持的定时时间单位"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			before := time.Now()
			actual, err := (&taskService{}).calculateScheduledAt(testCase.automation, testCase.input)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			want := before.Add(testCase.wantDelay)
			require.WithinDuration(t, want, time.UnixMilli(actual), time.Second)
		})
	}
}

type dispatchServiceStub struct {
	dispatchSvc.Service
	dispatches []domain.Dispatch
	err        error
}

func (s *dispatchServiceStub) ListByTemplateId(context.Context, int64, int64,
	int64) ([]domain.Dispatch, int64, error) {
	return s.dispatches, int64(len(s.dispatches)), s.err
}

type runnerCatalogStub struct {
	etaskclient.RunnerCatalog
	runner etaskclient.Runner
}

func (s *runnerCatalogStub) FindByID(context.Context, int64) (etaskclient.Runner, error) {
	return s.runner, nil
}
