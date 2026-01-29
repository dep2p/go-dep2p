// Package config 提供统一的配置管理
//
// 本包采用混合配置模式：
//   - 主 Config 结构体嵌入所有子配置
//   - 每个子配置在独立文件中定义
//   - 支持从 JSON 加载和保存配置
//   - 支持预设配置（mobile/desktop/server/minimal）
//
// 使用示例：
//
//	// 创建默认配置
//	cfg := config.NewConfig()
//	cfg.Transport.EnableQUIC = true
//	cfg.NAT.EnableAutoNAT = true
//
//	// 使用预设配置
//	cfg := config.NewServerConfig()
//
//	// 应用预设到现有配置
//	config.ApplyPreset(cfg, "server")
//
//	// 从 JSON 加载
//	cfg, err := config.FromJSON(data)
package config

// KnownPeer 已知节点配置
//
// 用于配置启动时直接连接的节点，不依赖引导节点或 DHT 发现。
// 适用于云服务器部署、私有网络等已知节点地址的场景。
type KnownPeer struct {
	// PeerID 目标节点的 Peer ID
	PeerID string `json:"peer_id"`

	// Addrs 目标节点的地址列表
	// 格式为 multiaddr，例如 "/ip4/1.2.3.4/udp/4001/quic-v1"
	Addrs []string `json:"addrs"`
}

// Config 是 DEP2P 的完整配置结构
//
// 该结构体嵌入了所有组件的子配置，提供统一的配置接口。
// 配置按照功能模块组织：
//   - Identity: 身份和密钥管理
//   - Transport: 传输协议（TCP/QUIC/WebSocket）
//   - Security: 安全传输（TLS/Noise）
//   - NAT: NAT 穿透（AutoNAT/UPnP/HolePunch）
//   - Relay: 中继服务
//   - Discovery: 节点发现（DHT/mDNS/Bootstrap）
//   - ConnMgr: 连接管理
//   - Messaging: 消息传递（PubSub/Streams）
//   - Realm: Realm 管理
//   - Resource: 资源管理
//   - Storage: 存储配置
//   - Bandwidth: 带宽统计
//   - PathHealth: 路径健康管理
//   - Recovery: 网络恢复
//   - ConnectionHealth: 连接健康监控
type Config struct {
	// Identity 身份配置
	Identity IdentityConfig `json:"identity"`

	// Transport 传输层配置
	Transport TransportConfig `json:"transport"`

	// Security 安全传输配置
	Security SecurityConfig `json:"security"`

	// NAT NAT 穿透配置
	NAT NATConfig `json:"nat"`

	// Relay 中继服务配置
	Relay RelayConfig `json:"relay"`

	// Discovery 节点发现配置
	Discovery DiscoveryConfig `json:"discovery"`

	// ConnMgr 连接管理配置
	ConnMgr ConnManagerConfig `json:"conn_mgr"`

	// Messaging 消息传递配置
	Messaging MessagingConfig `json:"messaging"`

	// Realm Realm 管理配置
	Realm RealmConfig `json:"realm"`

	// Resource 资源管理配置
	Resource ResourceConfig `json:"resource"`

	// Storage 存储配置
	Storage StorageConfig `json:"storage"`

	// Bandwidth 带宽统计配置
	Bandwidth BandwidthConfig `json:"bandwidth"`

	// PathHealth 路径健康管理配置
	PathHealth PathHealthConfig `json:"path_health"`

	// Recovery 网络恢复配置
	Recovery RecoveryConfig `json:"recovery"`

	// ConnectionHealth 连接健康监控配置
	ConnectionHealth ConnectionHealthConfig `json:"connection_health"`

	// Diagnostics 诊断服务配置
	Diagnostics DiagnosticsConfig `json:"diagnostics"`

	// KnownPeers 已知节点列表
	//
	// 启动时将直接连接这些节点，不依赖引导节点或 DHT 发现。
	// 适用于云服务器部署、私有网络等已知节点地址的场景。
	KnownPeers []KnownPeer `json:"known_peers,omitempty"`
}

// NewConfig 创建默认配置
//
// 返回的配置使用所有组件的默认值，适用于大多数场景。
// 可以通过修改字段或使用 Option 函数来定制配置。
func NewConfig() *Config {
	return &Config{
		Identity:         DefaultIdentityConfig(),
		Transport:        DefaultTransportConfig(),
		Security:         DefaultSecurityConfig(),
		NAT:              DefaultNATConfig(),
		Relay:            DefaultRelayConfig(),
		Discovery:        DefaultDiscoveryConfig(),
		ConnMgr:          DefaultConnManagerConfig(),
		Messaging:        DefaultMessagingConfig(),
		Realm:            DefaultRealmConfig(),
		Resource:         DefaultResourceConfig(),
		Storage:          DefaultStorageConfig(),
		Bandwidth:        DefaultBandwidthConfig(),
		PathHealth:       DefaultPathHealthConfig(),
		Recovery:         DefaultRecoveryConfig(),
		ConnectionHealth: DefaultConnectionHealthConfig(),
		Diagnostics:      DefaultDiagnosticsConfig(),
	}
}

// Validate 验证配置的有效性
//
// 检查所有子配置是否有效，如果发现无效配置则返回错误。
// 建议在使用配置前调用此方法。
func (c *Config) Validate() error {
	if err := c.Identity.Validate(); err != nil {
		return err
	}
	if err := c.Transport.Validate(); err != nil {
		return err
	}
	if err := c.Security.Validate(); err != nil {
		return err
	}
	if err := c.NAT.Validate(); err != nil {
		return err
	}
	if err := c.Relay.Validate(); err != nil {
		return err
	}
	if err := c.Discovery.Validate(); err != nil {
		return err
	}
	if err := c.ConnMgr.Validate(); err != nil {
		return err
	}
	if err := c.Messaging.Validate(); err != nil {
		return err
	}
	if err := c.Realm.Validate(); err != nil {
		return err
	}
	if err := c.Resource.Validate(); err != nil {
		return err
	}
	if err := c.Storage.Validate(); err != nil {
		return err
	}
	if err := c.Bandwidth.Validate(); err != nil {
		return err
	}
	if err := c.PathHealth.Validate(); err != nil {
		return err
	}
	if err := c.Recovery.Validate(); err != nil {
		return err
	}
	if err := c.ConnectionHealth.Validate(); err != nil {
		return err
	}
	return nil
}
