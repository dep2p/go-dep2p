package config

import "time"

// ============================================================================
//                              预设默认值
// ============================================================================

// 连接限制默认值
const (
	// DefaultLowWater 默认低水位线
	DefaultLowWater = 50

	// DefaultHighWater 默认高水位线
	DefaultHighWater = 100

	// DefaultEmergencyWater 默认紧急水位线
	DefaultEmergencyWater = 150

	// DefaultGracePeriod 默认新连接保护期
	DefaultGracePeriod = 30 * time.Second

	// DefaultIdleTimeout 默认空闲超时
	DefaultIdleTimeout = 5 * time.Minute
)

// 传输默认值
const (
	// DefaultMaxConnections 默认最大连接数
	DefaultMaxConnections = 100

	// DefaultMaxStreamsPerConn 默认每连接最大流数
	DefaultMaxStreamsPerConn = 256

	// DefaultDialTimeout 默认拨号超时
	DefaultDialTimeout = 10 * time.Second

	// DefaultHandshakeTimeout 默认握手超时
	DefaultHandshakeTimeout = 10 * time.Second
)

// QUIC 默认值
const (
	// DefaultQUICMaxIdleTimeout 默认 QUIC 最大空闲超时
	DefaultQUICMaxIdleTimeout = 30 * time.Second

	// DefaultQUICMaxIncomingStreams 默认 QUIC 最大入站流数
	DefaultQUICMaxIncomingStreams = 256

	// DefaultQUICKeepAlivePeriod 默认 QUIC 保活周期
	DefaultQUICKeepAlivePeriod = 15 * time.Second
)

// 发现默认值
const (
	// DefaultDiscoveryRefreshInterval 默认发现刷新间隔
	DefaultDiscoveryRefreshInterval = 3 * time.Minute

	// DefaultDHTBucketSize 默认 DHT K-桶大小
	DefaultDHTBucketSize = 20

	// DefaultDHTConcurrency 默认 DHT 并发度
	DefaultDHTConcurrency = 3

	// DefaultMDNSInterval 默认 mDNS 发现间隔
	DefaultMDNSInterval = 10 * time.Second
)

// NAT 默认值
const (
	// DefaultNATRefreshInterval 默认 NAT 刷新间隔
	DefaultNATRefreshInterval = 30 * time.Second
)

// 中继默认值
const (
	// DefaultRelayMaxReservations 默认最大预留数
	DefaultRelayMaxReservations = 128

	// DefaultRelayMaxCircuits 默认最大电路数
	DefaultRelayMaxCircuits = 16

	// DefaultRelayMaxCircuitsPerPeer 默认每节点最大电路数
	DefaultRelayMaxCircuitsPerPeer = 4

	// DefaultRelayReservationTTL 默认预留有效期
	DefaultRelayReservationTTL = 1 * time.Hour
)

// 消息默认值
const (
	// DefaultRequestTimeout 默认请求超时
	DefaultRequestTimeout = 30 * time.Second

	// DefaultMaxMessageSize 默认最大消息大小
	DefaultMaxMessageSize = 4 * 1024 * 1024 // 4 MB

	// DefaultMessageCacheSize 默认消息缓存大小
	DefaultMessageCacheSize = 1000

	// DefaultMessageCacheTTL 默认消息缓存 TTL
	DefaultMessageCacheTTL = 2 * time.Minute
)

// ============================================================================
//                              默认 STUN 服务器
// ============================================================================

// DefaultSTUNServers 默认 STUN 服务器列表
var DefaultSTUNServers = []string{
	"stun:stun.l.google.com:19302",
	"stun:stun1.l.google.com:19302",
	"stun:stun2.l.google.com:19302",
	"stun:stun3.l.google.com:19302",
	"stun:stun4.l.google.com:19302",
}

// ============================================================================
//                              默认端口
// ============================================================================

const (
	// DefaultListenPort 默认监听端口
	// QUIC 协议监听端口，0 表示使用系统分配的随机端口
	DefaultListenPort = 4001
)

// ============================================================================
//                              默认引导节点
// ============================================================================

// defaultBootstrapPeers 官方公共引导节点（内部变量）
//
// 这些是 dep2p 官方部署的公共基础设施节点，用于首次加入网络时发现其他节点。
// 格式：/dns4/<domain>/udp/<port>/quic-v1/p2p/<NodeID>
//
// 注意：
//   - 用户可以通过 WithBootstrapPeers() 覆盖
//   - 私有网络部署应使用自己的 bootstrap 节点
//   - 测试环境建议使用 WithBootstrapPeers(nil) 创建隔离网络
//
// TODO: 部署公共 bootstrap 节点后更新此列表
var defaultBootstrapPeers = []string{
	// 亚太区域 (Asia-Pacific)
	// "/dns4/bootstrap-ap.dep2p.io/udp/4001/quic-v1/p2p/<NodeID>",

	// 欧洲区域 (Europe)
	// "/dns4/bootstrap-eu.dep2p.io/udp/4001/quic-v1/p2p/<NodeID>",

	// 北美区域 (North America)
	// "/dns4/bootstrap-na.dep2p.io/udp/4001/quic-v1/p2p/<NodeID>",
}

// GetDefaultBootstrapPeers 返回官方默认引导节点列表的副本
//
// 返回副本而非直接暴露变量，避免外部修改影响内部状态。
// 这是配置系统的唯一默认值真源。
func GetDefaultBootstrapPeers() []string {
	result := make([]string, len(defaultBootstrapPeers))
	copy(result, defaultBootstrapPeers)
	return result
}

// ============================================================================
//                              环境变量（供 CLI 使用）
// ============================================================================

// 环境变量前缀和名称常量（供 cmd 层使用）
const (
	// EnvPrefix 环境变量前缀
	EnvPrefix = "DEP2P_"

	// EnvBootstrapPeers bootstrap 节点列表（逗号分隔）
	EnvBootstrapPeers = "BOOTSTRAP_PEERS"

	// EnvListenPort 监听端口
	EnvListenPort = "LISTEN_PORT"

	// EnvPreset 预设名称
	EnvPreset = "PRESET"

	// EnvRealm 业务域 ID
	EnvRealm = "REALM"

	// EnvLogFile 日志文件路径
	EnvLogFile = "LOG_FILE"

	// EnvIdentityKeyFile 身份密钥文件路径
	EnvIdentityKeyFile = "IDENTITY_KEY_FILE"

	// EnvEnableRelay 启用中继
	EnvEnableRelay = "ENABLE_RELAY"

	// EnvEnableNAT 启用 NAT 穿透
	EnvEnableNAT = "ENABLE_NAT"
)

