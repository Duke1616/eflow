package task

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Duke1616/eflow/internal/domain"
	"github.com/Duke1616/eflow/internal/pkg/easyflow"
)

const (
	defaultScheduleTimezone = "Asia/Shanghai"
	localDateTimeLayout     = "2006-01-02 15:04:05"
	localDateLayout         = "2006-01-02"
	localTimeLayout         = "15:04:05"
	localShortTimeLayout    = "15:04"
)

func (s *taskService) calculateScheduledAt(automation easyflow.AutomationProperty,
	input domain.TaskArgs, ticketTemplateID int64) (int64, error) {
	schedule, err := resolveAutomationSchedule(automation)
	if err != nil {
		return 0, err
	}

	now := time.Now()
	switch schedule.Type {
	case easyflow.ScheduleImmediate:
		return now.UnixMilli(), nil
	case easyflow.ScheduleDelay:
		quantity, quantityErr := resolveDelayQuantity(schedule, input, ticketTemplateID)
		if quantityErr != nil {
			return 0, quantityErr
		}
		duration, durationErr := scheduleDuration(quantity, schedule.Unit)
		if durationErr != nil {
			return 0, durationErr
		}
		return now.Add(duration).UnixMilli(), nil
	case easyflow.ScheduleAt:
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

// resolveAutomationSchedule 统一当前调度模型与历史节点配置的读取入口。
func resolveAutomationSchedule(automation easyflow.AutomationProperty) (easyflow.ScheduleConfig, error) {
	if automation.Schedule != nil {
		return *automation.Schedule, nil
	}
	return adaptLegacySchedule(automation)
}

func resolveDelayQuantity(schedule easyflow.ScheduleConfig, input domain.TaskArgs,
	ticketTemplateID int64) (int64, error) {
	var quantity int64
	switch schedule.Source.Type {
	case easyflow.ScheduleSourceFixed:
		quantity = schedule.Source.Value
	case easyflow.ScheduleSourceTemplateField:
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

func validateTemplateSource(source easyflow.ScheduleSource, ticketTemplateID int64) error {
	if source.Type != easyflow.ScheduleSourceTemplateField {
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

func scheduleDuration(quantity int64, unit easyflow.ScheduleUnit) (time.Duration, error) {
	switch unit {
	case easyflow.ScheduleUnitMinute:
		return time.Duration(quantity) * time.Minute, nil
	case easyflow.ScheduleUnitHour:
		return time.Duration(quantity) * time.Hour, nil
	case easyflow.ScheduleUnitDay:
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

func scheduleSourceLabel(source easyflow.ScheduleSource) string {
	if source.TimeField == "" {
		return source.Field
	}
	return source.Field + " + " + source.TimeField
}

func resolveScheduledTime(source easyflow.ScheduleSource, input domain.TaskArgs, location *time.Location) (time.Time, error) {
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
	switch current := value.(type) {
	case time.Time:
		return current.In(location), nil
	case int:
		return time.UnixMilli(int64(current)).In(location), nil
	case int64:
		return time.UnixMilli(current).In(location), nil
	case float64:
		if current != float64(int64(current)) {
			return time.Time{}, fmt.Errorf("必须是有效时间")
		}
		return time.UnixMilli(int64(current)).In(location), nil
	case string:
		normalized := strings.TrimSpace(current)
		if normalized == "" {
			return time.Time{}, fmt.Errorf("不能为空")
		}
		for _, layout := range []string{localTimeLayout, localShortTimeLayout} {
			if parsed, err := time.ParseInLocation(layout, normalized, location); err == nil {
				return parsed, nil
			}
		}
		if parsed, err := time.Parse(time.RFC3339, normalized); err == nil {
			return parsed.In(location), nil
		}
		if parsed, err := time.ParseInLocation(localDateTimeLayout, normalized, location); err == nil {
			return parsed, nil
		}
	}
	return time.Time{}, fmt.Errorf("必须是 HH:mm:ss、HH:mm 或日期时间格式")
}

func parseScheduledTime(value any, location *time.Location) (time.Time, error) {
	switch current := value.(type) {
	case time.Time:
		return current, nil
	case int:
		return time.UnixMilli(int64(current)), nil
	case int64:
		return time.UnixMilli(current), nil
	case float64:
		if current != float64(int64(current)) {
			return time.Time{}, fmt.Errorf("时间戳必须是整数")
		}
		return time.UnixMilli(int64(current)), nil
	case string:
		normalized := strings.TrimSpace(current)
		if normalized == "" {
			return time.Time{}, fmt.Errorf("不能为空")
		}
		if parsed, err := time.Parse(time.RFC3339, normalized); err == nil {
			return parsed, nil
		}
		if parsed, err := time.ParseInLocation(localDateTimeLayout, normalized, location); err == nil {
			return parsed, nil
		}
		if parsed, err := time.ParseInLocation(localDateLayout, normalized, location); err == nil {
			return parsed, nil
		}
		return time.Time{}, fmt.Errorf("必须是 RFC3339、YYYY-MM-DD HH:mm:ss 或 YYYY-MM-DD 格式")
	default:
		return time.Time{}, fmt.Errorf("必须是日期时间或毫秒时间戳")
	}
}

func parseQuantity(value any) (int64, error) {
	switch current := value.(type) {
	case int:
		return int64(current), nil
	case int64:
		return current, nil
	case float64:
		if current != float64(int64(current)) {
			return 0, fmt.Errorf("必须是整数")
		}
		return int64(current), nil
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(current), 10, 64)
		if err == nil {
			return parsed, nil
		}
	}
	return 0, fmt.Errorf("必须是有效整数")
}
