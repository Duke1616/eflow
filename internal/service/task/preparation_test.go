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
	templateSvc "github.com/Duke1616/eflow/internal/service/template"
	"github.com/stretchr/testify/require"
)

func TestResolveRunnerByDispatch(t *testing.T) {
	testCases := []struct {
		name         string
		dispatches   []domain.Dispatch
		listErr      error
		runners      map[int64]etaskclient.Runner
		wantMatch    bool
		wantRunnerID int64
		wantRuleID   int64
		wantErr      string
	}{
		{name: "没有匹配规则允许回退", dispatches: []domain.Dispatch{{
			AutomationNodeID: "node-1", Field: "environment", Value: "prod", RunnerId: 10,
		}}},
		{name: "规则查询失败阻断执行", listErr: errors.New("database unavailable"), wantErr: "查询执行单元路由规则失败"},
		{name: "匹配规则缺少执行单元被拒绝", dispatches: []domain.Dispatch{{
			AutomationNodeID: "node-1", Field: "environment", Value: "test",
		}}, wantErr: "缺少有效执行单元"},
		{name: "其他节点的匹配规则被忽略", dispatches: []domain.Dispatch{{
			AutomationNodeID: "node-2", Field: "environment", Value: "test", RunnerId: 10,
		}}, runners: map[int64]etaskclient.Runner{10: {ID: 10, CodebookID: 20}}},
		{name: "不同脚本文件的执行单元被拒绝", dispatches: []domain.Dispatch{
			{AutomationNodeID: "node-1", Field: "environment", Value: "test", RunnerId: 10},
		}, runners: map[int64]etaskclient.Runner{
			10: {ID: 10, CodebookID: 99},
		}, wantErr: "未绑定自动化节点脚本 20"},
		{name: "有效匹配返回执行单元", dispatches: []domain.Dispatch{{
			AutomationNodeID: "node-1", Field: "environment", Value: "test", RunnerId: 10,
			Id: 21,
		}}, runners: map[int64]etaskclient.Runner{10: {ID: 10, CodebookID: 20}},
			wantMatch: true, wantRunnerID: 10, wantRuleID: 21},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			svc := &taskService{
				dispatches: &dispatchServiceStub{dispatches: testCase.dispatches, err: testCase.listErr},
				runners:    &runnerCatalogStub{runners: testCase.runners},
			}
			runner, ruleID, err := svc.resolveRunnerByDispatch(context.Background(), 1, "node-1",
				20, domain.TaskArgs{"environment": "test"})
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, testCase.wantMatch, ruleID > 0)
			require.Equal(t, testCase.wantRuleID, ruleID)
			require.Equal(t, testCase.wantRunnerID, runner.ID)
		})
	}
}

func TestResolveRunnerUsesDefaultRunner(t *testing.T) {
	runners := &runnerCatalogStub{
		runner: etaskclient.Runner{ID: 20, CodebookID: 20},
	}
	svc := &taskService{
		dispatches: &dispatchServiceStub{dispatches: []domain.Dispatch{{
			AutomationNodeID: "node-1", Field: "environment", Value: "test", RunnerId: 10,
		}}},
		runners: runners,
	}

	decision, err := svc.resolveRunner(context.Background(), 1, "node-1",
		easyflow.AutomationProperty{CodebookId: 20, RunnerID: 20},
		domain.TaskArgs{"environment": "test"})

	require.NoError(t, err)
	require.Equal(t, domain.RunnerRouteDecision{
		DefaultRunnerID: 20, SelectedRunnerID: 20,
	}, decision)
}

func TestResolveRunnerUsesDynamicRouteWithoutDefaultRunner(t *testing.T) {
	svc := &taskService{
		dispatches: &dispatchServiceStub{dispatches: []domain.Dispatch{{
			Id: 21, AutomationNodeID: "node-1", Field: "environment", Value: "test", RunnerId: 10,
		}}},
		runners: &runnerCatalogStub{runners: map[int64]etaskclient.Runner{
			10: {ID: 10, CodebookID: 20},
		}},
	}

	decision, err := svc.resolveRunner(context.Background(), 1, "node-1",
		easyflow.AutomationProperty{CodebookId: 20}, domain.TaskArgs{"environment": "test"})

	require.NoError(t, err)
	require.Equal(t, domain.RunnerRouteDecision{SelectedRunnerID: 10, RuleID: 21}, decision)
}

