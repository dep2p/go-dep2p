package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// FromJSON 从 JSON 数据创建配置
//
// 支持从 JSON 文件或字符串加载配置。
// JSON 格式与 Config 结构体一一对应。
//
// 示例 JSON:
//
//	{
//	  "Identity": {"KeyType": "Ed25519"},
//	  "Transport": {"EnableQUIC": true},
//	  "ConnMgr": {"LowWater": 100, "HighWater": 400}
//	}
func FromJSON(data []byte) (*Config, error) {
	cfg := NewConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	return cfg, nil
}

// ApplyPreset 应用预设配置
//
// Preset 提供了针对不同场景优化的配置组合。
// 该函数将预设应用到配置上。
//
// 支持的预设：
//   - "mobile": 移动端优化
//   - "desktop": 桌面端默认
//   - "server": 服务器优化
//   - "minimal": 最小配置
func ApplyPreset(cfg *Config, presetName string) error {
	if cfg == nil {
		return errors.New("config is nil")
	}

	switch presetName {
	case "mobile":
		return applyMobilePreset(cfg)
	case "desktop":
		return applyDesktopPreset(cfg)
	case "server":
		return applyServerPreset(cfg)
	case "minimal":
		return applyMinimalPreset(cfg)
	case "":
		// 空预设，不做任何操作
		return nil
	default:
		return fmt.Errorf("unknown preset: %s", presetName)
	}
}

// applyMobilePreset 应用移动端预设
//
// 移动端配置优化：
//   - 低资源占用
//   - 省电（更长的发现间隔）
//   - 仅 QUIC（0-RTT、连接迁移）
//   - 禁用服务端功能
func applyMobilePreset(cfg *Config) error {
	// 传输：仅 QUIC，移动网络最佳选择
	cfg.Transport.EnableQUIC = true
	cfg.Transport.EnableTCP = false
	cfg.Transport.EnableWebSocket = false

	// NAT：启用客户端功能，禁用服务端
	cfg.NAT.EnableAutoNAT = true
	cfg.NAT.EnableUPnP = true
	cfg.NAT.EnableNATPMP = true
	cfg.NAT.EnableHolePunch = true
	cfg.NAT.AutoNAT.EnableServer = false // 移动端不应为他人服务

	// 中继：仅客户端
	cfg.Relay.EnableClient = true
	cfg.Relay.EnableServer = false // 移动端不应为他人中继

	// 发现：更长的间隔，节省电量
	cfg.Discovery.DHT.RefreshInterval = Duration(2 * time.Hour)
	cfg.Discovery.MDNS.Interval = Duration(30 * time.Second)

	// 连接管理：低限制
	cfg.ConnMgr.LowWater = 20
	cfg.ConnMgr.HighWater = 100

	// 消息传递：更长的心跳间隔
	cfg.Messaging.Liveness.HeartbeatInterval = 60 * time.Second
	cfg.Messaging.Liveness.HeartbeatTimeout = 180 * time.Second

	// 资源：低限制，适合移动设备
	cfg.Resource.System.MaxConnections = 200
	cfg.Resource.System.MaxStreams = 2000
	cfg.Resource.System.MaxMemory = 256 << 20 // 256 MB
	cfg.Resource.Peer.MaxConnectionsPerPeer = 4
	cfg.Resource.Peer.MaxStreamsPerPeer = 10

	return nil
}

// applyDesktopPreset 应用桌面端预设
func applyDesktopPreset(_ *Config) error {
	// 使用默认配置（已经针对桌面优化）
	return nil
}

// applyServerPreset 应用服务器预设
//
// 服务器配置优化：
//   - 高资源配置
//   - 启用所有传输协议
//   - 启用中继和 AutoNAT 服务端
//   - 支持大规模并发
func applyServerPreset(cfg *Config) error {
	// 传输：启用所有协议，最大化兼容性
	cfg.Transport.EnableQUIC = true
	cfg.Transport.EnableTCP = true
	cfg.Transport.EnableWebSocket = true  // 支持浏览器客户端
	cfg.Transport.QUIC.MaxStreams = 4096  // 高并发支持

	// NAT：启用服务端，帮助其他节点
	cfg.NAT.AutoNAT.EnableServer = true
	cfg.NAT.AutoNAT.ServerMaxProbes = 50 // 每分钟最大探测数

	// 中继：启用服务端
	cfg.Relay.EnableClient = true
	cfg.Relay.EnableServer = true
	cfg.Relay.Server.MaxReservations = 512 // 支持更多节点
	cfg.Relay.Server.MaxCircuits = 64      // 支持更多并发

	// 连接管理：高限制
	cfg.ConnMgr.LowWater = 500
	cfg.ConnMgr.HighWater = 2000

	// 资源：高限制，充分利用服务器资源
	// 注意：MaxStreams >= MaxConnections * MaxStreamsPerPeer
	cfg.Resource.System.MaxConnections = 5000
	cfg.Resource.System.MaxStreams = 100000 // 5000 * 20 = 100000
	cfg.Resource.System.MaxMemory = 4 << 30 // 4 GB
	cfg.Resource.Peer.MaxConnectionsPerPeer = 16
	cfg.Resource.Peer.MaxStreamsPerPeer = 20 // 避免单节点占用过多

	return nil
}

