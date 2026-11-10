package task

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/flowdsl"
)

const (
	defaultScheduleTimezone = "Asia/Shanghai"
	localDateTimeLayout     = "2006-01-02 15:04:05"
	localDateLayout         = "2006-01-02"
	localTimeLayout         = "15:04:05"
	localShortTimeLayout    = "15:04"
)

func (s *taskService) calculateScheduledAt(automation flowdsl.AutoTaskConfig,
	input domain.TaskArgs, ticketTemplateID int64) (int64, error) {
	schedule, err := resolveAutomationSchedule(automation)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	switch schedule.Type {
	case flowdsl.ScheduleImmediate:
		return now.UnixMilli(), nil
	case flowdsl.ScheduleDelay:
		quantity, quantityErr := resolveDelayQuantity(schedule, input, ticketTemplateID)
		if quantityErr != nil {
			return 0, quantityErr
		}
		duration, durationErr := scheduleDuration(quantity, schedule.Unit)
		if durationErr != nil {
			return 0, durationErr
		}
		return now.Add(duration).UnixMilli(), nil
	case flowdsl.ScheduleAt:
		if err = validateTemplateSource(schedule.Source, ticketTemplateID); err != nil {
			return 0, err
		}
		location, locationErr := scheduleLocation(schedule.Timezone)
		if locationErr != nil {
			return 0, locationErr
		}
		executeAt, parseErr := resolveScheduledTime(schedule.Source, input, location)
		if parseErr != nil {
			return 0, fmt.Errorf("执行时间字段 %s 非法: %w", scheduleSourceLabel(schedule.Source), parseErr)
		}
		if !executeAt.After(now) {
			return 0, fmt.Errorf("执行时间字段 %s 必须晚于当前时间", scheduleSourceLabel(schedule.Source))
		}
		return executeAt.UnixMilli(), nil
	default:
		return 0, fmt.Errorf("不支持的执行时间类型: %s", schedule.Type)
	}
}

// resolveAutomationSchedule 提取自动化执行调度规格（若未显式配置则为立即执行）
func resolveAutomationSchedule(automation flowdsl.AutoTaskConfig) (flowdsl.ScheduleConfig, error) {
	if automation.Schedule != nil {
		return *automation.Schedule, nil
	}
	return flowdsl.ScheduleConfig{Type: flowdsl.ScheduleImmediate}, nil
}

func resolveDelayQuantity(schedule flowdsl.ScheduleConfig, input domain.TaskArgs,
	ticketTemplateID int64) (int64, error) {
	var quantity int64
	switch schedule.Source.Type {
	case flowdsl.ScheduleSourceFixed:
		quantity = schedule.Source.Value
	case flowdsl.ScheduleSourceTemplateField:
		if err := validateTemplateSource(schedule.Source, ticketTemplateID); err != nil {
			return 0, err
		}
		parsed, err := parseQuantity(input[schedule.Source.Field])
		if err != nil {
			return 0, fmt.Errorf("延迟字段 %s 非法: %w", schedule.Source.Field, err)
		}
		quantity = parsed
	default:
		return 0, fmt.Errorf("不支持的延迟来源: %s", schedule.Source.Type)
	}
	if quantity <= 0 {
		return 0, fmt.Errorf("延迟时间必须大于 0")
	}
	return quantity, nil
}

func validateTemplateSource(source flowdsl.ScheduleSource, ticketTemplateID int64) error {
	if source.Type != flowdsl.ScheduleSourceTemplateField {
		return fmt.Errorf("指定时间必须来自模板字段")
	}
	if source.Field == "" {
		return fmt.Errorf("模板字段配置缺少字段")
	}
	if source.TemplateID > 0 && ticketTemplateID != source.TemplateID {
		return fmt.Errorf("当前工单模板与执行时间字段所属模板不一致")
	}
	return nil
}

