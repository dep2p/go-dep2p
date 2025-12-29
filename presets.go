package dep2p

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// Preset 预设配置
// 预设封装了一组针对特定场景优化的默认配置
type Preset struct {
	// Name 预设名称
	Name string

	// Description 预设描述
	Description string

	// 连接限制
	ConnectionLowWater  int
	ConnectionHighWater int

	// 发现配置
	DiscoveryInterval time.Duration
	// BootstrapPeers 引导节点列表
	//
	// 格式（推荐完整地址，含 /p2p/<NodeID>）：
	//   /dns4/bootstrap1.dep2p.io/udp/4001/quic-v1/p2p/<NodeID>
	//   /ip4/1.2.3.4/udp/4001/quic-v1/p2p/<NodeID>
	//
	// 如果为 nil 或空，表示创世节点/无 bootstrap 场景。
	// PresetMobile/Desktop/Server 默认填充公共 bootstrap 节点。
	// PresetMinimal/Test 默认为空（测试隔离）。
	BootstrapPeers []string

	// 中继配置
	EnableRelay       bool
	EnableRelayServer bool

	// NAT 配置
	EnableNAT bool

	// 传输配置
	MaxConnections    int
	MaxStreamsPerConn int
	IdleTimeout       time.Duration

	// Realm 配置
	// v1.1+：Realm 为底层必备能力，不再提供 Enable 开关
	IsolateDiscovery bool
	IsolatePubSub    bool

	// Liveness 配置
	EnableLiveness    bool
	HeartbeatInterval time.Duration
	EnableGoodbye     bool
}

// Apply 将预设配置应用到内部配置
func (p *Preset) Apply(cfg *config.Config) {
	// 连接限制
	cfg.ConnectionManager.LowWater = p.ConnectionLowWater
	cfg.ConnectionManager.HighWater = p.ConnectionHighWater

	// 发现配置
	cfg.Discovery.RefreshInterval = p.DiscoveryInterval
	// Bootstrap peers：仅在 Preset 提供了非空列表时应用
	// 这允许 WithBootstrapPeers(nil) 覆盖 Preset 的默认值
	if len(p.BootstrapPeers) > 0 {
		cfg.Discovery.BootstrapPeers = p.BootstrapPeers
	}

	// 中继配置
	cfg.Relay.Enable = p.EnableRelay
	cfg.Relay.EnableServer = p.EnableRelayServer

	// NAT 配置
	cfg.NAT.Enable = p.EnableNAT

	// 传输配置
	cfg.Transport.MaxConnections = p.MaxConnections
	cfg.Transport.MaxStreamsPerConn = p.MaxStreamsPerConn
	cfg.Transport.IdleTimeout = p.IdleTimeout

	// Realm 配置
	cfg.Realm.IsolateDiscovery = p.IsolateDiscovery
	cfg.Realm.IsolatePubSub = p.IsolatePubSub

	// Liveness 配置
	cfg.Liveness.Enable = p.EnableLiveness
	if p.HeartbeatInterval > 0 {
		cfg.Liveness.HeartbeatInterval = p.HeartbeatInterval
	}
	cfg.Liveness.EnableGoodbye = p.EnableGoodbye
}

// ============================================================================
//                              预定义预设
// ============================================================================

// 注意: 默认 bootstrap 节点由 internal/config.GetDefaultBootstrapPeers() 提供
// 这是配置系统的唯一默认值真源

// PresetMobile 移动端预设
//
// 针对手机、平板等资源受限设备优化:
//   - 低连接数限制 (20/50)
//   - 启用发现服务（低频率）
//   - 启用中继（用于 NAT 穿透）
//   - 较短的空闲超时
//   - 使用默认 bootstrap 节点
var PresetMobile = &Preset{
	Name:        "mobile",
	Description: "移动端优化配置，低资源占用",

	ConnectionLowWater:  20,
	ConnectionHighWater: 50,

	DiscoveryInterval: 5 * time.Minute,
	BootstrapPeers:    config.GetDefaultBootstrapPeers(),

	EnableRelay:       true,
	EnableRelayServer: false,

	EnableNAT: true,

	MaxConnections:    50,
	MaxStreamsPerConn: 128,
	IdleTimeout:       2 * time.Minute,

	// Realm: 强制内建，隔离发现和 PubSub
	IsolateDiscovery: true,
	IsolatePubSub:    true,

	// Liveness: 启用，较长心跳间隔（省电）
	EnableLiveness:    true,
	HeartbeatInterval: 30 * time.Second,
	EnableGoodbye:     true,
}