// applyMinimalPreset 应用最小预设
//
// 最小配置优化：
//   - 最低资源占用
//   - 仅本地发现
//   - 禁用大多数功能
//   - 适合测试和开发
func applyMinimalPreset(cfg *Config) error {
	// 传输：仅 QUIC
	cfg.Transport.EnableQUIC = true
	cfg.Transport.EnableTCP = false
	cfg.Transport.EnableWebSocket = false
	cfg.Transport.QUIC.MaxStreams = 100
	cfg.Transport.DialTimeout = Duration(10 * time.Second) // 快速失败

	// NAT：仅 AutoNAT，禁用路由器交互
	cfg.NAT.EnableAutoNAT = true
	cfg.NAT.EnableUPnP = false   // 禁用 UPnP
	cfg.NAT.EnableNATPMP = false // 禁用 NAT-PMP
	cfg.NAT.EnableHolePunch = false
	cfg.NAT.AutoNAT.EnableServer = false

	// 中继：完全禁用
	cfg.Relay.EnableClient = false
	cfg.Relay.EnableServer = false

	// 发现：仅 mDNS（局域网）
	cfg.Discovery.EnableDHT = false
	cfg.Discovery.EnableMDNS = true
	cfg.Discovery.EnableBootstrap = false
	cfg.Discovery.EnableRendezvous = false
	cfg.Discovery.EnableDNS = false

	// 连接管理：极低限制
	cfg.ConnMgr.LowWater = 10
	cfg.ConnMgr.HighWater = 50

	// 资源：极低限制
	cfg.Resource.System.MaxConnections = 100
	cfg.Resource.System.MaxStreams = 1000
	cfg.Resource.System.MaxMemory = 64 << 20 // 64 MB
	cfg.Resource.Peer.MaxConnectionsPerPeer = 2
	cfg.Resource.Peer.MaxStreamsPerPeer = 10

	return nil
}

// MergeConfigs 合并多个配置
//
// 将多个配置合并为一个，后面的配置会完全覆盖前面的配置。
// 用于实现配置的分层覆盖（默认配置 -> 预设配置 -> 用户配置）。
//
// 合并策略：后者完全覆盖前者
//   - 如果需要逐字段合并，请在调用前手动处理
//   - nil 配置会被跳过
//
// 示例：
//
//	// 用户配置完全覆盖预设配置
//	merged, _ := MergeConfigs(DefaultConfig(), PresetDesktop(), userConfig)
func MergeConfigs(configs ...*Config) (*Config, error) {
	if len(configs) == 0 {
		return NewConfig(), nil
	}

	// 从第一个非 nil 配置开始
	var result *Config
	for _, cfg := range configs {
		if cfg != nil {
			// 后者完全覆盖前者
			result = cfg
		}
	}

	if result == nil {
		return NewConfig(), nil
	}

	return result, nil
}

// CloneConfig 克隆配置
//
// 创建配置的深拷贝，用于安全地修改配置而不影响原始配置。
func CloneConfig(cfg *Config) *Config {
	if cfg == nil {
		return nil
	}

	// 创建新配置
	cloned := &Config{
		Identity:  cfg.Identity,
		Transport: cfg.Transport,
		Security:  cfg.Security,
		NAT:       cfg.NAT,
		Relay:     cfg.Relay,
		Discovery: cfg.Discovery,
		ConnMgr:   cfg.ConnMgr,
		Messaging: cfg.Messaging,
		Realm:     cfg.Realm,
		Resource:  cfg.Resource,
	}

	return cloned
}

// ToComponentConfig 将统一配置转换为组件特定配置
//
// 各个组件可能需要特定格式的配置，该函数提供转换功能。
// 使用泛型或接口来支持不同类型的组件配置。
type ComponentConfigConverter interface {
	// FromConfig 从统一配置创建组件配置
	FromConfig(cfg *Config) error
}

// ConvertForComponent 为特定组件转换配置
func ConvertForComponent(cfg *Config, component string) (interface{}, error) {
	if cfg == nil {
		return nil, errors.New("config is nil")
	}

	switch component {
	case "transport":
		return cfg.Transport, nil
	case "security":
		return cfg.Security, nil
	case "nat":
		return cfg.NAT, nil
	case "relay":
		return cfg.Relay, nil
	case "discovery":
		return cfg.Discovery, nil
	case "connmgr":
		return cfg.ConnMgr, nil
	case "messaging":
		return cfg.Messaging, nil
	case "realm":
		return cfg.Realm, nil
	case "resource":
		return cfg.Resource, nil
	default:
		return nil, fmt.Errorf("unknown component: %s", component)
	}
}
