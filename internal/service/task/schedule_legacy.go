package task

import (
	"fmt"

	"github.com/Duke1616/eflow/internal/pkg/easyflow"
)

// adaptLegacySchedule 将旧 is_timing/exec_method 字段转换为统一调度模型。
// 历史逻辑只在此处解释，当前调度主链不直接读取废弃字段。
func adaptLegacySchedule(automation easyflow.AutomationProperty) (easyflow.ScheduleConfig, error) {
	if !automation.IsTiming {
		return easyflow.ScheduleConfig{Type: easyflow.ScheduleImmediate}, nil
	}

	switch automation.ExecMethod {
	case "hand":
		unit, err := adaptLegacyScheduleUnit(automation.Unit)
		if err != nil {
			return easyflow.ScheduleConfig{}, err
		}
		return easyflow.ScheduleConfig{
			Type: easyflow.ScheduleDelay,
			Source: easyflow.ScheduleSource{
				Type:  easyflow.ScheduleSourceFixed,
				Value: automation.Quantity,
			},
			Unit: unit,
		}, nil
	case "template":
		return easyflow.ScheduleConfig{
			Type: easyflow.ScheduleDelay,
			Source: easyflow.ScheduleSource{
				Type:       easyflow.ScheduleSourceTemplateField,
				TemplateID: automation.TemplateId,
				Field:      automation.TemplateField,
			},
			Unit: easyflow.ScheduleUnitHour,
		}, nil
	default:
		return easyflow.ScheduleConfig{}, fmt.Errorf("不支持的定时配置方式: %s", automation.ExecMethod)
	}
}

func adaptLegacyScheduleUnit(unit uint8) (easyflow.ScheduleUnit, error) {
	switch unit {
	case 1:
		return easyflow.ScheduleUnitMinute, nil
	case 2:
		return easyflow.ScheduleUnitHour, nil
	case 3:
		return easyflow.ScheduleUnitDay, nil
	default:
		return "", fmt.Errorf("不支持的定时时间单位: %d", unit)
	}
}
