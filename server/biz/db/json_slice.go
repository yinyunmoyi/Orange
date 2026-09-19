package db

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

// JSONStringSlice 让 GORM 把 []string 序列化为 JSON 存进 MySQL 的 JSON 列
type JSONStringSlice []string

// Value 实现 driver.Valuer：写入前编码为 JSON
func (s JSONStringSlice) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

// Scan 实现 sql.Scanner：从 DB 读出时反序列化 JSON
func (s *JSONStringSlice) Scan(value any) error {
	if value == nil {
		*s = nil
		return nil
	}
	var raw []byte
	switch v := value.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return errors.New("JSONStringSlice: unsupported scan type")
	}
	if len(raw) == 0 {
		*s = nil
		return nil
	}
	return json.Unmarshal(raw, s)
}