func TestResolveRunnerDoesNotLoadDefaultWhenDynamicRouteMatches(t *testing.T) {
	svc := &taskService{
		dispatches: &dispatchServiceStub{dispatches: []domain.Dispatch{{
			Id: 21, AutomationNodeID: "node-1", Field: "environment", Value: "test", RunnerId: 10,
		}}},
		runners: &runnerCatalogStub{runners: map[int64]etaskclient.Runner{
			10: {ID: 10, CodebookID: 20},
		}},
	}

	decision, err := svc.resolveRunner(context.Background(), 1, "node-1",
		easyflow.AutomationProperty{CodebookId: 20, RunnerID: 99}, domain.TaskArgs{"environment": "test"})

	require.NoError(t, err)
	require.Equal(t, domain.RunnerRouteDecision{
		DefaultRunnerID: 99, SelectedRunnerID: 10, RuleID: 21,
	}, decision)
}

func TestResolveRunnerRequiresFallbackWhenNoRuleMatches(t *testing.T) {
	svc := &taskService{
		dispatches: &dispatchServiceStub{},
		runners:    &runnerCatalogStub{},
	}

	_, err := svc.resolveRunner(context.Background(), 1, "node-1",
		easyflow.AutomationProperty{CodebookId: 20}, nil)

	require.ErrorContains(t, err, "未命中动态路由，且自动化节点未配置默认执行单元")
}

func TestResolveRunnerRejectsDifferentCodebook(t *testing.T) {
	svc := &taskService{runners: &runnerCatalogStub{
		runner: etaskclient.Runner{ID: 20, CodebookID: 99},
	}}

	_, err := svc.resolveRunner(context.Background(), 0, "node-1",
		easyflow.AutomationProperty{CodebookId: 20, RunnerID: 20}, nil)

	require.ErrorContains(t, err, "未绑定自动化节点脚本 20")
}

func TestCalculateScheduledAt(t *testing.T) {
	location, err := time.LoadLocation(defaultScheduleTimezone)
	require.NoError(t, err)
	future := time.Now().In(location).Add(2 * time.Hour).Truncate(time.Second)
	futureDate := time.Now().In(location).Add(24 * time.Hour).Format(localDateLayout)
	parsedFutureDate, err := time.ParseInLocation(localDateLayout, futureDate, location)
	require.NoError(t, err)
	combinedFuture := time.Now().In(location).Add(26 * time.Hour).Truncate(time.Second)

	testCases := []struct {
		name       string
		automation easyflow.AutomationProperty
		input      domain.TaskArgs
		templateID int64
		wantDelay  time.Duration
		wantAt     time.Time
		wantErr    string
	}{
		{name: "非定时任务立即执行"},
		{name: "手动分钟配置", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "hand", Unit: 1, Quantity: 3,
		}, wantDelay: 3 * time.Minute},
		{name: "动态字段按小时计算", automation: easyflow.AutomationProperty{
			IsTiming: true, ExecMethod: "template", TemplateField: "delay",
		}, input: domain.TaskArgs{"delay": "2"}, wantDelay: 2 * time.Hour},
		{name: "新配置固定延迟", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleDelay, Unit: easyflow.ScheduleUnitMinute,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceFixed, Value: 15},
		}}, wantDelay: 15 * time.Minute},
		{name: "新配置模板数字字段", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleDelay, Unit: easyflow.ScheduleUnitDay,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceTemplateField,
				TemplateID: 10, Field: "delay"},
		}}, input: domain.TaskArgs{"delay": 2}, templateID: 10, wantDelay: 48 * time.Hour},
		{name: "模板日期字段指定执行时间", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleAt, Timezone: defaultScheduleTimezone,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceTemplateField,
				TemplateID: 10, Field: "execute_at"},
		}}, input: domain.TaskArgs{"execute_at": future.Format(localDateTimeLayout)}, templateID: 10, wantAt: future},
		{name: "模板日期字段支持日期格式", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleAt, Timezone: defaultScheduleTimezone,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceTemplateField,
				TemplateID: 10, Field: "execute_at"},
		}}, input: domain.TaskArgs{"execute_at": futureDate}, templateID: 10, wantAt: parsedFutureDate},
		{name: "组合模板日期与时间字段", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleAt, Timezone: defaultScheduleTimezone,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceTemplateField,
				TemplateID: 10, Field: "execute_date", TimeField: "execute_time"},
		}}, input: domain.TaskArgs{
			"execute_date": combinedFuture.Format(localDateLayout),
			"execute_time": combinedFuture.Format(localTimeLayout),
		}, templateID: 10, wantAt: combinedFuture},
		{name: "组合日期时间拒绝空时间字段", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleAt, Timezone: defaultScheduleTimezone,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceTemplateField,
				TemplateID: 10, Field: "execute_date", TimeField: "execute_time"},
		}}, input: domain.TaskArgs{"execute_date": combinedFuture.Format(localDateLayout)},
			templateID: 10, wantErr: "时间字段 execute_time 不能为空"},
		{name: "拒绝模板不一致", automation: easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
			Type: easyflow.ScheduleDelay, Unit: easyflow.ScheduleUnitHour,
			Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceTemplateField,
				TemplateID: 10, Field: "delay"},
		}}, input: domain.TaskArgs{"delay": 2}, templateID: 11, wantErr: "模板不一致"},
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
			actual, err := (&taskService{}).calculateScheduledAt(testCase.automation, testCase.input,
				testCase.templateID)
			if testCase.wantErr != "" {
				require.ErrorContains(t, err, testCase.wantErr)
				return
			}
			require.NoError(t, err)
			want := testCase.wantAt
			if want.IsZero() {
				want = before.Add(testCase.wantDelay)
			}
			require.WithinDuration(t, want, time.UnixMilli(actual), time.Second)
		})
	}
}

