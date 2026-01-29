package config

import (
	"errors"
	"time"
)

// ConnManagerConfig 连接管理配置
//
// 配置连接管理策略：
//   - 连接数限制（高低水位）
//   - 连接优先级和评分
//   - 连接过滤和黑名单
type ConnManagerConfig struct {
	// LowWater 低水位（目标连接数）
	// 连接数低于此值时，主动发现并连接新节点
	LowWater int `json:"low_water"`

	// HighWater 高水位（触发回收）
	// 连接数超过此值时，开始清理低优先级连接
	HighWater int `json:"high_water"`

	// GracePeriod 新连接保护期
	// 新连接在此期间内不会被清理
	GracePeriod Duration `json:"grace_period"`

	// DecayInterval 评分衰减间隔
	// 定期对连接评分进行衰减
	DecayInterval Duration `json:"decay_interval,omitempty"`

	// Gater 连接过滤配置
	Gater ConnectionGaterConfig `json:"gater,omitempty"`
}

// ConnectionGaterConfig 连接过滤配置
type ConnectionGaterConfig struct {
	// EnableIPBlacklist 启用 IP 黑名单
	EnableIPBlacklist bool `json:"enable_ip_blacklist,omitempty"`

	// BlacklistedIPs IP 黑名单列表
	BlacklistedIPs []string `json:"blacklisted_ips,omitempty"`

	// EnablePortFilter 启用端口过滤
	EnablePortFilter bool `json:"enable_port_filter,omitempty"`

	// BlockedPorts 禁止的端口列表
	BlockedPorts []int `json:"blocked_ports,omitempty"`

	// AllowPrivateIPs 是否允许私有 IP
	AllowPrivateIPs bool `json:"allow_private_ips,omitempty"`

	// AllowLoopback 是否允许 loopback
	AllowLoopback bool `json:"allow_loopback,omitempty"`
}

// DefaultConnManagerConfig 返回默认连接管理配置
func DefaultConnManagerConfig() ConnManagerConfig {
	return ConnManagerConfig{
		// ════════════════════════════════════════════════════════════════════
		// 连接数限制（基于水位线的自动管理）
		// ════════════════════════════════════════════════════════════════════
		LowWater:  100, // 低水位：100 个连接，低于此值时主动发现新节点
		HighWater: 400, // 高水位：400 个连接，超过此值时清理低优先级连接

		// ════════════════════════════════════════════════════════════════════
		// 连接生命周期管理
		// ════════════════════════════════════════════════════════════════════
		GracePeriod:   Duration(20 * time.Second), // 新连接保护期：20 秒内不会被清理
		DecayInterval: Duration(1 * time.Minute),  // 评分衰减间隔：每分钟衰减一次连接分数

		// ════════════════════════════════════════════════════════════════════
		// 连接过滤配置（Connection Gater）
		// ════════════════════════════════════════════════════════════════════
		Gater: ConnectionGaterConfig{
			EnableIPBlacklist: false,        // IP 黑名单：默认禁用
			BlacklistedIPs:    []string{},   // 黑名单列表：默认空
			EnablePortFilter:  false,        // 端口过滤：默认禁用
			BlockedPorts:      []int{},      // 禁止端口列表：默认空
			AllowPrivateIPs:   true,         // 允许私有 IP：允许局域网连接
			AllowLoopback:     true,         // 允许 Loopback：允许本地测试
		},
	}
}

// Validate 验证连接管理配置
func (c ConnManagerConfig) Validate() error {
	if c.LowWater <= 0 {
		return errors.New("low water must be positive")
	}

	if c.HighWater <= c.LowWater {
		return errors.New("high water must be greater than low water")
	}

	if c.GracePeriod < 0 {
		return errors.New("grace period must be non-negative")
	}

	if c.DecayInterval < 0 {
		return errors.New("decay interval must be non-negative")
	}

	// 验证 Gater 配置
	if c.Gater.EnablePortFilter && len(c.Gater.BlockedPorts) == 0 {
		return errors.New("port filter enabled but no blocked ports specified")
	}

	return nil
}

// WithLowWater 设置低水位
func (c ConnManagerConfig) WithLowWater(low int) ConnManagerConfig {
	c.LowWater = low
	return c
}

// WithHighWater 设置高水位
func (c ConnManagerConfig) WithHighWater(high int) ConnManagerConfig {
	c.HighWater = high
	return c
}

// WithGracePeriod 设置保护期
func (c ConnManagerConfig) WithGracePeriod(period time.Duration) ConnManagerConfig {
	c.GracePeriod = Duration(period)
	return c
}

// WithIPBlacklist 设置 IP 黑名单
func (c ConnManagerConfig) WithIPBlacklist(enabled bool, ips []string) ConnManagerConfig {
	c.Gater.EnableIPBlacklist = enabled
	c.Gater.BlacklistedIPs = ips
	return c
}

// WithPortFilter 设置端口过滤
func (c ConnManagerConfig) WithPortFilter(enabled bool, ports []int) ConnManagerConfig {
	c.Gater.EnablePortFilter = enabled
	c.Gater.BlockedPorts = ports
	return c
}
