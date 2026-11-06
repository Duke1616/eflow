package mapx

import (
	"encoding/json"
	"fmt"
)

// Map 表示可承载任意 JSON 字段的动态字典。
type Map map[string]any

// New 创建已初始化的动态字典。
func New() Map {
	return make(Map)
}

// Get 返回键对应的原始值。
func (m *Map) Get(key string) any {
	if m == nil {
		return nil
	}
	return (*m)[key]
}

// Has 判断键存在且值不为 nil。
func (m *Map) Has(key string) bool {
	if m == nil {
		return false
	}
	value, ok := (*m)[key]
	return ok && value != nil
}

// Ensure 确保字典已初始化。
func (m *Map) Ensure() {
	if m != nil && *m == nil {
		*m = make(Map)
	}
}

// Set 设置键值；字典为 nil 时自动初始化。
func (m *Map) Set(key string, value any) {
	if m == nil {
		return
	}
	m.Ensure()
	(*m)[key] = value
}

// Merge 将增量字典合并到当前字典；字典为 nil 时自动初始化。
func (m *Map) Merge(patch Map) {
	if m == nil || len(patch) == 0 {
		return
	}
	if *m == nil {
		*m = make(Map, len(patch))
	}
	for key, value := range patch {
		(*m)[key] = value
	}
}

// Clone 返回浅拷贝。
func (m *Map) Clone() Map {
	if m == nil {
		return nil
	}
	clone := make(Map, len(*m))
	for key, value := range *m {
		clone[key] = value
	}
	return clone
}

// GetString 获取字符串字段，并支持常见标量转换。
func (m *Map) GetString(key string, defaultValue ...string) string {
	return get[string](m, key, defaultValue...)
}

// GetInt 获取 int 字段。
func (m *Map) GetInt(key string, defaultValue ...int) int {
	return get[int](m, key, defaultValue...)
}

// GetInt64 获取 int64 字段。
func (m *Map) GetInt64(key string, defaultValue ...int64) int64 {
	return get[int64](m, key, defaultValue...)
}

// GetUint64 获取 uint64 字段。
func (m *Map) GetUint64(key string, defaultValue ...uint64) uint64 {
	return get[uint64](m, key, defaultValue...)
}

// GetFloat64 获取 float64 字段。
func (m *Map) GetFloat64(key string, defaultValue ...float64) float64 {
	return get[float64](m, key, defaultValue...)
}

// GetBool 获取布尔字段，并支持常见字符串和数字表示。
func (m *Map) GetBool(key string, defaultValue ...bool) bool {
	return get[bool](m, key, defaultValue...)
}

// GetStringSlice 获取字符串切片，并兼容 []any。
func (m *Map) GetStringSlice(key string) []string {
	return get[[]string](m, key)
}

// Bind 将字段值通过 JSON 解码到目标对象。
func (m *Map) Bind(key string, target any) error {
	raw := m.Get(key)
	if raw == nil {
		return fmt.Errorf("字典键 %q 不存在", key)
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("字典键 %q 序列化失败: %w", key, err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		return fmt.Errorf("字典键 %q 解码失败: %w", key, err)
	}
	return nil
}