func TestApplyTemplateScheduleOverrideSupportsSharedWorkflow(t *testing.T) {
	location, err := time.LoadLocation(defaultScheduleTimezone)
	require.NoError(t, err)
	firstTime := time.Now().In(location).Add(2 * time.Hour).Truncate(time.Second)
	secondTime := time.Now().In(location).Add(3 * time.Hour).Truncate(time.Second)
	templates := map[int64]domain.Template{
		10: {
			Name: "日期和时间分离模板",
			ScheduleOverrides: domain.ScheduleOverrides{"automation-1": {
				Type: "at", Field: "deploy_date", TimeField: "deploy_time",
			}},
			Rules: []domain.Rule{
				{"type": "datePicker", "field": "deploy_date", "props": map[string]any{"type": "date"}},
				{"type": "timePicker", "field": "deploy_time", "props": map[string]any{}},
			},
		},
		20: {
			Name:              "日期时间模板",
			ScheduleOverrides: domain.ScheduleOverrides{"automation-1": {Type: "at", Field: "planned_at"}},
			Rules: []domain.Rule{
				{"type": "datePicker", "field": "planned_at", "props": map[string]any{"type": "datetime"}},
			},
		},
		30: {
			Name:  "历史模板",
			Rules: []domain.Rule{{"type": "datePicker", "field": "old_at", "props": map[string]any{"type": "datetime"}}},
		},
		40: {Name: "使用默认延迟的模板"},
		50: {
			Name: "表单延迟模板",
			ScheduleOverrides: domain.ScheduleOverrides{"automation-1": {
				Type: "delay", Field: "delay_minutes", Unit: "minute",
			}},
			Rules: []domain.Rule{{"type": "inputNumber", "field": "delay_minutes"}},
		},
	}
	svc := &taskService{templates: &scheduleTemplateServiceStub{templates: templates}}

	testCases := []struct {
		name       string
		templateID int64
		input      domain.TaskArgs
		want       time.Time
	}{
		{
			name: "第一个模板组合日期与时间", templateID: 10, want: firstTime,
			input: domain.TaskArgs{
				"deploy_date": firstTime.Format(localDateLayout),
				"deploy_time": firstTime.Format(localTimeLayout),
			},
		},
		{
			name: "第二个模板读取完整日期时间", templateID: 20, want: secondTime,
			input: domain.TaskArgs{"planned_at": secondTime.Format(localDateTimeLayout)},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			automation := easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
				Type: easyflow.ScheduleDelay, Unit: easyflow.ScheduleUnitHour,
				Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceFixed, Value: 1},
			}}
			err = svc.applyTemplateScheduleOverride(context.Background(), "automation-1", testCase.templateID, &automation)
			require.NoError(t, err)
			actual, calculateErr := svc.calculateScheduledAt(automation, testCase.input, testCase.templateID)
			require.NoError(t, calculateErr)
			require.WithinDuration(t, testCase.want, time.UnixMilli(actual), time.Second)
		})
	}

	fallback := easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
		Type: easyflow.ScheduleDelay, Unit: easyflow.ScheduleUnitMinute,
		Source: easyflow.ScheduleSource{Type: easyflow.ScheduleSourceFixed, Value: 30},
	}}
	require.NoError(t, svc.applyTemplateScheduleOverride(context.Background(), "automation-1", 40, &fallback))
	require.Equal(t, easyflow.ScheduleSourceFixed, fallback.Schedule.Source.Type)
	require.Equal(t, int64(30), fallback.Schedule.Source.Value)

	formDelay := easyflow.AutomationProperty{IsTiming: true, ExecMethod: "hand", Unit: 2, Quantity: 1}
	require.NoError(t, svc.applyTemplateScheduleOverride(context.Background(), "automation-1", 50, &formDelay))
	require.Equal(t, easyflow.ScheduleSourceTemplateField, formDelay.Schedule.Source.Type)
	require.Equal(t, easyflow.ScheduleUnitMinute, formDelay.Schedule.Unit)
	actualDelay, err := svc.calculateScheduledAt(formDelay, domain.TaskArgs{"delay_minutes": 15}, 50)
	require.NoError(t, err)
	require.WithinDuration(t, time.Now().Add(15*time.Minute), time.UnixMilli(actualDelay), time.Second)

	legacy := easyflow.AutomationProperty{Schedule: &easyflow.ScheduleConfig{
		Type: easyflow.ScheduleAt, Timezone: defaultScheduleTimezone,
		Source: easyflow.ScheduleSource{
			Type: easyflow.ScheduleSourceTemplateField, TemplateID: 30, Field: "old_at",
		},
	}}
	require.NoError(t, svc.applyTemplateScheduleOverride(context.Background(), "automation-1", 30, &legacy))
	actual, err := svc.calculateScheduledAt(legacy, domain.TaskArgs{
		"old_at": secondTime.Format(localDateTimeLayout),
	}, 30)
	require.NoError(t, err)
	require.WithinDuration(t, secondTime, time.UnixMilli(actual), time.Second)
}

