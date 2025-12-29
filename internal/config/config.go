// Package config 提供 dep2p 配置管理层
//
// config 包负责：
// - 定义内部配置结构
// - 提供默认值
// - 配置校验
// - 配置转换（用户配置 → 内部配置）
package config

import (
	"time"
)

// Config 内部配置结构
//
// 这是详细的内部配置结构，用于组件初始化。
// 用户配置（pkg/dep2p.UserConfig）会被转换为此结构。
type Config struct {
	// LogFile 日志文件路径
	// 为空时输出到 stderr，非空时输出到指定文件
	LogFile string

	// ListenAddrs 监听地址
	ListenAddrs []string

	// RealmID 业务域标识
	// 同一 Realm 的节点互相发现，不同 Realm 的节点互不可见
	RealmID string

	// Identity 身份配置
	Identity IdentityConfig

	// Transport 传输配置
	Transport TransportConfig

	// Security 安全配置
	Security SecurityConfig

	// Muxer 多路复用配置
	Muxer MuxerConfig

	// NAT NAT 穿透配置
	NAT NATConfig

	// Discovery 发现服务配置
	Discovery DiscoveryConfig

	// Relay 中继配置
	Relay RelayConfig

	// ConnectionManager 连接管理配置
	ConnectionManager ConnectionManagerConfig

	// Protocol 协议管理配置
	Protocol ProtocolConfig

	// Messaging 消息服务配置
	Messaging MessagingConfig

	// Realm 领域配置
	Realm RealmConfig

	// Liveness 存活检测配置
	Liveness LivenessConfig

	// Introspect 自省服务配置
	Introspect IntrospectConfig
}

// NewConfig 创建默认配置
func NewConfig() *Config {
	return &Config{
		ListenAddrs:       DefaultListenAddrs(),
		Identity:          DefaultIdentityConfig(),
		Transport:         DefaultTransportConfig(),
		Security:          DefaultSecurityConfig(),
		Muxer:             DefaultMuxerConfig(),
		NAT:               DefaultNATConfig(),
		Discovery:         DefaultDiscoveryConfig(),
		Relay:             DefaultRelayConfig(),
		ConnectionManager: DefaultConnectionManagerConfig(),
		Protocol:          DefaultProtocolConfig(),
		Messaging:         DefaultMessagingConfig(),
		Realm:             DefaultRealmConfig(),
		Liveness:          DefaultLivenessConfig(),
		Introspect:        DefaultIntrospectConfig(),
	}
}

// ============================================================================
//                              身份配置
// ============================================================================

// IdentityConfig 身份配置
type IdentityConfig struct {
	// KeyFile 密钥文件路径
	// 如果为空，将自动生成新密钥
	KeyFile string

	// KeyType 密钥类型
	// 可选: ed25519 (默认)
	KeyType string

	// PrivateKey 直接注入的私钥（可选）
	//
	// 如果提供，将跳过文件加载/生成，直接使用此私钥创建身份。
	// 用于 WithIdentity(key) 场景。
	// 类型为 any 以避免循环依赖，实际类型为 identityif.PrivateKey。
	PrivateKey any
}

// DefaultIdentityConfig 默认身份配置
func DefaultIdentityConfig() IdentityConfig {
	return IdentityConfig{
		KeyFile:    "",
		KeyType:    "ed25519",
		PrivateKey: nil,
	}
}

// ============================================================================
//                              传输配置
// ============================================================================

// TransportConfig 传输配置
type TransportConfig struct {
	// QUIC QUIC 传输配置
	QUIC QUICConfig

	// MaxConnections 最大连接数
	MaxConnections int

	// MaxStreamsPerConn 每连接最大流数
	MaxStreamsPerConn int

	// DialTimeout 拨号超时
	DialTimeout time.Duration

	// HandshakeTimeout 握手超时
	HandshakeTimeout time.Duration

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
}

// QUICConfig QUIC 配置
type QUICConfig struct {
	// MaxIdleTimeout 最大空闲超时
	MaxIdleTimeout time.Duration

	// MaxIncomingStreams 最大入站流数
	MaxIncomingStreams int64

	// MaxIncomingUniStreams 最大入站单向流数
	MaxIncomingUniStreams int64

	// KeepAlivePeriod 保活周期
	KeepAlivePeriod time.Duration

	// EnableDatagrams 启用数据报
	EnableDatagrams bool
}

