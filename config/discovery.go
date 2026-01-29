package config

import (
	"errors"
	"time"
)

// DiscoveryConfig 节点发现配置
//
// 配置节点发现机制：
//   - DHT: 分布式哈希表
//   - mDNS: 本地网络发现
//   - Bootstrap: 引导节点
//   - Rendezvous: 会合点发现
//   - DNS: DNS 解析发现
type DiscoveryConfig struct {
	// EnableDHT 是否启用 DHT
	EnableDHT bool `json:"enable_dht"`

	// EnableMDNS 是否启用 mDNS
	EnableMDNS bool `json:"enable_mdns"`

	// EnableBootstrap 是否启用 Bootstrap
	EnableBootstrap bool `json:"enable_bootstrap"`

	// EnableRendezvous 是否启用 Rendezvous
	EnableRendezvous bool `json:"enable_rendezvous"`

	// EnableDNS 是否启用 DNS
	EnableDNS bool `json:"enable_dns"`

	// DHT DHT 配置
	DHT DHTConfig `json:"dht,omitempty"`

	// MDNS mDNS 配置
	MDNS MDNSConfig `json:"mdns,omitempty"`

	// Bootstrap Bootstrap 配置
	Bootstrap BootstrapConfig `json:"bootstrap,omitempty"`

	// Rendezvous Rendezvous 配置
	Rendezvous RendezvousConfig `json:"rendezvous,omitempty"`

	// DNS DNS 配置
	DNS DNSConfig `json:"dns,omitempty"`
}

// DHTConfig DHT 配置
//
// # TTL 协调策略（v2.0）
//
// ProviderTTL 必须 <= Relay AddressBook TTL，避免 DHT 地址指向过期的 Relay 预留。
// 当前默认值：
//   - ProviderTTL: 24h（节点地址在 DHT 中的有效期）
//   - Relay AddressBook TTL: 24h（节点地址在 Relay 地址簿中的有效期）
//
// 修改 ProviderTTL 时，请确保 Relay 地址簿 TTL 同步调整。
type DHTConfig struct {
	// BucketSize K-桶大小
	BucketSize int `json:"bucket_size,omitempty"`

	// Alpha 并发查询参数
	Alpha int `json:"alpha,omitempty"`

	// QueryTimeout 查询超时
	QueryTimeout Duration `json:"query_timeout,omitempty"`

	// RefreshInterval 路由表刷新间隔
	RefreshInterval Duration `json:"refresh_interval,omitempty"`

	// ReplicationFactor 值复制因子
	ReplicationFactor int `json:"replication_factor,omitempty"`

	// EnableValueStore 启用值存储
	EnableValueStore bool `json:"enable_value_store,omitempty"`

	// MaxRecordAge 记录最大存活时间
	MaxRecordAge Duration `json:"max_record_age,omitempty"`

	// ProviderTTL Provider 记录 TTL
	// 重要：必须 <= Relay AddressBook TTL（默认 24h）
	ProviderTTL Duration `json:"provider_ttl,omitempty"`

	// PeerRecordTTL PeerRecord TTL
	PeerRecordTTL Duration `json:"peer_record_ttl,omitempty"`

	// CleanupInterval 清理间隔
	CleanupInterval Duration `json:"cleanup_interval,omitempty"`

	// RepublishInterval 重新发布间隔
	RepublishInterval Duration `json:"republish_interval,omitempty"`

	// ============= v2.0 新增：地址发布与验证配置 =============

	// AddressPublishStrategy 地址发布策略
	// - "auto": 自动根据可达性决定（默认，推荐）
	// - "all": 发布所有地址（直连 + 中继）
	// - "direct_only": 仅发布直连地址
	// - "relay_only": 仅发布中继地址（用于 Private 节点）
	AddressPublishStrategy string `json:"address_publish_strategy,omitempty"`

	// AllowPrivateAddrs 是否允许私网地址
	// 设置为 true 时，DHT 会接受私网地址（如 192.168.x.x、10.x.x.x 等）
	// 在局域网测试或私有网络部署时很有用
	// 默认: false（仅接受公网地址）
	AllowPrivateAddrs bool `json:"allow_private_addrs,omitempty"`

	// EnableReachabilityVerification 是否启用发布前可达性验证
	// 启用后，发布 PeerRecord 前会通过 ReachabilityChecker 验证地址
	// 只有经过验证的直连地址才会被发布
	// 默认: true
	EnableReachabilityVerification bool `json:"enable_reachability_verification,omitempty"`

	// EnableDynamicTTL 是否启用动态 TTL
	// 启用后，根据 NAT 类型动态调整 PeerRecord TTL
	// - 公网节点使用较长 TTL
	// - NAT 后节点使用较短 TTL
	// 默认: true
	EnableDynamicTTL bool `json:"enable_dynamic_ttl,omitempty"`

	// EnableAddressBookIntegration 是否启用 Relay 地址簿集成
	// 启用后，DHT 查询失败时会回退到 Relay 地址簿
	// DHT 查询成功时会更新 Relay 地址簿缓存
	// 默认: true
	EnableAddressBookIntegration bool `json:"enable_addressbook_integration,omitempty"`
}

