// Package config 提供统一的配置管理
package config

import (
	"fmt"
	"time"
)

// PathHealthConfig 路径健康管理配置
//
// 配置路径健康监控，用于跟踪和管理到各个 Peer 的多条路径的健康状态。
type PathHealthConfig struct {
	// Enabled 是否启用路径健康管理
	// 默认值: true
	Enabled bool `json:"enabled"`

	// ProbeInterval 探测间隔
	// 默认值: 30s
	ProbeInterval Duration `json:"probe_interval"`

	// ProbeTimeout 单次探测超时
	// 默认值: 5s
	ProbeTimeout Duration `json:"probe_timeout"`

	// HealthyThreshold 判定为健康的连续成功次数
	// 默认值: 2
	HealthyThreshold int `json:"healthy_threshold"`

	// SuspectThreshold 判定为 Suspect 的连续失败次数
	// 默认值: 2
	SuspectThreshold int `json:"suspect_threshold"`

	// DeadThreshold 判定为 Dead 的连续失败次数
	// 默认值: 5
	DeadThreshold int `json:"dead_threshold"`

	// EWMAAlpha EWMA 平滑系数 (0-1)
	// 较小的值使 RTT 更平滑，较大的值使 RTT 更敏感
	// 默认值: 0.2
	EWMAAlpha float64 `json:"ewma_alpha"`

	// SwitchHysteresis 路径切换滞后阈值
	// 新路径必须比当前路径好这么多才会切换
	// 默认值: 1.5 (50% 更好)
	SwitchHysteresis float64 `json:"switch_hysteresis"`

	// StabilityWindow 稳定性窗口
	// 路径必须稳定这么长时间才能用于切换
	// 默认值: 30s
	StabilityWindow Duration `json:"stability_window"`

	// PathExpiry 路径过期时间
	// 超过此时间没有活动的路径会被清理
	// 默认值: 10m
	PathExpiry Duration `json:"path_expiry"`

	// CleanupInterval 清理间隔
	// 默认值: 1m
	CleanupInterval Duration `json:"cleanup_interval"`
}

// DefaultPathHealthConfig 返回默认的路径健康配置
func DefaultPathHealthConfig() PathHealthConfig {
	return PathHealthConfig{
		Enabled:          true,
		ProbeInterval:    Duration(30 * time.Second),
		ProbeTimeout:     Duration(5 * time.Second),
		HealthyThreshold: 2,
		SuspectThreshold: 2,
		DeadThreshold:    5,
		EWMAAlpha:        0.2,
		SwitchHysteresis: 1.5,
		StabilityWindow:  Duration(30 * time.Second),
		PathExpiry:       Duration(10 * time.Minute),
		CleanupInterval:  Duration(1 * time.Minute),
	}
}

// Validate 验证路径健康配置的有效性
func (c *PathHealthConfig) Validate() error {
	if c.EWMAAlpha < 0 || c.EWMAAlpha > 1 {
		return fmt.Errorf("path_health: ewma_alpha must be between 0 and 1")
	}
	if c.SwitchHysteresis < 1 {
		return fmt.Errorf("path_health: switch_hysteresis must be >= 1")
	}
	return nil
}
