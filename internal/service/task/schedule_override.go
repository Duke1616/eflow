package task

import (
	"context"
	"fmt"
	"strings"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
	formrule "github.com/Duke1616/eflow/internal/pkg/rule"
)

func (s *taskService) applyTemplateScheduleOverride(ctx context.Context, nodeID string,
	templateID int64, automation *easyflow.AutomationProperty) error {
	fallback, err := resolveAutomationSchedule(*automation)
	if err != nil {
		return err
	}
	if fallback.Type == easyflow.ScheduleImmediate || s.templates == nil || templateID <= 0 {
		return nil
	}

	template, err := s.templates.DetailTemplate(ctx, templateID)
	if err != nil {
		return fmt.Errorf("查询工单模板调度覆盖失败: %w", err)
	}
	override, ok := template.ScheduleOverrides[nodeID]
	if !ok {
		return nil
	}

	effective, err := resolveTemplateScheduleOverride(override, template.Rules)
	if err != nil {
		return fmt.Errorf("模板 %s 的调度覆盖无效: %w", template.Name, err)
	}
	automation.Schedule = &effective
	return nil
}

// resolveTemplateScheduleOverride 将模板调度覆盖解析成统一的运行时配置。
func resolveTemplateScheduleOverride(override domain.ScheduleOverride, rules []domain.Rule) (easyflow.ScheduleConfig, error) {
	parsed, err := formrule.ParseRules(rules)
	if err != nil {
		return easyflow.ScheduleConfig{}, fmt.Errorf("解析模板字段失败: %w", err)
	}
	fields := make(map[string]formrule.Rule, len(parsed))
	for _, item := range parsed {
		fields[item.Field] = item
	}
	primary, ok := fields[override.Field]
	if !ok {
		return easyflow.ScheduleConfig{}, fmt.Errorf("字段 %s 不存在", override.Field)
	}

	source := easyflow.ScheduleSource{
		Type:      easyflow.ScheduleSourceTemplateField,
		Field:     override.Field,
		TimeField: override.TimeField,
	}
	switch easyflow.ScheduleType(override.Type) {
	case easyflow.ScheduleDelay:
		if !isNumberRule(primary) {
			return easyflow.ScheduleConfig{}, fmt.Errorf("延迟字段 %s 必须是数字字段", override.Field)
		}
		unit := easyflow.ScheduleUnit(override.Unit)
		if unit == "" {
			return easyflow.ScheduleConfig{}, fmt.Errorf("表单延迟覆盖缺少时间单位")
		}
		if _, durationErr := scheduleDuration(1, unit); durationErr != nil {
			return easyflow.ScheduleConfig{}, durationErr
		}
		return easyflow.ScheduleConfig{Type: easyflow.ScheduleDelay, Source: source, Unit: unit}, nil
	case easyflow.ScheduleAt:
		kind := dateRuleKind(primary)
		if kind == "" {
			return easyflow.ScheduleConfig{}, fmt.Errorf("执行日期字段 %s 必须是日期或日期时间字段", override.Field)
		}
		if kind == "date" {
			clock, exists := fields[override.TimeField]
			if override.TimeField == "" || !exists || !isTimeRule(clock) {
				return easyflow.ScheduleConfig{}, fmt.Errorf("纯日期字段必须配套有效的时间字段")
			}
		}
		return easyflow.ScheduleConfig{
			Type: easyflow.ScheduleAt, Source: source, Timezone: defaultScheduleTimezone,
		}, nil
	default:
		return easyflow.ScheduleConfig{}, fmt.Errorf("不支持的模板调度类型: %s", override.Type)
	}
}

func isNumberRule(item formrule.Rule) bool {
	typeName := strings.ToLower(item.Type)
	return typeName == "number" || typeName == "inputnumber"
}

func dateRuleKind(item formrule.Rule) string {
	typeName := strings.ToLower(item.Type)
	if typeName == "datetime" {
		return "datetime"
	}
	if typeName == "date" {
		return "date"
	}
	if typeName != "datepicker" {
		return ""
	}
	pickerType := strings.ToLower(fmt.Sprint(item.Props["type"]))
	if pickerType == "datetime" {
		return "datetime"
	}
	if pickerType == "" || pickerType == "date" {
		return "date"
	}
	return ""
}

func isTimeRule(item formrule.Rule) bool {
	typeName := strings.ToLower(item.Type)
	if typeName != "time" && typeName != "timepicker" {
		return false
	}
	isRange, _ := item.Props["isRange"].(bool)
	return !isRange
}