// MDNSConfig mDNS 配置
type MDNSConfig struct {
	// Interval 发现间隔
	Interval Duration `json:"interval,omitempty"`

	// ServiceTag 服务标签
	ServiceTag string `json:"service_tag,omitempty"`

	// EnableIPv6 是否支持 IPv6
	EnableIPv6 bool `json:"enable_ipv6,omitempty"`
}

// BootstrapConfig Bootstrap 配置
type BootstrapConfig struct {
	// Peers 引导节点列表（multiaddr 格式）
	Peers []string `json:"peers"`

	// MinPeers 最小连接节点数
	// 低于此值时触发引导
	MinPeers int `json:"min_peers"`

	// Interval 引导间隔
	Interval Duration `json:"interval,omitempty"`

	// Timeout 连接超时
	Timeout Duration `json:"timeout"`

	// EnableService 启用 Bootstrap 服务能力
	// 启用后，节点将作为 Bootstrap 服务器，为其他节点提供初始发现服务
	// 前置条件：节点必须有公网可达地址
	EnableService bool `json:"enable_service"`
}

// RendezvousConfig Rendezvous 配置
type RendezvousConfig struct {
	// DefaultTTL 默认注册 TTL
	DefaultTTL Duration `json:"default_ttl,omitempty"`

	// QueryInterval 查询间隔
	QueryInterval Duration `json:"query_interval,omitempty"`

	// MaxPeers 每次查询最大返回节点数
	MaxPeers int `json:"max_peers,omitempty"`
}

// DNSConfig DNS 配置
type DNSConfig struct {
	// ResolverURL DNS 解析器 URL
	ResolverURL string `json:"resolver_url,omitempty"`

	// Timeout 解析超时
	Timeout Duration `json:"timeout,omitempty"`

	// CacheTTL 缓存 TTL
	CacheTTL Duration `json:"cache_ttl,omitempty"`
}

