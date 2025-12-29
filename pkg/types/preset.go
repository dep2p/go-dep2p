package types

import "time"

// ============================================================================
//                              PresetConfig - 预设配置
// ============================================================================

// PresetConfig 预设配置详情
type PresetConfig struct {
	// Name 预设名称
	Name string

	// Description 描述
	Description string

	// ConnectionLimits 连接限制
	ConnectionLimits ConnectionLimits

	// DiscoveryConfig 发现配置
	DiscoveryConfig DiscoveryConfig

	// TransportConfig 传输配置
	TransportConfig TransportConfig

	// SecurityConfig 安全配置
	SecurityConfig SecurityConfig
}

// ============================================================================
//                              ConnectionLimits - 连接限制
// ============================================================================

// ConnectionLimits 连接限制配置
type ConnectionLimits struct {
	// LowWater 低水位线（停止裁剪）
	LowWater int

	// HighWater 高水位线（触发裁剪）
	HighWater int

	// EmergencyWater 紧急水位线（拒绝新连接）
	EmergencyWater int

	// GracePeriod 新连接保护期
	GracePeriod time.Duration

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
}

// ============================================================================
//                              DiscoveryConfig - 发现配置
// ============================================================================

// DiscoveryConfig 发现配置
type DiscoveryConfig struct {
	// EnableDHT 启用 DHT
	EnableDHT bool

	// EnableMDNS 启用 mDNS
	EnableMDNS bool

	// EnableBootstrap 启用引导
	EnableBootstrap bool

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration
}

// ============================================================================
//                              TransportConfig - 传输配置
// ============================================================================

// TransportConfig 传输配置
type TransportConfig struct {
	// MaxConnections 最大连接数
	MaxConnections int

	// MaxStreamsPerConn 每连接最大流数
	MaxStreamsPerConn int

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
}

// ============================================================================
//                              SecurityConfig - 安全配置
// ============================================================================

// SecurityConfig 安全配置
type SecurityConfig struct {
	// Protocol 安全协议（"tls" 或 "noise"）
	Protocol string

	// RequireClientAuth 是否要求客户端认证
	RequireClientAuth bool
}

