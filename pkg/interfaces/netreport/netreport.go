// Package netreport 定义网络诊断接口
//
// 网络诊断模块负责：
// - IPv4/IPv6 连通性检测
// - NAT 类型检测（对称 NAT / 非对称 NAT）
// - 中继延迟测量
// - 诊断报告生成
//
// 参考: iroh 的 NetReport 实现
package netreport

import (
	"context"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              诊断报告
// ============================================================================

// Report 网络诊断报告
//
// Report 包含网络环境的完整诊断信息，用于：
// - 判断节点的网络可达性
// - 选择最佳连接策略
// - 调试网络问题
type Report struct {
	// ======================== 连通性状态 ========================

	// UDPv4 表示 IPv4 UDP 是否可用
	// 通过 STUN 探测确认
	UDPv4 bool

	// UDPv6 表示 IPv6 UDP 是否可用
	// 通过 STUN 探测确认
	UDPv6 bool

	// GlobalV4 检测到的公网 IPv4 地址
	// 如果 UDPv4 为 false，则为 nil
	GlobalV4 net.IP

	// GlobalV4Port 公网 IPv4 端口
	GlobalV4Port uint16

	// GlobalV6 检测到的公网 IPv6 地址
	// 如果 UDPv6 为 false，则为 nil
	GlobalV6 net.IP

	// GlobalV6Port 公网 IPv6 端口
	GlobalV6Port uint16

	// ======================== NAT 类型 ========================

	// NATType 检测到的 NAT 类型
	NATType types.NATType

	// MappingVariesByDestIPv4 表示 IPv4 映射是否随目标变化
	// true 表示对称 NAT，false 表示锥形 NAT
	MappingVariesByDestIPv4 *bool

	// MappingVariesByDestIPv6 表示 IPv6 映射是否随目标变化
	// true 表示对称 NAT，false 表示锥形 NAT
	MappingVariesByDestIPv6 *bool

	// ======================== 中继信息 ========================

	// RelayLatencies 各中继服务器的延迟
	// key: 中继 URL, value: RTT 延迟
	RelayLatencies map[string]time.Duration

	// PreferredRelay 首选中继服务器 URL
	// 基于延迟和可用性选择
	PreferredRelay string

	// ======================== 端口映射 ========================

	// UPnPAvailable 表示 UPnP 是否可用
	UPnPAvailable bool

	// NATPMPAvailable 表示 NAT-PMP 是否可用
	NATPMPAvailable bool

	// PCPAvailable 表示 PCP 是否可用
	PCPAvailable bool

	// ======================== 其他 ========================

	// CaptivePortal 表示是否检测到强制门户
	// nil 表示未知，true 表示存在，false 表示不存在
	CaptivePortal *bool

	// Timestamp 报告生成时间
	Timestamp time.Time

	// Duration 报告生成耗时
	Duration time.Duration
}

// HasUDP 返回是否有任何 UDP 连通性
func (r *Report) HasUDP() bool {
	return r.UDPv4 || r.UDPv6
}

// IsSymmetricNAT 返回是否为对称 NAT
func (r *Report) IsSymmetricNAT() bool {
	return r.NATType == types.NATTypeSymmetric
}

// MappingVariesByDest 返回映射是否随目标变化
func (r *Report) MappingVariesByDest() *bool {
	switch {
	case r.MappingVariesByDestIPv4 != nil && r.MappingVariesByDestIPv6 != nil:
		result := *r.MappingVariesByDestIPv4 || *r.MappingVariesByDestIPv6
		return &result
	case r.MappingVariesByDestIPv4 != nil:
		return r.MappingVariesByDestIPv4
	case r.MappingVariesByDestIPv6 != nil:
		return r.MappingVariesByDestIPv6
	default:
		return nil
	}
}

// HasPortMapping 返回是否有任何端口映射协议可用
func (r *Report) HasPortMapping() bool {
	return r.UPnPAvailable || r.NATPMPAvailable || r.PCPAvailable
}

// BestRelayLatency 返回最佳中继延迟
func (r *Report) BestRelayLatency() time.Duration {
	if r.PreferredRelay == "" {
		return 0
	}
	return r.RelayLatencies[r.PreferredRelay]
}

// ============================================================================
//                              客户端接口
// ============================================================================

// Client 网络诊断客户端接口
type Client interface {
	// GetReport 生成网络诊断报告
	//
	// 执行完整的网络诊断，包括：
	// - IPv4/IPv6 连通性检测
	// - NAT 类型检测
	// - 中继延迟测量
	//
	// 参数：
	//   - ctx: 上下文，用于取消和超时控制
	//
	// 返回：
	//   - *Report: 诊断报告
	//   - error: 如果诊断失败则返回错误
	GetReport(ctx context.Context) (*Report, error)

	// GetReportAsync 异步生成报告
	//
	// 异步执行诊断，返回报告通道。
	// 诊断完成后，报告会发送到通道中。
	//
	// 参数：
	//   - ctx: 上下文，用于取消和超时控制
	//
	// 返回：
	//   - <-chan *Report: 报告通道，完成后关闭
	GetReportAsync(ctx context.Context) <-chan *Report

	// LastReport 返回最后一次诊断报告
	//
	// 如果没有执行过诊断，返回 nil
	LastReport() *Report

	// SetSTUNServers 设置 STUN 服务器列表
	SetSTUNServers(servers []string)

	// SetRelayServers 设置中继服务器列表
	SetRelayServers(relays []string)
}

// ============================================================================
//                              配置
// ============================================================================

// Config 网络诊断配置
type Config struct {
	// STUNServers STUN 服务器列表
	// 用于 IPv4/IPv6 连通性检测和 NAT 类型检测
	STUNServers []string

	// RelayServers 中继服务器列表
	// 用于中继延迟测量
	RelayServers []string

	// Timeout 诊断超时时间
	// 默认 30 秒
	Timeout time.Duration

	// ProbeTimeout 单个探测超时时间
	// 默认 5 秒
	ProbeTimeout time.Duration

	// EnableIPv4 启用 IPv4 探测
	// 默认 true
	EnableIPv4 bool

	// EnableIPv6 启用 IPv6 探测
	// 默认 true
	EnableIPv6 bool

	// EnableRelayProbe 启用中继延迟探测
	// 默认 true
	EnableRelayProbe bool

	// EnablePortMapProbe 启用端口映射协议探测
	// 默认 true
	EnablePortMapProbe bool

	// EnableCaptivePortalProbe 启用强制门户检测
	// 默认 true
	EnableCaptivePortalProbe bool

	// MaxConcurrentProbes 最大并发探测数
	// 默认 10
	MaxConcurrentProbes int

	// FullReportInterval 完整报告间隔
	// 在此时间内的报告可以是增量的
	// 默认 5 分钟
	FullReportInterval time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		STUNServers: []string{
			"stun.l.google.com:19302",
			"stun1.l.google.com:19302",
			"stun2.l.google.com:19302",
			"stun.cloudflare.com:3478",
		},
		RelayServers:             []string{},
		Timeout:                  30 * time.Second,
		ProbeTimeout:             5 * time.Second,
		EnableIPv4:               true,
		EnableIPv6:               true,
		EnableRelayProbe:         true,
		EnablePortMapProbe:       true,
		EnableCaptivePortalProbe: true,
		MaxConcurrentProbes:      10,
		FullReportInterval:       5 * time.Minute,
	}
}

