// Package netreport 提供网络诊断功能
//
// IMPL-NETWORK-RESILIENCE Phase 6.3: 诊断报告
package netreport

import (
	"net"
	"sync"
	"time"
)

// logger 在 module.go 中定义

// ============================================================================
//                              NAT 类型
// ============================================================================

// NATType NAT 类型
type NATType int

const (
	// NATTypeUnknown 未知 NAT 类型
	NATTypeUnknown NATType = iota

	// NATTypeFull 完全锥形 NAT (Full Cone / EIM)
	// 映射不随目标变化，任何外部主机都可以访问
	NATTypeFull

	// NATTypeRestricted 限制型锥形 NAT
	// 只有被访问过的 IP 可以发送数据
	NATTypeRestricted

	// NATTypePortRestricted 端口限制型锥形 NAT
	// 只有被访问过的 IP:Port 可以发送数据
	NATTypePortRestricted

	// NATTypeSymmetric 对称型 NAT
	// 映射随目标变化，每个目标有不同的映射
	NATTypeSymmetric
)

// String 返回 NAT 类型字符串
func (n NATType) String() string {
	switch n {
	case NATTypeUnknown:
		return "unknown"
	case NATTypeFull:
		return "full_cone"
	case NATTypeRestricted:
		return "restricted"
	case NATTypePortRestricted:
		return "port_restricted"
	case NATTypeSymmetric:
		return "symmetric"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              诊断报告
// ============================================================================

// Report 网络诊断报告
type Report struct {
	// 连通性状态
	UDPv4        bool   // IPv4 UDP 是否可用
	UDPv6        bool   // IPv6 UDP 是否可用
	GlobalV4     net.IP // 公网 IPv4 地址
	GlobalV4Port uint16 // 公网 IPv4 端口
	GlobalV6     net.IP // 公网 IPv6 地址
	GlobalV6Port uint16 // 公网 IPv6 端口

	// NAT 类型
	NATType                 NATType // NAT 类型
	MappingVariesByDestIPv4 *bool   // IPv4 映射是否随目标变化
	MappingVariesByDestIPv6 *bool   // IPv6 映射是否随目标变化

	// 中继信息
	RelayLatencies map[string]time.Duration // 中继延迟
	PreferredRelay string                   // 首选中继

	// 端口映射
	UPnPAvailable   bool // UPnP 可用
	NATPMPAvailable bool // NAT-PMP 可用
	PCPAvailable    bool // PCP 可用

	// 其他
	CaptivePortal *bool         // 是否存在强制门户
	Timestamp     time.Time     // 报告生成时间
	Duration      time.Duration // 报告生成耗时

	// STUN 探测诊断
	STUNIPv4Failed  bool   // IPv4 STUN 探测失败
	STUNIPv6Failed  bool   // IPv6 STUN 探测失败
	STUNFallbackUsed bool  // 是否触发 STUN 兜底探测
	STUNLastError   string // 最后一次 STUN 错误（用于排查）
}

// HasUDP 返回是否有任何 UDP 连通性
func (r *Report) HasUDP() bool {
	return r.UDPv4 || r.UDPv6
}

// IsSymmetricNAT 返回是否为对称 NAT
func (r *Report) IsSymmetricNAT() bool {
	return r.NATType == NATTypeSymmetric
}

// HasPortMapping 返回是否有任何端口映射协议可用
func (r *Report) HasPortMapping() bool {
	return r.UPnPAvailable || r.NATPMPAvailable || r.PCPAvailable
}

// BestRelayLatency 返回最佳中继延迟
func (r *Report) BestRelayLatency() time.Duration {
	if r.PreferredRelay != "" {
		return r.RelayLatencies[r.PreferredRelay]
	}
	return 0
}

// ============================================================================
//                              报告构建器
// ============================================================================

// ReportBuilder 报告构建器
type ReportBuilder struct {
	mu     sync.RWMutex
	report *Report

	// NAT 类型检测的多服务器结果
	ipv4Mappings []mappingResult
	ipv6Mappings []mappingResult
}

// mappingResult 映射结果
type mappingResult struct {
	server string
	ip     net.IP
	port   uint16
}

// NewReportBuilder 创建报告构建器
func NewReportBuilder() *ReportBuilder {
	return &ReportBuilder{
		report: &Report{
			RelayLatencies: make(map[string]time.Duration),
			Timestamp:      time.Now(),
		},
		ipv4Mappings: make([]mappingResult, 0),
		ipv6Mappings: make([]mappingResult, 0),
	}
}

// SetUDPv4 设置 IPv4 UDP 状态
func (b *ReportBuilder) SetUDPv4(available bool, ip net.IP, port uint16) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.report.UDPv4 = available
	if available && ip != nil {
		b.report.GlobalV4 = ip
		b.report.GlobalV4Port = port
	}
}

// SetUDPv6 设置 IPv6 UDP 状态
func (b *ReportBuilder) SetUDPv6(available bool, ip net.IP, port uint16) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.report.UDPv6 = available
	if available && ip != nil {
		b.report.GlobalV6 = ip
		b.report.GlobalV6Port = port
	}
}