// DefaultDiscoveryConfig 返回默认发现配置
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		// ════════════════════════════════════════════════════════════════════
		// 发现机制启用配置
		// ════════════════════════════════════════════════════════════════════
		EnableDHT:        true,  // 启用 DHT：分布式哈希表，核心发现机制
		EnableMDNS:       true,  // 启用 mDNS：局域网自动发现（支持优雅降级）
		EnableBootstrap:  true,  // 启用 Bootstrap：引导节点连接
		EnableRendezvous: false, // 禁用 Rendezvous：按需启用的会合点发现
		EnableDNS:        false, // 禁用 DNS：按需启用的 DNS 发现

		// ════════════════════════════════════════════════════════════════════
		// DHT 配置（Kademlia 分布式哈希表）
		// ════════════════════════════════════════════════════════════════════
		DHT: DHTConfig{
			BucketSize:        20,                          // K-桶大小：20（Kademlia 标准值）
			Alpha:             3,                           // 并发查询数：3（Kademlia 标准值）
			QueryTimeout:      Duration(30 * time.Second),  // 单次查询超时：30 秒
			RefreshInterval:   Duration(1 * time.Hour),     // 路由表刷新间隔：1 小时
			ReplicationFactor: 3,                           // 值复制因子：3（存储到最近的 3 个节点）
			EnableValueStore:  true,                        // 启用值存储：支持 PUT/GET 操作
			MaxRecordAge:      Duration(24 * time.Hour),    // 记录最大存活时间：24 小时
			ProviderTTL:       Duration(24 * time.Hour),    // Provider 记录 TTL：24 小时
			PeerRecordTTL:     Duration(1 * time.Hour),     // PeerRecord TTL：1 小时
			CleanupInterval:   Duration(10 * time.Minute),  // 过期记录清理间隔：10 分钟
			RepublishInterval: Duration(20 * time.Minute),  // 记录重新发布间隔：20 分钟
			// v2.0 新增：地址发布与验证配置
			AddressPublishStrategy:         "auto", // 自动根据可达性决定发布策略
			AllowPrivateAddrs:              false,  // 默认不允许私网地址
			EnableReachabilityVerification: true,   // 默认启用可达性验证
			EnableDynamicTTL:               true,   // 默认启用动态 TTL
			EnableAddressBookIntegration:   true,   // 默认启用地址簿集成
		},

		// ════════════════════════════════════════════════════════════════════
		// mDNS 配置（多播 DNS，局域网发现）
		// ════════════════════════════════════════════════════════════════════
		MDNS: MDNSConfig{
			Interval:   Duration(10 * time.Second), // 广播间隔：10 秒
			ServiceTag: "_dep2p._udp",              // 服务标签：标识 DEP2P 节点
			EnableIPv6: true,                       // 启用 IPv6：支持 IPv6 局域网
		},

		// ════════════════════════════════════════════════════════════════════
		// Bootstrap 配置（引导节点）
		// ════════════════════════════════════════════════════════════════════
		Bootstrap: BootstrapConfig{
			// Peers 引导节点列表
			// 默认为空，用户需要自行配置或使用 WithBootstrapPeers() 选项
			//
			// 格式示例：
			//   "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW..."
			//   "/dnsaddr/bootstrap.example.com/p2p/12D3KooW..."
			//
			// 注意：如果不配置 Bootstrap 节点，节点将依赖 mDNS 进行局域网发现，
			// 或需要通过 /connect 命令手动连接其他节点。
			Peers:    []string{},
			MinPeers: 4,                          // 最小节点数：低于 4 个时触发引导
			Interval: Duration(5 * time.Minute),  // 引导间隔：每 5 分钟检查一次
			Timeout:  Duration(30 * time.Second), // 连接超时：30 秒
		},

		// ════════════════════════════════════════════════════════════════════
		// Rendezvous 配置（会合点发现）
		// ════════════════════════════════════════════════════════════════════
		Rendezvous: RendezvousConfig{
			DefaultTTL:    Duration(1 * time.Hour),   // 默认注册 TTL：1 小时
			QueryInterval: Duration(1 * time.Minute), // 查询间隔：1 分钟
			MaxPeers:      100,                       // 每次查询最大返回：100 个节点
		},

		// ════════════════════════════════════════════════════════════════════
		// DNS 配置（DNS 发现）
		// ════════════════════════════════════════════════════════════════════
		DNS: DNSConfig{
			ResolverURL: "https://cloudflare-dns.com/dns-query", // DoH 解析器
			Timeout:     Duration(10 * time.Second),             // 解析超时：10 秒
			CacheTTL:    Duration(5 * time.Minute),              // 缓存 TTL：5 分钟
		},
	}
}