// DefaultTransportConfig 默认传输配置
func DefaultTransportConfig() TransportConfig {
	return TransportConfig{
		QUIC: QUICConfig{
			MaxIdleTimeout:        30 * time.Second,
			MaxIncomingStreams:    256,
			MaxIncomingUniStreams: 256,
			KeepAlivePeriod:       15 * time.Second,
			EnableDatagrams:       true,
		},
		MaxConnections:    100,
		MaxStreamsPerConn: 256,
		DialTimeout:       10 * time.Second,
		HandshakeTimeout:  10 * time.Second,
		IdleTimeout:       5 * time.Minute,
	}
}

// ============================================================================
//                              安全配置
// ============================================================================

// SecurityConfig 安全配置
type SecurityConfig struct {
	// Protocol 安全协议
	// 可选: tls (默认), noise
	Protocol string

	// TLS TLS 配置
	TLS TLSConfig
}

// TLSConfig TLS 配置
type TLSConfig struct {
	// MinVersion 最小 TLS 版本
	MinVersion string

	// CipherSuites 加密套件
	CipherSuites []string
}

// DefaultSecurityConfig 默认安全配置
func DefaultSecurityConfig() SecurityConfig {
	return SecurityConfig{
		Protocol: "tls",
		TLS: TLSConfig{
			MinVersion:   "1.3",
			CipherSuites: nil, // 使用默认
		},
	}
}

// ============================================================================
//                              多路复用配置
// ============================================================================

// MuxerConfig 多路复用配置
type MuxerConfig struct {
	// Protocol 多路复用协议
	// 可选: yamux (默认), quic (QUIC 原生)
	Protocol string

	// StreamReceiveWindow 流接收窗口大小
	StreamReceiveWindow uint32

	// ConnectionReceiveWindow 连接接收窗口大小
	ConnectionReceiveWindow uint32
}

// DefaultMuxerConfig 默认多路复用配置
func DefaultMuxerConfig() MuxerConfig {
	return MuxerConfig{
		Protocol:                "yamux",
		StreamReceiveWindow:     256 * 1024,      // 256 KB
		ConnectionReceiveWindow: 16 * 1024 * 1024, // 16 MB
	}
}

// ============================================================================
//                              NAT 配置
// ============================================================================

// NATConfig NAT 穿透配置
type NATConfig struct {
	// Enable 启用 NAT 穿透
	Enable bool

	// EnableUPnP 启用 UPnP
	EnableUPnP bool

	// EnableAutoNAT 启用自动 NAT 检测
	EnableAutoNAT bool

	// EnableHolePunching 启用打洞
	EnableHolePunching bool

	// STUNServers STUN 服务器列表
	STUNServers []string

	// RefreshInterval 映射刷新间隔
	RefreshInterval time.Duration

	// ExternalAddrs 用户显式声明的公网地址（作为候选地址）
	//
	// 用于公网服务器场景：当节点有独立公网IP但无法通过UPnP/STUN自动发现时，
	// 用户可以显式声明其公网地址。这些地址将作为 Candidate 输入到 dial-back
	// 验证流程，验证成功后才会成为 VerifiedDirect 并发布到 DHT。
	//
	// 格式：multiaddr 或简化格式（如 "1.2.3.4:4001"）
	// 示例：["/ip4/203.0.113.5/udp/4001/quic-v1", "198.51.100.1:4001"]
	ExternalAddrs []string
}

// DefaultNATConfig 默认 NAT 配置
func DefaultNATConfig() NATConfig {
	return NATConfig{
		Enable:             true,
		EnableUPnP:         true,
		EnableAutoNAT:      true,
		EnableHolePunching: true,
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		RefreshInterval: 30 * time.Second,
	}
}

// ============================================================================
//                              发现配置
// ============================================================================

// DiscoveryConfig 发现服务配置
type DiscoveryConfig struct {
	// BootstrapPeers 引导节点列表
	BootstrapPeers []string

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// DHT DHT 配置
	DHT DHTConfig

	// MDNS mDNS 配置
	MDNS MDNSConfig
}

