// Package config 提供统一的配置管理
package config

import (
	"fmt"
	"time"
)

// ConnectionHealthConfig 连接健康监控配置
//
// 配置连接级别的健康监控，用于检测网络问题和触发恢复。
type ConnectionHealthConfig struct {
	// Enabled 是否启用连接健康监控
	// 默认值: true
	Enabled bool `json:"enabled"`

	// ErrorThreshold 错误阈值
	// 达到此阈值后状态变为 Degraded
	// 默认值: 3
	ErrorThreshold int `json:"error_threshold"`

	// CriticalThreshold 严重阈值
	// 达到此阈值后状态变为 Down
	// 默认值: 10
	CriticalThreshold int `json:"critical_threshold"`

	// ErrorWindow 错误计数窗口
	// 窗口内的错误才会被计数
	// 默认值: 1m
	ErrorWindow Duration `json:"error_window"`

	// DebounceDuration 状态变更防抖时间
	// 防止状态频繁切换
	// 默认值: 5s
	DebounceDuration Duration `json:"debounce_duration"`

	// ProbeInterval 主动探测间隔
	// 默认值: 30s
	ProbeInterval Duration `json:"probe_interval"`

	// ProbeTimeout 探测超时
	// 默认值: 5s
	ProbeTimeout Duration `json:"probe_timeout"`
}

// DefaultConnectionHealthConfig 返回默认的连接健康配置
func DefaultConnectionHealthConfig() ConnectionHealthConfig {
	return ConnectionHealthConfig{
		Enabled:           true,
		ErrorThreshold:    3,
		CriticalThreshold: 10,
		ErrorWindow:       Duration(1 * time.Minute),
		DebounceDuration:  Duration(5 * time.Second),
		ProbeInterval:     Duration(30 * time.Second),
		ProbeTimeout:      Duration(5 * time.Second),
	}
}

// Validate 验证连接健康配置的有效性
func (c *ConnectionHealthConfig) Validate() error {
	if c.ErrorThreshold < 1 {
		return fmt.Errorf("connection_health: error_threshold must be >= 1")
	}
	if c.CriticalThreshold <= c.ErrorThreshold {
		return fmt.Errorf("connection_health: critical_threshold must be > error_threshold")
	}
	return nil
}
