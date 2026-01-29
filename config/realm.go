package config

import (
	"errors"
	"time"
)

// RealmConfig Realm 管理配置
//
// 配置 Realm 层的各个组件：
//   - Gateway: 网关管理
//   - Routing: 路由管理
//   - Member: 成员管理
//   - Auth: 认证管理
type RealmConfig struct {
	// EnableGateway 启用网关
	EnableGateway bool

	// EnableRouting 启用路由
	EnableRouting bool

	// EnableMember 启用成员管理
	EnableMember bool

	// EnableAuth 启用认证
	EnableAuth bool

	// Gateway 网关配置
	Gateway GatewayConfig

	// Routing 路由配置
	Routing RoutingConfig

	// Member 成员配置
	Member MemberConfig

	// Auth 认证配置
	Auth AuthConfig
}

// GatewayConfig 网关配置
type GatewayConfig struct {
	// MaxPeers 最大对等节点数
	MaxPeers int

	// DiscoveryInterval 发现间隔
	DiscoveryInterval time.Duration

	// ConnectionTimeout 连接超时
	ConnectionTimeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int
}

// RoutingConfig 路由配置
type RoutingConfig struct {
	// RefreshInterval 路由表刷新间隔
	RefreshInterval time.Duration

	// MaxRoutes 最大路由条目数
	MaxRoutes int

	// DefaultTTL 默认路由 TTL
	DefaultTTL time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration
}

// MemberConfig 成员配置
type MemberConfig struct {
	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration

	// HeartbeatTimeout 心跳超时
	HeartbeatTimeout time.Duration

	// MaxMembers 最大成员数
	MaxMembers int

	// SyncInterval 状态同步间隔
	SyncInterval time.Duration
}

// AuthConfig 认证配置
type AuthConfig struct {
	// EnablePSK 启用 PSK 认证
	EnablePSK bool

	// EnableChallenge 启用挑战认证
	EnableChallenge bool

	// TokenTTL 认证令牌 TTL
	TokenTTL time.Duration

	// MaxRetries 最大认证重试次数
	MaxRetries int

	// Timeout 认证超时
	Timeout time.Duration

	// ReplayWindow 重放攻击检测时间窗口
	ReplayWindow time.Duration
}

// DefaultRealmConfig 返回默认 Realm 配置
func DefaultRealmConfig() RealmConfig {
	return RealmConfig{
		// ════════════════════════════════════════════════════════════════════
		// Realm 组件启用配置
		// 注意：Realm 功能默认禁用，因为 Realm 是按需创建的
		// 这些组件在创建 Realm 时由 Realm 工厂内部管理
		// ════════════════════════════════════════════════════════════════════
		EnableGateway: false, // 禁用 Gateway：需要时通过 Realm 创建
		EnableRouting: false, // 禁用 Routing：需要时通过 Realm 创建
		EnableMember:  false, // 禁用 Member：需要时通过 Realm 创建
		EnableAuth:    false, // 禁用 Auth：需要时通过 Realm 创建

		// ════════════════════════════════════════════════════════════════════
		// Gateway 配置（跨 Realm 网关）
		// ════════════════════════════════════════════════════════════════════
		Gateway: GatewayConfig{
			MaxPeers:          100,              // 最大对等节点：100 个网关节点
			DiscoveryInterval: 30 * time.Second, // 发现间隔：30 秒
			ConnectionTimeout: 30 * time.Second, // 连接超时：30 秒
			MaxRetries:        3,                // 最大重试：3 次
		},

		// ════════════════════════════════════════════════════════════════════
		// Routing 配置（Realm 内部路由表）
		// ════════════════════════════════════════════════════════════════════
		Routing: RoutingConfig{
			RefreshInterval: 5 * time.Minute,   // 刷新间隔：5 分钟
			MaxRoutes:       10000,             // 最大路由条目：10000 条
			DefaultTTL:      1 * time.Hour,     // 默认 TTL：1 小时
			CleanupInterval: 10 * time.Minute,  // 清理间隔：10 分钟
		},

		// ════════════════════════════════════════════════════════════════════
		// Member 配置（成员管理）
		// ════════════════════════════════════════════════════════════════════
		Member: MemberConfig{
			HeartbeatInterval: 30 * time.Second, // 心跳间隔：30 秒
			HeartbeatTimeout:  90 * time.Second, // 心跳超时：90 秒（3 倍间隔）
			MaxMembers:        1000,             // 最大成员数：1000 个节点
			SyncInterval:      1 * time.Minute,  // 状态同步间隔：1 分钟
		},

		// ════════════════════════════════════════════════════════════════════
		// Auth 配置（Realm 认证）
		// ════════════════════════════════════════════════════════════════════
		Auth: AuthConfig{
			EnablePSK:       true,             // 启用 PSK 认证：预共享密钥
			EnableChallenge: false,            // 禁用挑战认证：按需启用
			TokenTTL:        1 * time.Hour,    // 认证令牌 TTL：1 小时
			MaxRetries:      3,                // 最大重试：3 次
			Timeout:         30 * time.Second, // 认证超时：30 秒
			ReplayWindow:    5 * time.Minute,  // 重放检测窗口：5 分钟
		},
	}
}