// ============================================================================
//                              探测类型
// ============================================================================

// ProbeType 探测类型
type ProbeType int

const (
	// ProbeTypeIPv4 IPv4 连通性探测
	ProbeTypeIPv4 ProbeType = iota

	// ProbeTypeIPv6 IPv6 连通性探测
	ProbeTypeIPv6

	// ProbeTypeNAT NAT 类型探测
	ProbeTypeNAT

	// ProbeTypeRelay 中继延迟探测
	ProbeTypeRelay

	// ProbeTypePortMap 端口映射协议探测
	ProbeTypePortMap

	// ProbeTypeCaptivePortal 强制门户探测
	ProbeTypeCaptivePortal
)

// String 返回探测类型名称
func (p ProbeType) String() string {
	switch p {
	case ProbeTypeIPv4:
		return "IPv4"
	case ProbeTypeIPv6:
		return "IPv6"
	case ProbeTypeNAT:
		return "NAT"
	case ProbeTypeRelay:
		return "Relay"
	case ProbeTypePortMap:
		return "PortMap"
	case ProbeTypeCaptivePortal:
		return "CaptivePortal"
	default:
		return "Unknown"
	}
}

// ============================================================================
//                              探测结果
// ============================================================================

// ProbeResult 单个探测结果
type ProbeResult struct {
	// Type 探测类型
	Type ProbeType

	// Success 是否成功
	Success bool

	// Latency 探测延迟
	Latency time.Duration

	// Error 错误信息（如果失败）
	Error error

	// Data 探测数据（类型因探测类型而异）
	Data interface{}
}

// IPv4ProbeData IPv4 探测数据
type IPv4ProbeData struct {
	// GlobalIP 公网 IP
	GlobalIP net.IP

	// GlobalPort 公网端口
	GlobalPort uint16

	// Server 响应的 STUN 服务器
	Server string
}

// IPv6ProbeData IPv6 探测数据
type IPv6ProbeData struct {
	// GlobalIP 公网 IP
	GlobalIP net.IP

	// GlobalPort 公网端口
	GlobalPort uint16

	// Server 响应的 STUN 服务器
	Server string
}

// NATProbeData NAT 类型探测数据
type NATProbeData struct {
	// NATType NAT 类型
	NATType types.NATType

	// MappingVaries 映射是否随目标变化
	MappingVaries bool
}

// RelayProbeData 中继探测数据
type RelayProbeData struct {
	// URL 中继 URL
	URL string

	// Latency 延迟
	Latency time.Duration
}

// PortMapProbeData 端口映射探测数据
type PortMapProbeData struct {
	// UPnP UPnP 可用
	UPnP bool

	// NATPMP NAT-PMP 可用
	NATPMP bool

	// PCP PCP 可用
	PCP bool
}

// CaptivePortalProbeData 强制门户探测数据
type CaptivePortalProbeData struct {
	// Detected 是否检测到强制门户
	Detected bool
}