// DHTConfig DHT 配置
type DHTConfig struct {
	// Mode DHT 模式
	// 可选: auto, server, client
	Mode string

	// BucketSize K-桶大小
	BucketSize int

	// Concurrency 并发度
	Concurrency int
}

// MDNSConfig mDNS 配置
type MDNSConfig struct {
	// ServiceTag 服务标签
	ServiceTag string

	// Interval 发现间隔
	Interval time.Duration
}

// DefaultDiscoveryConfig 默认发现配置
func DefaultDiscoveryConfig() DiscoveryConfig {
	return DiscoveryConfig{
		BootstrapPeers:  []string{},
		RefreshInterval: 3 * time.Minute,
		DHT: DHTConfig{
			Mode:        "auto",
			BucketSize:  20,
			Concurrency: 3,
		},
		MDNS: MDNSConfig{
			ServiceTag: "_dep2p._udp",
			Interval:   10 * time.Second,
		},
	}
}

// ============================================================================
//                              中继配置
// ============================================================================

// RelayConfig 中继配置
type RelayConfig struct {
	// Enable 启用中继客户端
	Enable bool

	// EnableServer 启用中继服务器
	EnableServer bool

	// MaxReservations 最大预留数（服务器）
	MaxReservations int

	// MaxCircuits 最大电路数（服务器）
	MaxCircuits int

	// MaxCircuitsPerPeer 每节点最大电路数（服务器）
	MaxCircuitsPerPeer int

	// ReservationTTL 预留有效期
	ReservationTTL time.Duration
}

// DefaultRelayConfig 默认中继配置
func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		Enable:             true,
		EnableServer:       false,
		MaxReservations:    128,
		MaxCircuits:        16,
		MaxCircuitsPerPeer: 4,
		ReservationTTL:     1 * time.Hour,
	}
}

// ============================================================================
//                              连接管理配置
// ============================================================================

// ConnectionManagerConfig 连接管理配置
type ConnectionManagerConfig struct {
	// LowWater 低水位线
	LowWater int

	// HighWater 高水位线
	HighWater int

	// EmergencyWater 紧急水位线
	EmergencyWater int

	// GracePeriod 新连接保护期
	GracePeriod time.Duration

	// IdleTimeout 空闲超时
	IdleTimeout time.Duration
}

// DefaultConnectionManagerConfig 默认连接管理配置
func DefaultConnectionManagerConfig() ConnectionManagerConfig {
	return ConnectionManagerConfig{
		LowWater:       50,
		HighWater:      100,
		EmergencyWater: 150,
		GracePeriod:    30 * time.Second,
		IdleTimeout:    5 * time.Minute,
	}
}

// ============================================================================
//                              协议配置
// ============================================================================

// ProtocolConfig 协议管理配置
type ProtocolConfig struct {
	// NegotiationTimeout 协商超时
	NegotiationTimeout time.Duration
}

// DefaultProtocolConfig 默认协议配置
func DefaultProtocolConfig() ProtocolConfig {
	return ProtocolConfig{
		NegotiationTimeout: 10 * time.Second,
	}
}

// ============================================================================
//                              消息服务配置
// ============================================================================

// MessagingConfig 消息服务配置
type MessagingConfig struct {
	// RequestTimeout 请求超时
	RequestTimeout time.Duration

	// MaxMessageSize 最大消息大小
	MaxMessageSize int

	// PubSub 发布订阅配置
	PubSub PubSubConfig
}

// PubSubConfig 发布订阅配置
type PubSubConfig struct {
	// Enable 启用发布订阅
	Enable bool

	// MessageCacheSize 消息缓存大小（用于去重）
	MessageCacheSize int

	// MessageCacheTTL 消息缓存 TTL
	MessageCacheTTL time.Duration
}

// DefaultMessagingConfig 默认消息服务配置
func DefaultMessagingConfig() MessagingConfig {
	return MessagingConfig{
		RequestTimeout: 30 * time.Second,
		MaxMessageSize: 4 * 1024 * 1024, // 4 MB
		PubSub: PubSubConfig{
			Enable:           true,
			MessageCacheSize: 1000,
			MessageCacheTTL:  2 * time.Minute,
		},
	}
}

// ============================================================================
//                              默认监听地址
// ============================================================================

// DefaultListenAddrs 默认监听地址
func DefaultListenAddrs() []string {
	return []string{
		"/ip4/0.0.0.0/udp/0/quic-v1",
		"/ip6/::/udp/0/quic-v1",
	}
}

