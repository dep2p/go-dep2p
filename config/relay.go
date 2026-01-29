package config

import (
	"errors"
	"time"
)

// RelayConfig 中继服务配置
//
// v2.0 统一 Relay 架构：
//   - 单一 Relay 服务，废弃双层中继（System/Realm）
//   - 客户端模式: 通过中继节点连接 NAT 后的节点
//   - 服务端模式: 为其他节点提供中继服务
//
// # TTL 协调策略
//
// Relay 地址簿 TTL 必须 >= DHT ProviderTTL，避免 DHT 地址指向过期的 Relay 预留。
// 当前默认值：
//   - DHT ProviderTTL: 24h（节点地址在 DHT 中的有效期）
//   - Relay AddressBook TTL: 24h（节点地址在 Relay 地址簿中的有效期）
//   - Relay ReservationTTL: 1h（Relay 预留有效期，需定期续租）
//
// 重要：ReservationTTL < AddressBook TTL，因为预留需要频繁续租，
// 而地址簿记录通过心跳机制保持最新。
type RelayConfig struct {
	// EnableClient 启用中继客户端
	EnableClient bool `json:"enable_client"`

	// EnableServer 启用中继服务端
	// 注意：服务端会消耗额外资源
	EnableServer bool `json:"enable_server"`

	// RelayAddr 要使用的 Relay 地址（客户端配置）
	// 格式：multiaddr，例如 "/ip4/relay.example.com/tcp/4001/p2p/QmRelay..."
	RelayAddr string `json:"relay_addr,omitempty"`

	// StaticRelays 静态中继列表（AutoRelay 使用）
	// 这些中继会被 AutoRelay 优先尝试使用
	// 格式：PeerID 列表
	StaticRelays []string `json:"static_relays,omitempty"`

	// Server 服务端配置
	Server RelayServerConfig `json:"server,omitempty"`

	// Client 客户端配置
	Client RelayClientConfig `json:"client,omitempty"`

	// Limits 统一中继限制
	Limits RelayLimitsConfig `json:"limits,omitempty"`
}

// RelayServerConfig 中继服务端配置
type RelayServerConfig struct {
	// MaxReservations 最大预约数
	MaxReservations int

	// MaxCircuits 最大活跃电路数
	// 0 表示不限制
	MaxCircuits int

	// ReservationTTL 预约有效期
	ReservationTTL time.Duration

	// BufferSize 中继缓冲区大小
	BufferSize int
}

// RelayClientConfig 中继客户端配置
type RelayClientConfig struct {
	// MaxRelays 最大中继节点数
	MaxRelays int

	// ReservationRefreshInterval 预约刷新间隔
	ReservationRefreshInterval time.Duration

	// ConnectTimeout 连接超时
	ConnectTimeout time.Duration
}

// RelayLimitsConfig 中继限制配置
type RelayLimitsConfig struct {
	// Bandwidth 带宽限制（字节/秒）
	// 0 表示不限制
	Bandwidth int64

	// Duration 最大持续时间
	// 0 表示不限制
	Duration time.Duration

	// MaxCircuitsPerPeer 每节点最大并发电路数
	// 0 表示不限制
	MaxCircuitsPerPeer int
}

// DefaultRelayConfig 返回默认中继配置
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		// ════════════════════════════════════════════════════════════════════
		// 中继模式启用配置
		// ════════════════════════════════════════════════════════════════════
		EnableClient: true,  // 启用客户端：允许通过中继节点连接 NAT 后的节点
		EnableServer: false, // 禁用服务端：默认不为他人提供中继（服务器预设会启用）

		// ════════════════════════════════════════════════════════════════════
		// 中继服务端配置（当 EnableServer=true 时生效）
		// ════════════════════════════════════════════════════════════════════
		Server: RelayServerConfig{
			MaxReservations: 128,            // 最大预约数：128 个节点可以预约此中继
			MaxCircuits:     16,             // 最大活跃电路：16 个并发中继连接
			ReservationTTL:  1 * time.Hour,  // 预约有效期：1 小时，过期需重新预约
			BufferSize:      2048,           // 中继缓冲区：2 KB
		},

		// ════════════════════════════════════════════════════════════════════
		// 中继客户端配置
		// ════════════════════════════════════════════════════════════════════
		Client: RelayClientConfig{
			MaxRelays:                  3,                 // 最大中继节点数：3 个，提供冗余
			ReservationRefreshInterval: 30 * time.Minute, // 预约刷新间隔：30 分钟
			ConnectTimeout:             30 * time.Second, // 中继连接超时：30 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// 统一中继限制（v2.0）
		// ════════════════════════════════════════════════════════════════════
		Limits: RelayLimitsConfig{
			Bandwidth:          0, // 带宽限制：0 表示不限制
			Duration:           0, // 持续时间：0 表示不限制
			MaxCircuitsPerPeer: 0, // 每节点最大并发：0 表示不限制
		},
	}
}

// Validate 验证中继配置
func (c RelayConfig) Validate() error {
	// 验证服务端配置
	if c.EnableServer {
		if c.Server.MaxReservations < 1 {
			return errors.New("relay max reservations must be at least 1")
		}
		if c.Server.MaxCircuits < 0 {
			return errors.New("relay max circuits must be non-negative")
		}
		if c.Server.ReservationTTL < time.Minute {
			return errors.New("relay reservation TTL must be at least 1 minute")
		}
		if c.Server.BufferSize < 1024 {
			return errors.New("relay buffer size must be at least 1024")
		}
	}

	// 验证客户端配置
	if c.EnableClient {
		if c.Client.MaxRelays < 1 {
			return errors.New("relay client max relays must be at least 1")
		}
		if c.Client.ReservationRefreshInterval <= 0 {
			return errors.New("relay client reservation refresh interval must be positive")
		}
		if c.Client.ConnectTimeout <= 0 {
			return errors.New("relay client connect timeout must be positive")
		}
	}

	// 验证限制配置
	if c.Limits.Bandwidth < 0 {
		return errors.New("relay bandwidth limit must be non-negative")
	}
	if c.Limits.Duration < 0 {
		return errors.New("relay duration limit must be non-negative")
	}
	if c.Limits.MaxCircuitsPerPeer < 0 {
		return errors.New("relay max circuits per peer must be non-negative")
	}

	return nil
}

// WithClient 设置是否启用中继客户端
func (c RelayConfig) WithClient(enabled bool) RelayConfig {
	c.EnableClient = enabled
	return c
}

// WithServer 设置是否启用中继服务端
func (c RelayConfig) WithServer(enabled bool) RelayConfig {
	c.EnableServer = enabled
	return c
}