// Validate 验证 Realm 配置
func (c RealmConfig) Validate() error {
	// 验证网关配置
	if c.EnableGateway {
		if c.Gateway.MaxPeers <= 0 {
			return errors.New("gateway max peers must be positive")
		}
		if c.Gateway.DiscoveryInterval <= 0 {
			return errors.New("gateway discovery interval must be positive")
		}
		if c.Gateway.ConnectionTimeout <= 0 {
			return errors.New("gateway connection timeout must be positive")
		}
		if c.Gateway.MaxRetries < 0 {
			return errors.New("gateway max retries must be non-negative")
		}
	}

	// 验证路由配置
	if c.EnableRouting {
		if c.Routing.RefreshInterval <= 0 {
			return errors.New("routing refresh interval must be positive")
		}
		if c.Routing.MaxRoutes <= 0 {
			return errors.New("routing max routes must be positive")
		}
		if c.Routing.DefaultTTL <= 0 {
			return errors.New("routing default TTL must be positive")
		}
		if c.Routing.CleanupInterval <= 0 {
			return errors.New("routing cleanup interval must be positive")
		}
	}

	// 验证成员配置
	if c.EnableMember {
		if c.Member.HeartbeatInterval <= 0 {
			return errors.New("member heartbeat interval must be positive")
		}
		if c.Member.HeartbeatTimeout <= 0 {
			return errors.New("member heartbeat timeout must be positive")
		}
		if c.Member.HeartbeatTimeout <= c.Member.HeartbeatInterval {
			return errors.New("member heartbeat timeout must be greater than interval")
		}
		if c.Member.MaxMembers <= 0 {
			return errors.New("member max members must be positive")
		}
		if c.Member.SyncInterval <= 0 {
			return errors.New("member sync interval must be positive")
		}
	}

	// 验证认证配置
	if c.EnableAuth {
		if !c.Auth.EnablePSK && !c.Auth.EnableChallenge {
			return errors.New("at least one auth method must be enabled")
		}
		if c.Auth.TokenTTL <= 0 {
			return errors.New("auth token TTL must be positive")
		}
		if c.Auth.MaxRetries < 0 {
			return errors.New("auth max retries must be non-negative")
		}
		if c.Auth.Timeout <= 0 {
			return errors.New("auth timeout must be positive")
		}
		if c.Auth.ReplayWindow <= 0 {
			return errors.New("auth replay window must be positive")
		}
	}

	return nil
}

// WithGateway 设置是否启用网关
func (c RealmConfig) WithGateway(enabled bool) RealmConfig {
	c.EnableGateway = enabled
	return c
}

// WithRouting 设置是否启用路由
func (c RealmConfig) WithRouting(enabled bool) RealmConfig {
	c.EnableRouting = enabled
	return c
}

// WithMember 设置是否启用成员管理
func (c RealmConfig) WithMember(enabled bool) RealmConfig {
	c.EnableMember = enabled
	return c
}

// WithAuth 设置是否启用认证
func (c RealmConfig) WithAuth(enabled bool) RealmConfig {
	c.EnableAuth = enabled
	return c
}
