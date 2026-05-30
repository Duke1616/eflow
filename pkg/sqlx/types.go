package sqlx

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JsonField 通用的 GORM JSON 序列化字段类型定义
// NOTE: 用于支持在 MySQL 中直接存取嵌套的 Map、Slice 以及复杂结构体
type JsonField[T any] struct {
	Val   T
	Valid bool
}

// Scan 实现 sql.Scanner 接口，用于反序列化
func (j *JsonField[T]) Scan(value interface{}) error {
	if value == nil {
		j.Val, j.Valid = *new(T), false
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("scan value must be []byte")
	}
	err := json.Unmarshal(bytes, &j.Val)
	if err != nil {
		return err
	}
	j.Valid = true
	return nil
}

// Value 实现 driver.Valuer 接口，用于序列化
func (j JsonField[T]) Value() (driver.Value, error) {
	if !j.Valid {
		return nil, nil
	}
	return json.Marshal(j.Val)
}