// PresetDesktop 桌面端预设
//
// 针对 PC、笔记本等普通设备优化:
//   - 中等连接数限制 (50/100)
//   - 启用全部发现服务
//   - 启用中继
//   - 标准空闲超时
//   - 使用默认 bootstrap 节点
var PresetDesktop = &Preset{
	Name:        "desktop",
	Description: "桌面端默认配置",

	ConnectionLowWater:  50,
	ConnectionHighWater: 100,

	DiscoveryInterval: 3 * time.Minute,
	BootstrapPeers:    config.GetDefaultBootstrapPeers(),

	EnableRelay:       true,
	EnableRelayServer: false,

	EnableNAT: true,

	MaxConnections:    100,
	MaxStreamsPerConn: 256,
	IdleTimeout:       5 * time.Minute,

	// Realm: 强制内建，隔离发现和 PubSub
	IsolateDiscovery: true,
	IsolatePubSub:    true,

	// Liveness: 启用，标准心跳间隔
	EnableLiveness:    true,
	HeartbeatInterval: 15 * time.Second,
	EnableGoodbye:     true,
}

// PresetServer 服务器预设
//
// 针对服务器、高性能节点优化:
//   - 高连接数限制 (200/500)
//   - 启用全部发现服务（高频率）
//   - 可作为中继服务器
//   - 较长的空闲超时
//   - 使用默认 bootstrap 节点
var PresetServer = &Preset{
	Name:        "server",
	Description: "服务器优化配置，高性能",

	ConnectionLowWater:  200,
	ConnectionHighWater: 500,

	DiscoveryInterval: 1 * time.Minute,
	BootstrapPeers:    config.GetDefaultBootstrapPeers(),

	EnableRelay:       true,
	EnableRelayServer: true,

	EnableNAT: true,

	MaxConnections:    500,
	MaxStreamsPerConn: 512,
	IdleTimeout:       10 * time.Minute,

	// Realm: 强制内建，隔离发现和 PubSub
	IsolateDiscovery: true,
	IsolatePubSub:    true,

	// Liveness: 启用，较短心跳间隔（高可用性）
	EnableLiveness:    true,
	HeartbeatInterval: 10 * time.Second,
	EnableGoodbye:     true,
}

// PresetMinimal 最小预设
//
// 仅用于测试和特殊场景:
//   - 极低连接数限制 (10/20)
//   - 发现服务为强制内建（无法禁用）；本预设仅通过更低连接上限/更低资源参数进行收敛
//   - 禁用中继
//   - 禁用 NAT
//   - 无 bootstrap 节点（隔离环境）
var PresetMinimal = &Preset{
	Name:        "minimal",
	Description: "最小配置，仅用于测试",

	ConnectionLowWater:  10,
	ConnectionHighWater: 20,

	DiscoveryInterval: 30 * time.Minute,
	BootstrapPeers:    nil, // 无 bootstrap，用于隔离测试

	EnableRelay:       false,
	EnableRelayServer: false,

	EnableNAT: false,

	MaxConnections:    20,
	MaxStreamsPerConn: 64,
	IdleTimeout:       1 * time.Minute,

	// Realm: 强制内建；Minimal 预设不改变隔离策略（保持默认隔离）
	IsolateDiscovery: true,
	IsolatePubSub:    true,

	// Liveness: 禁用
	EnableLiveness:    false,
	HeartbeatInterval: 0,
	EnableGoodbye:     false,
}

// PresetTest 测试预设
//
// 用于单元测试和集成测试:
//   - 低连接数限制
//   - 发现服务为强制内建（无法禁用）；测试场景仅通过更短刷新间隔/本地监听来收敛外部依赖
//   - 无 bootstrap 节点（测试隔离）
var PresetTest = &Preset{
	Name:        "test",
	Description: "测试配置",

	ConnectionLowWater:  5,
	ConnectionHighWater: 10,

	DiscoveryInterval: 10 * time.Second,
	BootstrapPeers:    nil, // 无 bootstrap，用于测试隔离

	EnableRelay:       false,
	EnableRelayServer: false,

	EnableNAT: false,

	MaxConnections:    10,
	MaxStreamsPerConn: 32,
	IdleTimeout:       30 * time.Second,

	// Realm: 强制内建（测试隔离）
	IsolateDiscovery: true,
	IsolatePubSub:    true,

	// Liveness: 启用，短间隔（快速检测）
	EnableLiveness:    true,
	HeartbeatInterval: 5 * time.Second,
	EnableGoodbye:     true,
}

// ============================================================================
//                              预设查询
// ============================================================================

// AllPresets 返回所有预定义预设
func AllPresets() []*Preset {
	return []*Preset{
		PresetMobile,
		PresetDesktop,
		PresetServer,
		PresetMinimal,
		PresetTest,
	}
}

// GetPresetByName 根据名称获取预设
func GetPresetByName(name string) *Preset {
	for _, p := range AllPresets() {
		if p.Name == name {
			return p
		}
	}
	return nil
}