func TestResolveTemplateScheduleOverrideRejectsIncompleteCurrentModel(t *testing.T) {
	rules := []domain.Rule{{"type": "inputNumber", "field": "delay"}}

	_, err := resolveTemplateScheduleOverride(domain.ScheduleOverride{Field: "delay"}, rules)
	require.ErrorContains(t, err, "不支持的模板调度类型")

	_, err = resolveTemplateScheduleOverride(domain.ScheduleOverride{Type: "delay", Field: "delay"}, rules)
	require.ErrorContains(t, err, "缺少时间单位")

	_, err = resolveTemplateScheduleOverride(domain.ScheduleOverride{Type: "delay", Field: "delay", Unit: "week"}, rules)
	require.ErrorContains(t, err, "不支持的延迟时间单位")
}

func TestResolveTemplateScheduleOverrideAcceptsDefaultDatePicker(t *testing.T) {
	rules := []domain.Rule{{
		"type": "fcRow",
		"children": []any{
			map[string]any{
				"type": "col",
				"children": []any{
					map[string]any{"type": "datePicker", "field": "schedule_date"},
				},
			},
			map[string]any{
				"type": "col",
				"children": []any{
					map[string]any{"type": "timePicker", "field": "schedule_time"},
				},
			},
		},
	}}

	schedule, err := resolveTemplateScheduleOverride(domain.ScheduleOverride{
		Type: "at", Field: "schedule_date", TimeField: "schedule_time",
	}, rules)

	require.NoError(t, err)
	require.Equal(t, easyflow.ScheduleAt, schedule.Type)
	require.Equal(t, easyflow.ScheduleSourceTemplateField, schedule.Source.Type)
	require.Equal(t, "schedule_date", schedule.Source.Field)
	require.Equal(t, "schedule_time", schedule.Source.TimeField)
}

type dispatchServiceStub struct {
	dispatchSvc.Service
	dispatches []domain.Dispatch
	err        error
}

type scheduleTemplateServiceStub struct {
	templateSvc.Service
	templates map[int64]domain.Template
}

func (s *scheduleTemplateServiceStub) DetailTemplate(_ context.Context, id int64) (domain.Template, error) {
	return s.templates[id], nil
}

func (s *dispatchServiceStub) ListByTemplateId(context.Context, int64, int64,
	int64) ([]domain.Dispatch, int64, error) {
	return s.dispatches, int64(len(s.dispatches)), s.err
}

func (s *dispatchServiceStub) ListByTemplateNode(_ context.Context, _ int64,
	nodeID string) ([]domain.Dispatch, error) {
	if s.err != nil {
		return nil, s.err
	}
	filtered := make([]domain.Dispatch, 0, len(s.dispatches))
	for _, dispatch := range s.dispatches {
		if dispatch.AutomationNodeID == nodeID {
			filtered = append(filtered, dispatch)
		}
	}
	return filtered, nil
}

type runnerCatalogStub struct {
	etaskclient.RunnerCatalog
	runner  etaskclient.Runner
	runners map[int64]etaskclient.Runner
}

func (s *runnerCatalogStub) FindByID(_ context.Context, id int64) (etaskclient.Runner, error) {
	if runner, ok := s.runners[id]; ok {
		return runner, nil
	}
	return s.runner, nil
}
