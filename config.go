// Package dep2p 提供去中心化点对点网络库
//
// dep2p 是一个模块化的 P2P 网络库，支持：
//   - 多种传输协议（QUIC、TCP、WebSocket）
//   - 多种安全传输（TLS 1.3、Noise）
//   - NAT 穿透（AutoNAT、UPnP、Hole Punching）
//   - 节点发现（DHT、mDNS、Bootstrap）
//   - 中继服务（Circuit Relay v2）
//   - Realm 管理（私有网络）
//
// 快速开始：
//
//	// 使用默认配置创建节点
//	node, err := dep2p.New(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer node.Close()
//
//	// 使用预设创建节点
//	node, err := dep2p.New(ctx, dep2p.WithPreset("server"))
//
//	// 使用自定义配置
//	node, err := dep2p.New(ctx,
//	    dep2p.WithQUIC(true),
//	    dep2p.WithDHT(true),
//	    dep2p.WithConnectionLimits(100, 400),
//	)
//
// 配置管理：
//
// dep2p 使用统一的配置系统，所有配置都在 config 包中定义：
//
//	import "github.com/dep2p/go-dep2p/config"
//
//	// 创建配置
//	cfg := config.NewConfig()
//
//	// 修改配置
//	cfg.Transport.EnableQUIC = true
//	cfg.NAT.EnableAutoNAT = true
//
//	// 验证配置
//	if err := cfg.Validate(); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 使用配置创建节点
//	node, err := dep2p.New(ctx, dep2p.WithConfig(cfg))
//
// 预设配置：
//
// dep2p 提供了针对不同场景优化的预设配置：
//
//	// 移动端配置
//	cfg := dep2p.GetMobileConfig()
//
//	// 桌面端配置（默认）
//	cfg := dep2p.GetDesktopConfig()
//
//	// 服务器配置
//	cfg := dep2p.GetServerConfig()
//
//	// 最小配置（测试用）
//	cfg := dep2p.GetMinimalConfig()
//
// 更多信息请参阅：
//   - config 包文档：配置系统详细说明
//   - examples 目录：完整示例代码
//   - design 目录：设计文档
package dep2p

import (
	"github.com/dep2p/go-dep2p/config"
)

// ════════════════════════════════════════════════════════════════════════════
//                              配置类型别名
// ════════════════════════════════════════════════════════════════════════════

// Config 是 config.Config 的类型别名
//
// 为了方便使用，可以直接使用 dep2p.Config 而不是 config.Config。
type Config = config.Config

// IdentityConfig 是 config.IdentityConfig 的类型别名
type IdentityConfig = config.IdentityConfig

// TransportConfig 是 config.TransportConfig 的类型别名
type TransportConfig = config.TransportConfig

// SecurityConfig 是 config.SecurityConfig 的类型别名
type SecurityConfig = config.SecurityConfig

// NATConfig 是 config.NATConfig 的类型别名
type NATConfig = config.NATConfig

// RelayConfig 是 config.RelayConfig 的类型别名
type RelayConfig = config.RelayConfig

// DiscoveryConfig 是 config.DiscoveryConfig 的类型别名
type DiscoveryConfig = config.DiscoveryConfig

// ConnManagerConfig 是 config.ConnManagerConfig 的类型别名
type ConnManagerConfig = config.ConnManagerConfig

// MessagingConfig 是 config.MessagingConfig 的类型别名
type MessagingConfig = config.MessagingConfig

// RealmConfig 是 config.RealmConfig 的类型别名
type RealmConfig = config.RealmConfig

// ResourceConfig 是 config.ResourceConfig 的类型别名
type ResourceConfig = config.ResourceConfig

// ════════════════════════════════════════════════════════════════════════════
//                              配置创建函数
// ════════════════════════════════════════════════════════════════════════════

// NewConfig 创建默认配置
//
// 这是 config.NewConfig() 的便捷包装。
//
// 示例：
//
//	cfg := dep2p.NewConfig()
//	cfg.Transport.EnableQUIC = true
func NewConfig() *Config {
	return config.NewConfig()
}

// ════════════════════════════════════════════════════════════════════════════
//                              子配置默认值
// ════════════════════════════════════════════════════════════════════════════

// DefaultIdentityConfig 返回默认身份配置
func DefaultIdentityConfig() IdentityConfig {
	return config.DefaultIdentityConfig()
}

// DefaultTransportConfig 返回默认传输配置
func DefaultTransportConfig() TransportConfig {
	return config.DefaultTransportConfig()
}

// DefaultSecurityConfig 返回默认安全配置
func DefaultSecurityConfig() SecurityConfig {
	return config.DefaultSecurityConfig()
}

// DefaultNATConfig 返回默认 NAT 配置
func DefaultNATConfig() NATConfig {
	return config.DefaultNATConfig()
}

// DefaultRelayConfig 返回默认中继配置
func DefaultRelayConfig() RelayConfig {
	return config.DefaultRelayConfig()
}

// DefaultDiscoveryConfig 返回默认发现配置
func DefaultDiscoveryConfig() DiscoveryConfig {
	return config.DefaultDiscoveryConfig()
}

// DefaultConnManagerConfig 返回默认连接管理配置
func DefaultConnManagerConfig() ConnManagerConfig {
	return config.DefaultConnManagerConfig()
}

// DefaultMessagingConfig 返回默认消息传递配置
func DefaultMessagingConfig() MessagingConfig {
	return config.DefaultMessagingConfig()
}

// DefaultRealmConfig 返回默认 Realm 配置
func DefaultRealmConfig() RealmConfig {
	return config.DefaultRealmConfig()
}

// DefaultResourceConfig 返回默认资源配置
func DefaultResourceConfig() ResourceConfig {
	return config.DefaultResourceConfig()
}

// ════════════════════════════════════════════════════════════════════════════
//                              配置验证
// ════════════════════════════════════════════════════════════════════════════

// ValidateConfig 验证配置
//
// 这是 config.ValidateAll() 的便捷包装。
//
// 示例：
//
//	if err := dep2p.ValidateConfig(cfg); err != nil {
//	    log.Fatal(err)
//	}
func ValidateConfig(cfg *Config) error {
	return config.ValidateAll(cfg)
}

// ValidateAndFixConfig 验证配置并尝试自动修复
//
// 这是 config.ValidateAndFix() 的便捷包装。
//
// 示例：
//
//	cfg, err := dep2p.ValidateAndFixConfig(cfg)
//	if err != nil {
//	    log.Fatal(err)
//	}
func ValidateAndFixConfig(cfg *Config) (*Config, error) {
	return config.ValidateAndFix(cfg)
}
