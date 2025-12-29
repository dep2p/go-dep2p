package config

import (
	"fmt"
	"strings"
	"time"

	"go.uber.org/fx"

	endpointmod "github.com/dep2p/go-dep2p/internal/core/endpoint"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Provider 配置提供者
//
// Provider 负责将配置分发给各个组件
type Provider struct {
	config *Config
}

// NewProvider 创建配置提供者
func NewProvider(config *Config) *Provider {
	return &Provider{
		config: config,
	}
}

// GetConfig 获取完整配置
func (p *Provider) GetConfig() *Config {
	return p.config
}

// GetIdentity 获取身份配置
func (p *Provider) GetIdentity() *IdentityConfig {
	return &p.config.Identity
}

// GetTransport 获取传输配置
func (p *Provider) GetTransport() *TransportConfig {
	return &p.config.Transport
}

// GetSecurity 获取安全配置
func (p *Provider) GetSecurity() *SecurityConfig {
	return &p.config.Security
}

// GetMuxer 获取多路复用配置
func (p *Provider) GetMuxer() *MuxerConfig {
	return &p.config.Muxer
}

// GetNAT 获取 NAT 配置
func (p *Provider) GetNAT() *NATConfig {
	return &p.config.NAT
}

// GetDiscovery 获取发现配置
func (p *Provider) GetDiscovery() *DiscoveryConfig {
	return &p.config.Discovery
}

// GetRelay 获取中继配置
func (p *Provider) GetRelay() *RelayConfig {
	return &p.config.Relay
}

// GetConnectionManager 获取连接管理配置
func (p *Provider) GetConnectionManager() *ConnectionManagerConfig {
	return &p.config.ConnectionManager
}

// GetProtocol 获取协议配置
func (p *Provider) GetProtocol() *ProtocolConfig {
	return &p.config.Protocol
}

// GetMessaging 获取消息服务配置
func (p *Provider) GetMessaging() *MessagingConfig {
	return &p.config.Messaging
}

// GetListenAddrs 获取监听地址
func (p *Provider) GetListenAddrs() []string {
	return p.config.ListenAddrs
}

// ============================================================================
//                              fx 模块
// ============================================================================

// ProviderResult fx 提供者结果
type ProviderResult struct {
	fx.Out

	Provider          *Provider
	IdentityConfig    *IdentityConfig          `name:"identity_config"`
	TransportConfig   *TransportConfig         `name:"transport_config"`
	SecurityConfig    *SecurityConfig          `name:"security_config"`
	MuxerConfig       *MuxerConfig             `name:"muxer_config"`
	NATConfig         *NATConfig               `name:"nat_config"`
	DiscoveryConfig   *DiscoveryConfig         `name:"discovery_config"`
	RelayConfig       *RelayConfig             `name:"relay_config"`
	ConnMgrConfig     *ConnectionManagerConfig `name:"connmgr_config"`
	ProtocolConfig    *ProtocolConfig          `name:"protocol_config"`
	MessagingConfig   *MessagingConfig         `name:"messaging_config"`
	EndpointConfig    *endpointmod.Config      // endpoint 配置（自动注入到 endpoint.ModuleInput.Config）

	// pkg/interfaces/* 配置（供各模块 Fx 注入使用）
	IdentityIfConfig  *identityif.Config  // identity 模块使用
	DiscoveryIfConfig *discoveryif.Config // discovery 模块使用
	RelayIfConfig     *relayif.Config     // relay 模块使用
}

// ProvideConfig 提供配置
//
// 返回错误如果配置验证失败（如 Bootstrap 地址不符合 SPEC-BOOTSTRAP-001）
func ProvideConfig(config *Config) (ProviderResult, error) {
	provider := NewProvider(config)

	// 创建 endpoint 配置
	endpointConfig := &endpointmod.Config{
		ListenAddrs:    config.ListenAddrs,
		DialTimeout:    int(config.Transport.DialTimeout.Seconds()),
		MaxConnections: config.ConnectionManager.HighWater,
		ExternalAddrs:  config.NAT.ExternalAddrs,
	}

	// 创建 identityif.Config（供 identity 模块使用）
	identityIfConfig := convertToIdentityIfConfig(&config.Identity)

	// 创建 discoveryif.Config（供 discovery 模块使用）
	// SPEC-BOOTSTRAP-001: 验证 Bootstrap 地址格式
	discoveryIfConfig, err := convertToDiscoveryIfConfig(&config.Discovery)
	if err != nil {
		return ProviderResult{}, err
	}

	// 创建 relayif.Config（供 relay 模块使用）
	relayIfConfig := convertToRelayIfConfig(&config.Relay)

	return ProviderResult{
		Provider:          provider,
		IdentityConfig:    provider.GetIdentity(),
		TransportConfig:   provider.GetTransport(),
		SecurityConfig:    provider.GetSecurity(),
		MuxerConfig:       provider.GetMuxer(),
		NATConfig:         provider.GetNAT(),
		DiscoveryConfig:   provider.GetDiscovery(),
		RelayConfig:       provider.GetRelay(),
		ConnMgrConfig:     provider.GetConnectionManager(),
		ProtocolConfig:    provider.GetProtocol(),
		MessagingConfig:   provider.GetMessaging(),
		EndpointConfig:    endpointConfig,
		IdentityIfConfig:  identityIfConfig,
		DiscoveryIfConfig: discoveryIfConfig,
		RelayIfConfig:     relayIfConfig,
	}, nil
}

// convertToIdentityIfConfig 将内部配置转换为 identityif.Config
func convertToIdentityIfConfig(cfg *IdentityConfig) *identityif.Config {
	result := &identityif.Config{
		KeyType:      types.KeyTypeEd25519, // 默认
		IdentityPath: cfg.KeyFile,
		AutoCreate:   true,
		PrivateKey:   nil,
	}

	// 处理 KeyType
	switch strings.ToLower(cfg.KeyType) {
	case "ed25519", "":
		result.KeyType = types.KeyTypeEd25519
	case "ecdsa", "ecdsa-p256":
		result.KeyType = types.KeyTypeECDSA
	}

	// 处理 PrivateKey（类型断言）
	if cfg.PrivateKey != nil {
		if pk, ok := cfg.PrivateKey.(identityif.PrivateKey); ok {
			result.PrivateKey = pk
		}
	}

	return result
}

// convertToDiscoveryIfConfig 将内部配置转换为 discoveryif.Config
//
// 返回错误如果 Bootstrap 配置不符合 SPEC-BOOTSTRAP-001
func convertToDiscoveryIfConfig(cfg *DiscoveryConfig) (*discoveryif.Config, error) {
	// SPEC-BOOTSTRAP-001: 解析并验证 Bootstrap peers
	bootstrapPeers, err := parseBootstrapPeers(cfg.BootstrapPeers)
	if err != nil {
		return nil, fmt.Errorf("配置验证失败: %w", err)
	}

	result := &discoveryif.Config{
		EnableDHT:          true,
		EnableMDNS:         true,
		EnableBootstrap:    len(cfg.BootstrapPeers) > 0,
		DHTMode:            convertDHTMode(cfg.DHT.Mode),
		BootstrapPeers:     bootstrapPeers,
		RefreshInterval:    cfg.RefreshInterval,
		MDNSServiceTag:     cfg.MDNS.ServiceTag,
		TargetPeers:        50,
		MinInterval:        5 * time.Second,
		MaxInterval:        5 * time.Minute,
		EnableRendezvous:   true,
		ServeRendezvous:    false,
		EnableDNS:          false,
	}

	if result.RefreshInterval == 0 {
		result.RefreshInterval = 10 * time.Minute
	}
	if result.MDNSServiceTag == "" {
		result.MDNSServiceTag = "_dep2p._udp"
	}

	return result, nil
}

// convertDHTMode 转换 DHT 模式
func convertDHTMode(mode string) discoveryif.DHTMode {
	switch strings.ToLower(mode) {
	case "server":
		return discoveryif.DHTModeServer
	case "client":
		return discoveryif.DHTModeClient
	default:
		return discoveryif.DHTModeAuto
	}
}

// convertToRelayIfConfig 将内部配置转换为 relayif.Config
//
// 这是确保 EnableServer 配置正确传递给 relay 模块的关键
func convertToRelayIfConfig(cfg *RelayConfig) *relayif.Config {
	return &relayif.Config{
		EnableClient:      cfg.Enable,
		EnableServer:      cfg.EnableServer,
		EnableAutoRelay:   true,
		MaxReservations:   cfg.MaxReservations,
		MaxCircuits:       cfg.MaxCircuits,
		ReservationTTL:    cfg.ReservationTTL,
		ConnectionTimeout: 30 * time.Second,
	}
}

// parseBootstrapPeers 解析 bootstrap peers 字符串为 PeerInfo
//
// SPEC-BOOTSTRAP-001: Bootstrap seed 地址必须使用 Full Address 格式
//
// 要求：
//   - 所有地址必须包含 /p2p/<NodeID> 后缀（Full Address 格式）
//   - NodeID 必须非空
//   - 不接受纯地址（Dial Address），否则返回错误
//   - 不接受 RelayCircuitAddress（含 /p2p-circuit/），否则返回错误
//
// 正确示例：
//
//	/ip4/203.0.113.5/udp/4001/quic-v1/p2p/5Q2STWvBExampleNodeID...
//
// 错误示例（将被拒绝）：
//
//	/ip4/203.0.113.5/udp/4001/quic-v1              ← 缺少 /p2p/<NodeID>
//	/ip4/.../p2p/RelayID/p2p-circuit/p2p/TargetID  ← RelayCircuitAddress 禁止
func parseBootstrapPeers(addrs []string) ([]discoveryif.PeerInfo, error) {
	if len(addrs) == 0 {
		return nil, nil
	}

	peers := make([]discoveryif.PeerInfo, 0, len(addrs))
	for _, addr := range addrs {
		if addr == "" {
			continue
		}

		// SPEC-BOOTSTRAP-001: 检查是否包含 /p2p/<NodeID>
		ma := types.Multiaddr(addr)
		peerID := ma.PeerID()
		if peerID.IsEmpty() {
			return nil, fmt.Errorf(
				"SPEC-BOOTSTRAP-001 违规: Bootstrap 地址必须使用 Full Address 格式（含 /p2p/<NodeID>），"+
					"收到 Dial Address: %s", addr)
		}

		// SPEC-BOOTSTRAP-001 + INV-005: 禁止 RelayCircuitAddress 作为 Bootstrap seed
		// Relay 仅作为 Layer 1 内部可达性兜底，不得对外作为入网种子
		if ma.IsRelay() {
			return nil, fmt.Errorf(
				"SPEC-BOOTSTRAP-001 违规: Bootstrap 地址不得为 RelayCircuitAddress（含 /p2p-circuit/），"+
					"Relay 仅作内部可达兜底，不可用于入网种子: %s", addr)
		}

		// 解析 Full Address: 获取去除 /p2p/ 后缀的 dial address
		dialAddr := string(ma.WithoutPeerID())

		peer := discoveryif.PeerInfo{
			ID:    peerID,
			Addrs: types.StringsToMultiaddrs([]string{dialAddr}),
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

// Module 返回配置 fx 模块
func Module() fx.Option {
	return fx.Module("config",
		fx.Provide(ProvideConfig),
	)
}

