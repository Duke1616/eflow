package idx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// ID 通用强类型多态自适应标识
// 抹平底层 MySQL 自增整型与分布式字符串/前缀标识差异，并原生支持 JSON 数字与字符串的双向解析
type ID string

// New 从任意整型或字符串创建强类型 ID
func New[T ~int | ~int64 | ~int32 | ~string](val T) ID {
	return ID(fmt.Sprint(val))
}

// Int 安全解析为整型，失败返回 0
func (id ID) Int() int {
	v, _ := strconv.Atoi(string(id))
	return v
}

// Int64 安全解析为 64 位整型，失败返回 0
func (id ID) Int64() int64 {
	v, _ := strconv.ParseInt(string(id), 10, 64)
	return v
}

// String 返回字符串形态
func (id ID) String() string {
	return string(id)
}

// IsZero 判断是否为零值
func (id ID) IsZero() bool {
	return id == "" || id == "0"
}

// IsNumeric 检查标识底层是否为纯数字形式（用于兼容历史整型 ID 探测）
func (id ID) IsNumeric() bool {
	if id == "" {
		return false
	}
	_, err := strconv.Atoi(string(id))
	return err == nil
}

// UnmarshalJSON 统一解析 JSON 格式的 ID，兼顾数字与字符串形态
func (id *ID) UnmarshalJSON(b []byte) error {
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		*id = ""
		return nil
	}

	var text string
	if err := json.Unmarshal(b, &text); err == nil {
		*id = ID(text)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(b, &number); err != nil {
		return fmt.Errorf("ID 必须是 JSON 字符串或数字: %w", err)
	}
	*id = ID(number.String())
	return nil
}
