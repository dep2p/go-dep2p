// Package config 提供统一的配置管理
package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration 是支持 JSON 字符串解析的 time.Duration 包装类型
//
// 支持的格式:
//   - 字符串: "30s", "5m", "1h30m", "100ms" 等
//   - 数字: 纳秒数（用于向后兼容）
//
// 使用示例:
//
//	type Config struct {
//	    Timeout Duration `json:"timeout"`
//	}
//
//	// JSON: {"timeout": "30s"} 或 {"timeout": 30000000000}
type Duration time.Duration

// UnmarshalJSON 实现 json.Unmarshaler 接口
//
// 支持两种格式:
//   - 字符串: 使用 time.ParseDuration 解析
//   - 数字: 直接作为纳秒数
func (d *Duration) UnmarshalJSON(data []byte) error {
	// 尝试解析为字符串
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		duration, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration string %q: %w", s, err)
		}
		*d = Duration(duration)
		return nil
	}

	// 尝试解析为数字（纳秒）
	var n int64
	if err := json.Unmarshal(data, &n); err == nil {
		*d = Duration(n)
		return nil
	}

	return fmt.Errorf("duration must be a string (e.g., \"30s\") or number (nanoseconds)")
}

// MarshalJSON 实现 json.Marshaler 接口
//
// 输出为人类可读的字符串格式
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Duration 返回底层的 time.Duration 值
func (d Duration) Duration() time.Duration {
	return time.Duration(d)
}

// String 返回字符串表示
func (d Duration) String() string {
	return time.Duration(d).String()
}
