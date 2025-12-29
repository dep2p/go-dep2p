// Package nat 定义 NAT 穿透相关接口
//
// NAT 模块负责网络地址转换的检测和穿透，包括：
// - NAT 类型检测
// - 外部地址发现
// - 端口映射
// - 打洞
package nat

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              NATService 接口
// ============================================================================

// NATService NAT 服务接口
//
// 提供 NAT 检测、地址发现和端口映射功能。
type NATService interface {
	// GetExternalAddress 获取外部地址
	//
	// 通过 STUN、HTTP、UPnP 等方式发现公网地址。
	GetExternalAddress() (endpoint.Address, error)

	// GetExternalAddressWithContext 带上下文的外部地址获取
	GetExternalAddressWithContext(ctx context.Context) (endpoint.Address, error)

	// GetExternalAddressFromPortMapperWithContext 仅从端口映射器获取外部地址（只发布可验证可达地址）
	//
	// 语义：
	// - 仅使用 UPnP/NAT-PMP 等 PortMapper 提供的外部地址来源
	// - 不使用 HTTP 等"仅外部IP参考"的发现器
	//
	// 用途：
	// - 只发布可验证可达地址：只有当存在端口映射能力并能从映射器拿到外部地址时，才生成可拨号的公网 multiaddr
	// - 这是"可达性优先"策略的一部分：确保发布的直连地址真正可达
	GetExternalAddressFromPortMapperWithContext(ctx context.Context) (endpoint.Address, error)

	// NATType 返回 NAT 类型
	NATType() types.NATType

	// DetectNATType 检测 NAT 类型
	DetectNATType(ctx context.Context) (types.NATType, error)

	// MapPort 映射端口
	//
	// 在 NAT 设备上创建端口映射。
	// protocol: "udp" 或 "tcp"
	// internalPort: 内部端口
	// externalPort: 期望的外部端口（0 表示自动分配）
	// duration: 映射有效期
	MapPort(protocol string, internalPort, externalPort int, duration time.Duration) error

	// UnmapPort 取消端口映射
	UnmapPort(protocol string, externalPort int) error

	// GetMappedPort 获取已映射的端口
	GetMappedPort(protocol string, internalPort int) (int, error)

	// Refresh 刷新所有映射
	Refresh(ctx context.Context) error

	// Close 关闭服务
	Close() error
}

// ============================================================================
//                              HolePuncher 接口
// ============================================================================

