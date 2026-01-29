// Package config 提供统一的配置管理
package config

import (
	"fmt"
	"time"
)

// RecoveryConfig 网络恢复配置
//
// 配置网络恢复机制，用于在网络问题时自动恢复连接。
type RecoveryConfig struct {
	// Enabled 是否启用网络恢复
	// 默认值: true
	Enabled bool `json:"enabled"`

	// RecoveryTimeout 恢复操作超时
	// 默认值: 30s
	RecoveryTimeout Duration `json:"recovery_timeout"`

	// RebindEnabled 是否启用传输层重绑定
	// 默认值: true
	RebindEnabled bool `json:"rebind_enabled"`

	// DiscoveryEnabled 是否启用地址重发现
	// 默认值: true
	DiscoveryEnabled bool `json:"discovery_enabled"`

	// ReconnectEnabled 是否启用关键节点重连
	// 默认值: true
	ReconnectEnabled bool `json:"reconnect_enabled"`

	// MaxConcurrentReconnects 最大并发重连数
	// 默认值: 5
	MaxConcurrentReconnects int `json:"max_concurrent_reconnects"`

	// ReconnectBackoff 重连退避基础时间
	// 默认值: 1s
	ReconnectBackoff Duration `json:"reconnect_backoff"`

	// MaxReconnectBackoff 最大重连退避时间
	// 默认值: 60s
	MaxReconnectBackoff Duration `json:"max_reconnect_backoff"`
}

// DefaultRecoveryConfig 返回默认的恢复配置
func DefaultRecoveryConfig() RecoveryConfig {
	return RecoveryConfig{
		Enabled:                 true,
		RecoveryTimeout:         Duration(30 * time.Second),
		RebindEnabled:           true,
		DiscoveryEnabled:        true,
		ReconnectEnabled:        true,
		MaxConcurrentReconnects: 5,
		ReconnectBackoff:        Duration(1 * time.Second),
		MaxReconnectBackoff:     Duration(60 * time.Second),
	}
}

// Validate 验证恢复配置的有效性
func (c *RecoveryConfig) Validate() error {
	if c.MaxConcurrentReconnects < 1 {
		return fmt.Errorf("recovery: max_concurrent_reconnects must be >= 1")
	}
	return nil
}
