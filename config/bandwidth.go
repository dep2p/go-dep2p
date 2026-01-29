// Package config 提供统一的配置管理
package config

import "time"

// BandwidthConfig 带宽统计配置
//
// 配置带宽统计功能，支持按 Peer 和 Protocol 分类统计流量。
type BandwidthConfig struct {
	// Enabled 是否启用带宽统计
	// 默认值: true
	Enabled bool `json:"enabled"`

	// EnablePerPeer 是否启用按 Peer 统计
	// 默认值: true
	EnablePerPeer bool `json:"enable_per_peer"`

	// EnablePerProtocol 是否启用按 Protocol 统计
	// 默认值: true
	EnablePerProtocol bool `json:"enable_per_protocol"`

	// TrimInterval 清理空闲条目的间隔
	// 默认值: 5m
	TrimInterval Duration `json:"trim_interval"`

	// IdleTimeout 空闲超时，超过此时间的条目会被清理
	// 默认值: 30m
	IdleTimeout Duration `json:"idle_timeout"`
}

// DefaultBandwidthConfig 返回默认的带宽统计配置
func DefaultBandwidthConfig() BandwidthConfig {
	return BandwidthConfig{
		Enabled:           true,
		EnablePerPeer:     true,
		EnablePerProtocol: true,
		TrimInterval:      Duration(5 * time.Minute),
		IdleTimeout:       Duration(30 * time.Minute),
	}
}

// Validate 验证带宽统计配置的有效性
func (c *BandwidthConfig) Validate() error {
	// 带宽统计配置无需严格验证
	return nil
}
