// Package netreport 提供网络诊断模块的实现
package netreport

import (
	"net"
	"sync"
	"time"

	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ReportBuilder 报告构建器
//
// 用于并发安全地构建诊断报告
type ReportBuilder struct {
	mu     sync.RWMutex
	report *netreportif.Report

	// 用于 NAT 类型检测的多服务器结果
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
		report: &netreportif.Report{
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
		// 比较 IP 和端口
		if !first.ip.Equal(m.ip) || first.port != m.port {
			return true
		}
	}
	return false
}

// SetNATType 设置 NAT 类型
func (b *ReportBuilder) SetNATType(natType types.NATType) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.report.NATType = natType
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
func (b *ReportBuilder) Build() *netreportif.Report {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 根据映射结果推断 NAT 类型
	if b.report.NATType == types.NATTypeUnknown {
		b.inferNATType()
	}

	return b.report
}

// inferNATType 根据映射结果推断 NAT 类型
func (b *ReportBuilder) inferNATType() {
	// 检查是否有 UDP 连通性
	if !b.report.UDPv4 && !b.report.UDPv6 {
		b.report.NATType = types.NATTypeUnknown
		return
	}

	// 检查映射是否变化
	v4Varies := b.report.MappingVariesByDestIPv4 != nil && *b.report.MappingVariesByDestIPv4
	v6Varies := b.report.MappingVariesByDestIPv6 != nil && *b.report.MappingVariesByDestIPv6

	if v4Varies || v6Varies {
		b.report.NATType = types.NATTypeSymmetric
	} else if b.report.UDPv4 || b.report.UDPv6 {
		// 有 UDP 连通性但映射不变化，可能是锥形 NAT
		// 需要更多探测才能确定具体类型
		b.report.NATType = types.NATTypeFull
	}
}

// Snapshot 获取当前报告快照
func (b *ReportBuilder) Snapshot() *netreportif.Report {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// 创建副本
	snapshot := *b.report

	// 复制 map
	snapshot.RelayLatencies = make(map[string]time.Duration, len(b.report.RelayLatencies))
	for k, v := range b.report.RelayLatencies {
		snapshot.RelayLatencies[k] = v
	}

	return &snapshot
}