func scheduleDuration(quantity int64, unit flowdsl.ScheduleUnit) (time.Duration, error) {
	switch unit {
	case flowdsl.ScheduleUnitMinute:
		return time.Duration(quantity) * time.Minute, nil
	case flowdsl.ScheduleUnitHour:
		return time.Duration(quantity) * time.Hour, nil
	case flowdsl.ScheduleUnitDay:
		return time.Duration(quantity) * 24 * time.Hour, nil
	default:
		return 0, fmt.Errorf("不支持的延迟时间单位: %s", unit)
	}
}

func scheduleLocation(name string) (*time.Location, error) {
	if strings.TrimSpace(name) == "" {
		name = defaultScheduleTimezone
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, fmt.Errorf("无效的执行时区 %s: %w", name, err)
	}
	return location, nil
}

func scheduleSourceLabel(source flowdsl.ScheduleSource) string {
	if source.TimeField == "" {
		return source.Field
	}
	return source.Field + " + " + source.TimeField
}

func resolveScheduledTime(source flowdsl.ScheduleSource, input domain.TaskArgs, location *time.Location) (time.Time, error) {
	if source.TimeField == "" {
		return parseScheduledTime(input[source.Field], location)
	}

	date, err := parseScheduledTime(input[source.Field], location)
	if err != nil {
		return time.Time{}, fmt.Errorf("日期字段 %s %w", source.Field, err)
	}
	clock, err := parseScheduledClock(input[source.TimeField], location)
	if err != nil {
		return time.Time{}, fmt.Errorf("时间字段 %s %w", source.TimeField, err)
	}

	date = date.In(location)
	clock = clock.In(location)
	return time.Date(date.Year(), date.Month(), date.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, location), nil
}

func parseScheduledClock(value any, location *time.Location) (time.Time, error) {
	if value == nil {
		return time.Time{}, fmt.Errorf("不能为空")
	}
	switch raw := value.(type) {
	case time.Time:
		return raw.In(location), nil
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return time.Time{}, fmt.Errorf("不能为空")
		}
		if parsed, err := time.ParseInLocation(localTimeLayout, trimmed, location); err == nil {
			return parsed, nil
		}
		if parsed, err := time.ParseInLocation(localShortTimeLayout, trimmed, location); err == nil {
			return parsed, nil
		}
		return time.Time{}, fmt.Errorf("格式必须是 HH:mm:ss 或 HH:mm")
	default:
		return time.Time{}, fmt.Errorf("类型必须是时间字符串")
	}
}

func parseScheduledTime(value any, location *time.Location) (time.Time, error) {
	if value == nil {
		return time.Time{}, fmt.Errorf("不能为空")
	}
	switch raw := value.(type) {
	case time.Time:
		return raw.In(location), nil
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return time.Time{}, fmt.Errorf("不能为空")
		}
		if parsed, err := time.Parse(time.RFC3339, trimmed); err == nil {
			return parsed.In(location), nil
		}
		if parsed, err := time.ParseInLocation(localDateTimeLayout, trimmed, location); err == nil {
			return parsed, nil
		}
		if parsed, err := time.ParseInLocation(localDateLayout, trimmed, location); err == nil {
			return parsed, nil
		}
		return time.Time{}, fmt.Errorf("无法解析为有效时间格式")
	default:
		return time.Time{}, fmt.Errorf("类型必须是时间字符串")
	}
}

func parseQuantity(value any) (int64, error) {
	if value == nil {
		return 0, fmt.Errorf("不能为空")
	}
	switch raw := value.(type) {
	case int:
		return int64(raw), nil
	case int32:
		return int64(raw), nil
	case int64:
		return raw, nil
	case float64:
		return int64(raw), nil
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return 0, fmt.Errorf("不能为空")
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("必须是有效整数: %w", err)
		}
		return parsed, nil
	default:
		return 0, fmt.Errorf("无法识别的数字类型: %T", value)
	}
}