// ============================================================================
//                              Realm 配置
// ============================================================================

// RealmConfig Realm 领域配置
//
// IMPL-1227 变更:
//   - 移除 DefaultRealmID（用户必须显式提供 realmKey）
//   - 移除 AutoJoin（用户必须显式调用 JoinRealm）
//   - 新增 RealmAuthEnabled 和 RealmAuthTimeout
type RealmConfig struct {
	// RealmAuthEnabled 启用 RealmAuth 协议 (v1.1 新增)
	// 启用后，非系统协议流需要先通过 RealmAuth 验证
	RealmAuthEnabled bool

	// RealmAuthTimeout RealmAuth 超时时间 (v1.1 新增)
	RealmAuthTimeout time.Duration

	// IsolateDiscovery 隔离节点发现（只发现同 Realm 节点）
	IsolateDiscovery bool

	// IsolatePubSub 隔离 Pub-Sub（只订阅同 Realm 主题）
	IsolatePubSub bool
}

// DefaultRealmConfig 默认 Realm 配置
//
// IMPL-1227 变更:
//   - 移除 DefaultRealmID 和 AutoJoin
//   - RealmAuthEnabled 默认为 true
//   - RealmAuthTimeout 默认为 10 秒
func DefaultRealmConfig() RealmConfig {
	return RealmConfig{
		RealmAuthEnabled: true, // 默认启用 RealmAuth
		RealmAuthTimeout: 10 * time.Second,
		IsolateDiscovery: true,
		IsolatePubSub:    true,
	}
}

// ============================================================================
//                              Liveness 配置
// ============================================================================

// LivenessConfig 存活检测配置
type LivenessConfig struct {
	// Enable 启用存活检测
	Enable bool

	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration

	// HeartbeatTimeout 心跳超时（超过此时间无响应判定为离线）
	HeartbeatTimeout time.Duration

	// DegradedRTTThreshold RTT 超过此值判定为降级
	DegradedRTTThreshold time.Duration

	// StatusExpiry 状态过期时间
	StatusExpiry time.Duration

	// EnableGoodbye 启用 Goodbye 协议
	EnableGoodbye bool

	// HealthScore 健康评分配置
	HealthScore HealthScoreConfig

	// MaxConcurrentPings 最大并发 Ping 数
	MaxConcurrentPings int
}

// HealthScoreConfig 健康评分配置
type HealthScoreConfig struct {
	// DecayInterval 衰减间隔
	DecayInterval time.Duration

	// DecayAmount 每次衰减量
	DecayAmount int

	// MinScore 最低分数
	MinScore int

	// RecoveryOnPing Ping 成功时恢复的分数
	RecoveryOnPing int

	// RecoveryOnData 数据传输成功时恢复的分数
	RecoveryOnData int
}

// DefaultLivenessConfig 默认存活检测配置
func DefaultLivenessConfig() LivenessConfig {
	return LivenessConfig{
		Enable:               true,
		HeartbeatInterval:    15 * time.Second,
		HeartbeatTimeout:     45 * time.Second, // 3 次心跳
		DegradedRTTThreshold: 500 * time.Millisecond,
		StatusExpiry:         5 * time.Minute,
		EnableGoodbye:        true,
		HealthScore:          DefaultHealthScoreConfig(),
		MaxConcurrentPings:   10,
	}
}

// DefaultHealthScoreConfig 默认健康评分配置
func DefaultHealthScoreConfig() HealthScoreConfig {
	return HealthScoreConfig{
		DecayInterval:  1 * time.Minute,
		DecayAmount:    5,
		MinScore:       0,
		RecoveryOnPing: 10,
		RecoveryOnData: 5,
	}
}

// ============================================================================
//                              自省服务配置
// ============================================================================

// IntrospectConfig 自省服务配置
type IntrospectConfig struct {
	// Enable 启用自省 HTTP 服务
	Enable bool

	// Addr 监听地址，默认 "127.0.0.1:6060"
	Addr string
}

// DefaultIntrospectConfig 默认自省服务配置
func DefaultIntrospectConfig() IntrospectConfig {
	return IntrospectConfig{
		Enable: false,
		Addr:   "127.0.0.1:6060",
	}
}