// HolePuncher 打洞器接口
//
// 用于 NAT 穿透，建立直接连接。
type HolePuncher interface {
	// Punch 尝试 UDP 打洞连接
	//
	// 通过协调双方同时发送数据包来穿透 NAT。
	Punch(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (endpoint.Address, error)

	// StartRendezvous 启动打洞协调
	//
	// 与协调服务器通信，协调打洞过程。
	StartRendezvous(ctx context.Context, remoteID types.NodeID) error
}

// ============================================================================
//                              TCPHolePuncher 接口
// ============================================================================

// TCPHolePuncher TCP 打洞器接口
//
// 使用 TCP Simultaneous Open 技术穿透 NAT。
type TCPHolePuncher interface {
	// PunchTCP 尝试 TCP 打洞连接
	//
	// 使用 SO_REUSEADDR/SO_REUSEPORT 和 Simultaneous Open 技术。
	// 返回建立的 TCP 连接和成功的远程地址。
	PunchTCP(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (conn interface{}, addr endpoint.Address, err error)

	// PunchTCPWithLocalPort 使用指定本地端口尝试 TCP 打洞
	//
	// 当需要与对端约定使用相同端口时使用。
	PunchTCPWithLocalPort(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address, localPort int) (conn interface{}, addr endpoint.Address, err error)
}

// ============================================================================
//                              PortMapper 接口
// ============================================================================

// PortMapper 端口映射器接口
type PortMapper interface {
	// Name 返回映射器名称
	// 如 "upnp", "nat-pmp", "pcp"
	Name() string

	// Available 检查映射器是否可用
	Available() bool

	// GetExternalAddress 获取外部地址
	GetExternalAddress() (endpoint.Address, error)

	// AddMapping 添加端口映射
	AddMapping(protocol string, internalPort int, description string, duration time.Duration) (int, error)

	// DeleteMapping 删除端口映射
	DeleteMapping(protocol string, externalPort int) error

	// GetMapping 获取端口映射
	GetMapping(protocol string, externalPort int) (*Mapping, error)
}

// Mapping 端口映射信息（类型别名，实际定义在 types 包）
// 注意：ExternalAddr 字段使用 string 格式
type Mapping = types.Mapping

// ============================================================================
//                              IPDiscoverer 接口
// ============================================================================

// IPDiscoverer IP 发现器接口
type IPDiscoverer interface {
	// Name 返回发现器名称
	// 如 "stun", "http", "upnp"
	Name() string

	// Discover 发现外部 IP
	Discover(ctx context.Context) (endpoint.Address, error)

	// Priority 返回优先级（数字越小优先级越高）
	Priority() int
}

// ============================================================================
//                              STUN 客户端
// ============================================================================

// STUNClient STUN 客户端接口
type STUNClient interface {
	// GetMappedAddress 获取映射地址
	GetMappedAddress(ctx context.Context) (endpoint.Address, error)

	// GetNATType 通过 STUN 检测 NAT 类型
	GetNATType(ctx context.Context) (types.NATType, error)

	// Close 关闭客户端
	Close() error
}

// ============================================================================
//                              配置
// ============================================================================

// PolicyMode NAT 自适应策略模式
//
// Layer1 修复：根据 Reachability 状态动态调整 NAT 探测/映射行为
type PolicyMode int

const (
	// PolicyModeStatic 静态模式（默认）
	// 按配置的静态开关运行，不根据 Reachability 状态调整
	PolicyModeStatic PolicyMode = iota

	// PolicyModeAdaptive 自适应模式（推荐）
	// 根据 Reachability 状态动态启停探测/映射：
	// - VerifiedDirect: 降频/关闭外部探测
	// - 非 VerifiedDirect: 按阶梯启用
	PolicyModeAdaptive

	// PolicyModePrivacy 隐私模式
	// 禁用所有外部 HTTP IP 服务、仅使用必要的 STUN
	// 适用于合规/隐私敏感场景
	PolicyModePrivacy
)

// String 返回策略模式的字符串表示
func (m PolicyMode) String() string {
	switch m {
	case PolicyModeStatic:
		return "static"
	case PolicyModeAdaptive:
		return "adaptive"
	case PolicyModePrivacy:
		return "privacy"
	default:
		return "unknown"
	}
}

// Config NAT 模块配置
type Config struct {
	// PolicyMode 策略模式（默认 PolicyModeAdaptive）
	// Layer1 修复：自适应策略，根据 Reachability 状态动态调整
	PolicyMode PolicyMode

	// EnableUPnP 启用 UPnP
	// 注意：在自适应模式下，VerifiedDirect 状态会自动关闭
	EnableUPnP bool

	// EnableNATPMP 启用 NAT-PMP
	// 注意：在自适应模式下，VerifiedDirect 状态会自动关闭
	EnableNATPMP bool

	// EnableSTUN 启用 STUN
	EnableSTUN bool

	// STUNServers STUN 服务器列表
	STUNServers []string

	// EnableHTTPIPServices 启用 HTTP 外网 IP 发现
	// Layer1 修复：默认禁用，因为会向第三方服务泄露外网出口信息
	// 仅在自适应模式下、非 VerifiedDirect 状态时作为最后手段启用
	EnableHTTPIPServices bool

	// HTTPIPServices HTTP IP 发现服务
	// 注意：这些是第三方服务，会泄露你的公网 IP 到这些服务
	HTTPIPServices []string

	// MappingRefreshInterval 映射刷新间隔
	MappingRefreshInterval time.Duration

	// MappingDuration 映射有效期
	MappingDuration time.Duration

	// PunchTimeout 打洞超时
	PunchTimeout time.Duration

	// AdaptivePolicy 自适应策略配置（仅在 PolicyModeAdaptive 时生效）
	AdaptivePolicy AdaptivePolicyConfig
}

// AdaptivePolicyConfig 自适应策略配置
//
// Layer1 修复：定义各阶梯的启用条件、冷却时间、重试上限
type AdaptivePolicyConfig struct {
	// STUNIntervalVerifiedDirect VerifiedDirect 状态下的 STUN 检查间隔（降频）
	// 默认 30 分钟，因为已有可达直连地址，仅需周期性健康检查
	STUNIntervalVerifiedDirect time.Duration

	// STUNIntervalNotVerified 非 VerifiedDirect 状态下的 STUN 检查间隔
	// 默认 5 分钟，更频繁探测以尽快发现外网地址
	STUNIntervalNotVerified time.Duration

	// PortMappingCooldown 端口映射冷却时间（失败后重试间隔）
	// 避免频繁访问可能不稳定的 UPnP/NAT-PMP 设备
	PortMappingCooldown time.Duration

	// PortMappingMaxRetries 端口映射最大重试次数
	PortMappingMaxRetries int

	// HTTPIPServicesCooldown HTTP IP 服务冷却时间
	// 避免频繁访问第三方服务
	HTTPIPServicesCooldown time.Duration

	// HTTPIPServicesMaxRetries HTTP IP 服务最大重试次数
	HTTPIPServicesMaxRetries int

	// DisableExternalOnVerified VerifiedDirect 状态下禁用所有外部探测
	// 默认 true，减少不必要的外部访问
	DisableExternalOnVerified bool
}

// DefaultAdaptivePolicyConfig 返回默认的自适应策略配置
func DefaultAdaptivePolicyConfig() AdaptivePolicyConfig {
	return AdaptivePolicyConfig{
		STUNIntervalVerifiedDirect: 30 * time.Minute,
		STUNIntervalNotVerified:    5 * time.Minute,
		PortMappingCooldown:           10 * time.Minute,
		PortMappingMaxRetries:         3,
		HTTPIPServicesCooldown:        30 * time.Minute,
		HTTPIPServicesMaxRetries:      2,
		DisableExternalOnVerified:     true,
	}
}

// DefaultConfig 返回默认配置
//
// Layer1 修复：
// - 默认使用自适应策略模式
// - 默认禁用 HTTP IP 服务（隐私/合规考虑）
// - 保留 STUN 用于 NAT 类型检测
// - UPnP/NAT-PMP 默认启用但受自适应策略控制
func DefaultConfig() Config {
	return Config{
		PolicyMode:           PolicyModeAdaptive, // Layer1: 默认自适应
		EnableUPnP:           true,
		EnableNATPMP:         true,
		EnableSTUN:           true,
		EnableHTTPIPServices: false, // Layer1: 默认禁用，保护隐私
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
			"stun:stun1.l.google.com:19302",
		},
		HTTPIPServices: []string{
			// 空列表：不会访问任何第三方 HTTP IP 服务
			// 如果用户显式启用并需要，可配置：
			// "https://api.ipify.org",
			// "https://ifconfig.me/ip",
		},
		MappingRefreshInterval: 20 * time.Minute,
		MappingDuration:        30 * time.Minute,
		PunchTimeout:           30 * time.Second,
		AdaptivePolicy:         DefaultAdaptivePolicyConfig(),
	}
}

// PrivacyConfig 返回隐私模式配置
//
// 适用于合规/隐私敏感场景：
// - 禁用所有 HTTP IP 服务
// - 禁用 UPnP/NAT-PMP（避免修改路由器配置）
// - 仅保留 STUN 用于基本 NAT 检测
func PrivacyConfig() Config {
	return Config{
		PolicyMode:           PolicyModePrivacy,
		EnableUPnP:           false,
		EnableNATPMP:         false,
		EnableSTUN:           true,
		EnableHTTPIPServices: false,
		STUNServers: []string{
			"stun:stun.l.google.com:19302",
		},
		HTTPIPServices:         []string{}, // 空列表
		MappingRefreshInterval: 30 * time.Minute,
		MappingDuration:        30 * time.Minute,
		PunchTimeout:           30 * time.Second,
		AdaptivePolicy:         DefaultAdaptivePolicyConfig(),
	}
}