// AddIPv4Mapping 添加 IPv4 映射结果
func (b *ReportBuilder) AddIPv4Mapping(server string, ip net.IP, port uint16) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ipv4Mappings = append(b.ipv4Mappings, mappingResult{
		server: server,
		ip:     ip,
		port:   port,
	})

	// 更新 GlobalV4（使用第一个结果）
	if b.report.GlobalV4 == nil {
		b.report.GlobalV4 = ip
		b.report.GlobalV4Port = port
		b.report.UDPv4 = true
	}

	// 检查映射是否变化
	if len(b.ipv4Mappings) >= 2 {
		varies := b.checkMappingVaries(b.ipv4Mappings)
		b.report.MappingVariesByDestIPv4 = &varies
	}
}

// AddIPv6Mapping 添加 IPv6 映射结果
func (b *ReportBuilder) AddIPv6Mapping(server string, ip net.IP, port uint16) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ipv6Mappings = append(b.ipv6Mappings, mappingResult{
		server: server,
		ip:     ip,
		port:   port,
	})

	// 更新 GlobalV6（使用第一个结果）
	if b.report.GlobalV6 == nil {
		b.report.GlobalV6 = ip
		b.report.GlobalV6Port = port
		b.report.UDPv6 = true
	}

	// 检查映射是否变化
	if len(b.ipv6Mappings) >= 2 {
		varies := b.checkMappingVaries(b.ipv6Mappings)
		b.report.MappingVariesByDestIPv6 = &varies
	}
}

// checkMappingVaries 检查映射是否随目标变化
func (b *ReportBuilder) checkMappingVaries(mappings []mappingResult) bool {
	if len(mappings) < 2 {
		return false
	}

	first := mappings[0]
	for _, m := range mappings[1:] {
		if !first.ip.Equal(m.ip) || first.port != m.port {
			return true
		}
	}
	return false
}

// SetNATType 设置 NAT 类型
func (b *ReportBuilder) SetNATType(natType NATType) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.report.NATType = natType
}

// MarkSTUNFailure 标记 STUN 探测失败
func (b *ReportBuilder) MarkSTUNFailure(ipv6 bool, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ipv6 {
		b.report.STUNIPv6Failed = true
	} else {
		b.report.STUNIPv4Failed = true
	}
	if err != nil {
		b.report.STUNLastError = err.Error()
	}
}

// MarkSTUNFallbackUsed 标记触发兜底探测
func (b *ReportBuilder) MarkSTUNFallbackUsed() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.report.STUNFallbackUsed = true
}

// AddRelayLatency 添加中继延迟
func (b *ReportBuilder) AddRelayLatency(url string, latency time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 只保留最低延迟
	if existing, ok := b.report.RelayLatencies[url]; !ok || latency < existing {
		b.report.RelayLatencies[url] = latency
	}

	// 更新首选中继
	b.updatePreferredRelay()
}

// updatePreferredRelay 更新首选中继
func (b *ReportBuilder) updatePreferredRelay() {
	var bestURL string
	var bestLatency time.Duration

	for url, latency := range b.report.RelayLatencies {
		if bestURL == "" || latency < bestLatency {
			bestURL = url
			bestLatency = latency
		}
	}

	b.report.PreferredRelay = bestURL
}

// SetPortMapAvailability 设置端口映射协议可用性
func (b *ReportBuilder) SetPortMapAvailability(upnp, natpmp, pcp bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.report.UPnPAvailable = upnp
	b.report.NATPMPAvailable = natpmp
	b.report.PCPAvailable = pcp
}

// SetCaptivePortal 设置强制门户检测结果
func (b *ReportBuilder) SetCaptivePortal(detected bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.report.CaptivePortal = &detected
}

// SetDuration 设置报告生成耗时
func (b *ReportBuilder) SetDuration(d time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.report.Duration = d
}

// Build 构建最终报告
func (b *ReportBuilder) Build() *Report {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 根据映射结果推断 NAT 类型
	if b.report.NATType == NATTypeUnknown {
		b.inferNATType()
	}

	return b.report
}

// inferNATType 根据映射结果推断 NAT 类型
func (b *ReportBuilder) inferNATType() {
	// 检查是否有 UDP 连通性
	if !b.report.UDPv4 && !b.report.UDPv6 {
		b.report.NATType = NATTypeUnknown
		return
	}

	// 检查映射是否变化
	v4Varies := b.report.MappingVariesByDestIPv4 != nil && *b.report.MappingVariesByDestIPv4
	v6Varies := b.report.MappingVariesByDestIPv6 != nil && *b.report.MappingVariesByDestIPv6

	if v4Varies || v6Varies {
		b.report.NATType = NATTypeSymmetric
	} else if b.report.UDPv4 || b.report.UDPv6 {
		b.report.NATType = NATTypeFull
	}
}