// Validate 验证发现配置
func (c DiscoveryConfig) Validate() error {
	// 至少启用一种发现机制
	if !c.EnableDHT && !c.EnableMDNS && !c.EnableBootstrap && !c.EnableRendezvous && !c.EnableDNS {
		return errors.New("at least one discovery mechanism must be enabled")
	}

	// 验证 DHT 配置
	if c.EnableDHT {
		if c.DHT.BucketSize <= 0 {
			return errors.New("DHT bucket size must be positive")
		}
		if c.DHT.Alpha <= 0 {
			return errors.New("DHT alpha must be positive")
		}
		if c.DHT.QueryTimeout <= 0 {
			return errors.New("DHT query timeout must be positive")
		}
		if c.DHT.RefreshInterval <= 0 {
			return errors.New("DHT refresh interval must be positive")
		}
		if c.DHT.ReplicationFactor <= 0 {
			return errors.New("DHT replication factor must be positive")
		}
		if c.DHT.ProviderTTL <= 0 {
			return errors.New("DHT provider TTL must be positive")
		}
		if c.DHT.PeerRecordTTL <= 0 {
			return errors.New("DHT peer record TTL must be positive")
		}
		// Relay 地址簿 TTL 默认 24h，DHT ProviderTTL 不能超过它
		if c.DHT.ProviderTTL > Duration(24*time.Hour) {
			return errors.New("DHT provider TTL must be <= relay addressbook TTL")
		}
		if c.DHT.PeerRecordTTL > c.DHT.ProviderTTL {
			return errors.New("DHT peer record TTL must be <= provider TTL")
		}
		if c.DHT.RepublishInterval > c.DHT.PeerRecordTTL {
			return errors.New("DHT republish interval must be <= peer record TTL")
		}
	}

	// 验证 mDNS 配置
	if c.EnableMDNS {
		if c.MDNS.Interval <= 0 {
			return errors.New("mDNS interval must be positive")
		}
		if c.MDNS.ServiceTag == "" {
			return errors.New("mDNS service tag must not be empty")
		}
	}

	// 验证 Bootstrap 配置
	if c.EnableBootstrap {
		if c.Bootstrap.MinPeers < 0 {
			return errors.New("bootstrap min peers must be non-negative")
		}
		if c.Bootstrap.Interval <= 0 {
			return errors.New("bootstrap interval must be positive")
		}
		if c.Bootstrap.Timeout <= 0 {
			return errors.New("bootstrap timeout must be positive")
		}
	}

	// 验证 Rendezvous 配置
	if c.EnableRendezvous {
		if c.Rendezvous.DefaultTTL <= 0 {
			return errors.New("rendezvous default TTL must be positive")
		}
		if c.Rendezvous.QueryInterval <= 0 {
			return errors.New("rendezvous query interval must be positive")
		}
		if c.Rendezvous.MaxPeers <= 0 {
			return errors.New("rendezvous max peers must be positive")
		}
	}

	// 验证 DNS 配置
	if c.EnableDNS {
		if c.DNS.ResolverURL == "" {
			return errors.New("DNS resolver URL must not be empty")
		}
		if c.DNS.Timeout <= 0 {
			return errors.New("DNS timeout must be positive")
		}
		if c.DNS.CacheTTL <= 0 {
			return errors.New("DNS cache TTL must be positive")
		}
	}

	return nil
}

// WithDHT 设置是否启用 DHT
func (c DiscoveryConfig) WithDHT(enabled bool) DiscoveryConfig {
	c.EnableDHT = enabled
	return c
}

// WithMDNS 设置是否启用 mDNS
func (c DiscoveryConfig) WithMDNS(enabled bool) DiscoveryConfig {
	c.EnableMDNS = enabled
	return c
}

// WithBootstrap 设置是否启用 Bootstrap
func (c DiscoveryConfig) WithBootstrap(enabled bool) DiscoveryConfig {
	c.EnableBootstrap = enabled
	return c
}

// WithBootstrapPeers 设置引导节点列表
func (c DiscoveryConfig) WithBootstrapPeers(peers []string) DiscoveryConfig {
	c.Bootstrap.Peers = peers
	return c
}
