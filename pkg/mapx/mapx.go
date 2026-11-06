package mapx

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

func get[T any](m *Map, key string, defaultValue ...T) T {
	if value, ok := read[T](m, key); ok {
		return value
	}
	if len(defaultValue) > 0 {
		return defaultValue[0]
	}
	var zero T
	return zero
}

func read[T any](m *Map, key string) (T, bool) {
	var zero T
	if m == nil || *m == nil {
		return zero, false
	}

	raw, ok := (*m)[key]
	if !ok || raw == nil {
		return zero, false
	}
	if value, ok := raw.(T); ok {
		return value, true
	}
	return convert[T](raw)
}

// Key 将字典键名与值类型绑定。
type Key[T any] struct {
	name         string
	defaultValue T
}

// NewKey 创建类型化键。
func NewKey[T any](name string, defaultValue ...T) Key[T] {
	var def T
	if len(defaultValue) > 0 {
		def = defaultValue[0]
	}
	return Key[T]{
		name:         name,
		defaultValue: def,
	}
}

func (k Key[T]) Name() string {
	return k.name
}

func (k Key[T]) Default() T {
	return k.defaultValue
}

func (k Key[T]) Has(m Map) bool {
	return m.Has(k.name)
}

func (k Key[T]) Get(m Map) T {
	return get[T](&m, k.name, k.defaultValue)
}

func (k Key[T]) GetOK(m Map) (T, bool) {
	return read[T](&m, k.name)
}

func (k Key[T]) Set(m *Map, val T) {
	m.Set(k.name, val)
}

func (k Key[T]) SetTo(m Map, val T) {
	if m != nil {
		m[k.name] = val
	}
}

func convert[T any](raw any) (T, bool) {
	targetType := reflect.TypeFor[T]()
	switch targetType.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value, ok := toInt64(raw); ok {
			return signed[T](value, targetType)
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if value, ok := toUint64(raw); ok {
			return unsigned[T](value, targetType)
		}
	case reflect.Float32, reflect.Float64:
		if value, ok := toFloat64(raw); ok {
			return floating[T](value, targetType)
		}
	case reflect.Bool:
		if value, ok := toBool(raw); ok {
			return converted[T](value, targetType)
		}
	case reflect.String:
		return converted[T](fmt.Sprint(raw), targetType)
	case reflect.Slice:
		if targetType.Elem().Kind() == reflect.String {
			if value, ok := toStringSlice(raw); ok {
				return converted[T](value, targetType)
			}
		}
	}

	var value T
	data, err := json.Marshal(raw)
	if err != nil || json.Unmarshal(data, &value) != nil {
		return value, false
	}
	return value, true
}

func converted[T any](value any, targetType reflect.Type) (T, bool) {
	raw := reflect.ValueOf(value)
	if !raw.IsValid() || !raw.Type().ConvertibleTo(targetType) {
		var zero T
		return zero, false
	}
	return raw.Convert(targetType).Interface().(T), true
}

func signed[T any](value int64, targetType reflect.Type) (T, bool) {
	target := reflect.New(targetType).Elem()
	if target.OverflowInt(value) {
		var zero T
		return zero, false
	}
	target.SetInt(value)
	return target.Interface().(T), true
}

func unsigned[T any](value uint64, targetType reflect.Type) (T, bool) {
	target := reflect.New(targetType).Elem()
	if target.OverflowUint(value) {
		var zero T
		return zero, false
	}
	target.SetUint(value)
	return target.Interface().(T), true
}

func floating[T any](value float64, targetType reflect.Type) (T, bool) {
	target := reflect.New(targetType).Elem()
	if target.OverflowFloat(value) {
		var zero T
		return zero, false
	}
	target.SetFloat(value)
	return target.Interface().(T), true
}

func toInt64(v any) (int64, bool) {
	if text, ok := numericText(v); ok {
		if value, err := strconv.ParseInt(text, 10, 64); err == nil {
			return value, true
		}
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			return floatToInt64(value)
		}
	}

	value := reflect.ValueOf(v)
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if value.Uint() > math.MaxInt64 {
			return 0, false
		}
		return int64(value.Uint()), true
	case reflect.Float32, reflect.Float64:
		return floatToInt64(value.Float())
	}
	return 0, false
}

func floatToInt64(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value ||
		value < -math.Exp2(63) || value >= math.Exp2(63) {
		return 0, false
	}
	return int64(value), true
}

func toUint64(v any) (uint64, bool) {
	if text, ok := numericText(v); ok {
		if value, err := strconv.ParseUint(text, 10, 64); err == nil {
			return value, true
		}
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			return floatToUint64(value)
		}
	}

	value := reflect.ValueOf(v)
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if value.Int() >= 0 {
			return uint64(value.Int()), true
		}
	case reflect.Float32, reflect.Float64:
		return floatToUint64(value.Float())
	}
	return 0, false
}

func floatToUint64(value float64) (uint64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value ||
		value < 0 || value >= math.Exp2(64) {
		return 0, false
	}
	return uint64(value), true
}

func toFloat64(v any) (float64, bool) {
	if text, ok := numericText(v); ok {
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			return value, true
		}
	}

	value := reflect.ValueOf(v)
	if !value.IsValid() {
		return 0, false
	}
	switch value.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(value.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(value.Uint()), true
	case reflect.Float32, reflect.Float64:
		return value.Float(), true
	}
	return 0, false
}

func numericText(value any) (string, bool) {
	switch value := value.(type) {
	case json.Number:
		return string(value), true
	case string:
		return strings.TrimSpace(value), true
	}
	valueOf := reflect.ValueOf(value)
	if valueOf.IsValid() && valueOf.Kind() == reflect.String {
		return strings.TrimSpace(valueOf.String()), true
	}
	return "", false
}

func toBool(v any) (bool, bool) {
	value := reflect.ValueOf(v)
	if value.IsValid() {
		switch value.Kind() {
		case reflect.Bool:
			return value.Bool(), true
		case reflect.String:
			switch strings.ToLower(strings.TrimSpace(value.String())) {
			case "true", "1", "yes":
				return true, true
			case "false", "0", "no":
				return false, true
			}
		}
	}
	if number, ok := toFloat64(v); ok && !math.IsNaN(number) && !math.IsInf(number, 0) {
		return number != 0, true
	}
	return false, false
}

func toStringSlice(v any) ([]string, bool) {
	if values, ok := v.([]string); ok {
		return values, true
	}

	value := reflect.ValueOf(v)
	if !value.IsValid() || value.Kind() != reflect.Slice {
		return nil, false
	}
	if value.Type().Elem().Kind() == reflect.String {
		values := make([]string, value.Len())
		for i := range values {
			values[i] = value.Index(i).String()
		}
		return values, true
	}
	if value.Type().Elem().Kind() == reflect.Interface {
		values := make([]string, value.Len())
		for i := range values {
			values[i] = fmt.Sprint(value.Index(i).Interface())
		}
		return values, true
	}
	return nil, false
}
