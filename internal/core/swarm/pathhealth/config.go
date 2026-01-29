// Package pathhealth 提供路径健康管理功能
package pathhealth

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Config 路径健康管理配置
type Config struct {
	// EWMAAlpha EWMA 平滑系数 (0-1)
	// 较小的值使 RTT 更平滑，较大的值更敏感
	EWMAAlpha float64

	// HealthyRTTThreshold 健康 RTT 阈值
	HealthyRTTThreshold time.Duration

	// SuspectRTTThreshold 可疑 RTT 阈值
	SuspectRTTThreshold time.Duration

	// DeadFailureThreshold 死亡判定的连续失败阈值
	DeadFailureThreshold int

	// ProbeInterval 主动探测间隔
	ProbeInterval time.Duration

	// SuspectProbeInterval 可疑路径探测间隔
	SuspectProbeInterval time.Duration

	// SwitchHysteresis 切换滞后阈值 (0-1)
	SwitchHysteresis float64

	// StabilityWindow 稳定性窗口
	StabilityWindow time.Duration

	// DirectPathBonus 直连路径评分加成 (0-1)
	DirectPathBonus float64

	// MaxPathsPerPeer 每个 Peer 最大跟踪路径数
	MaxPathsPerPeer int

	// PathExpiry 路径过期时间
	PathExpiry time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		EWMAAlpha:            0.2,
		HealthyRTTThreshold:  200 * time.Millisecond,
		SuspectRTTThreshold:  500 * time.Millisecond,
		DeadFailureThreshold: 3,
		ProbeInterval:        30 * time.Second,
		SuspectProbeInterval: 5 * time.Second,
		SwitchHysteresis:     0.2,
		StabilityWindow:      5 * time.Second,
		DirectPathBonus:      0.8,
		MaxPathsPerPeer:      10,
		PathExpiry:           10 * time.Minute,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.EWMAAlpha <= 0 || c.EWMAAlpha > 1 {
		c.EWMAAlpha = 0.2
	}
	if c.HealthyRTTThreshold <= 0 {
		c.HealthyRTTThreshold = 200 * time.Millisecond
	}
	if c.SuspectRTTThreshold <= 0 {
		c.SuspectRTTThreshold = 500 * time.Millisecond
	}
	if c.DeadFailureThreshold <= 0 {
		c.DeadFailureThreshold = 3
	}
	if c.ProbeInterval <= 0 {
		c.ProbeInterval = 30 * time.Second
	}
	if c.SuspectProbeInterval <= 0 {
		c.SuspectProbeInterval = 5 * time.Second
	}
	if c.SwitchHysteresis < 0 || c.SwitchHysteresis > 1 {
		c.SwitchHysteresis = 0.2
	}
	if c.StabilityWindow < 0 {
		c.StabilityWindow = 5 * time.Second
	}
	// 注意: StabilityWindow = 0 是合法的，表示禁用稳定性检查
	if c.DirectPathBonus <= 0 || c.DirectPathBonus > 1 {
		c.DirectPathBonus = 0.8
	}
	if c.MaxPathsPerPeer <= 0 {
		c.MaxPathsPerPeer = 10
	}
	if c.PathExpiry <= 0 {
		c.PathExpiry = 10 * time.Minute
	}
	return nil
}

// ToInterfaceConfig 转换为接口配置
func (c *Config) ToInterfaceConfig() interfaces.PathHealthConfig {
	return interfaces.PathHealthConfig{
		EWMAAlpha:            c.EWMAAlpha,
		HealthyRTTThreshold:  c.HealthyRTTThreshold,
		SuspectRTTThreshold:  c.SuspectRTTThreshold,
		DeadFailureThreshold: c.DeadFailureThreshold,
		ProbeInterval:        c.ProbeInterval,
		SuspectProbeInterval: c.SuspectProbeInterval,
		SwitchHysteresis:     c.SwitchHysteresis,
		StabilityWindow:      c.StabilityWindow,
		DirectPathBonus:      c.DirectPathBonus,
		MaxPathsPerPeer:      c.MaxPathsPerPeer,
		PathExpiry:           c.PathExpiry,
	}
}

// FromInterfaceConfig 从接口配置创建
func FromInterfaceConfig(cfg interfaces.PathHealthConfig) *Config {
	return &Config{
		EWMAAlpha:            cfg.EWMAAlpha,
		HealthyRTTThreshold:  cfg.HealthyRTTThreshold,
		SuspectRTTThreshold:  cfg.SuspectRTTThreshold,
		DeadFailureThreshold: cfg.DeadFailureThreshold,
		ProbeInterval:        cfg.ProbeInterval,
		SuspectProbeInterval: cfg.SuspectProbeInterval,
		SwitchHysteresis:     cfg.SwitchHysteresis,
		StabilityWindow:      cfg.StabilityWindow,
		DirectPathBonus:      cfg.DirectPathBonus,
		MaxPathsPerPeer:      cfg.MaxPathsPerPeer,
		PathExpiry:           cfg.PathExpiry,
	}
}
