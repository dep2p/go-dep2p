// Package address 提供地址管理模块的实现
package address

import (
	"net"
	"sort"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// ============================================================================
//                              地址状态
// ============================================================================

// AddressState 地址状态
type AddressState int

const (
	// AddressStateUnknown 未知状态 - 地址刚添加，未验证
	AddressStateUnknown AddressState = iota

	// AddressStatePending 待验证 - 正在验证可达性
	AddressStatePending

	// AddressStateAvailable 可用 - 验证成功，可正常使用
	AddressStateAvailable

	// AddressStateDegraded 降级 - 可用但性能下降（如 RTT 高）
	AddressStateDegraded

	// AddressStateUnreachable 不可达 - 多次验证失败
	AddressStateUnreachable

	// AddressStateInvalid 无效 - 地址格式错误或签名无效
	AddressStateInvalid
)

// String 返回状态的字符串表示
func (s AddressState) String() string {
	switch s {
	case AddressStateUnknown:
		return "unknown"
	case AddressStatePending:
		return "pending"
	case AddressStateAvailable:
		return "available"
	case AddressStateDegraded:
		return "degraded"
	case AddressStateUnreachable:
		return "unreachable"
	case AddressStateInvalid:
		return "invalid"
	default:
		return "unknown"
	}
}

// IsUsable 地址是否可用于连接
func (s AddressState) IsUsable() bool {
	return s == AddressStateAvailable || s == AddressStateDegraded || s == AddressStateUnknown
}

// ============================================================================
//                              地址类型
// ============================================================================

// AddressType 地址类型（用于优先级计算）
type AddressType int

const (
	// AddressTypePublic 公网地址 - 直接可达
	AddressTypePublic AddressType = iota

	// AddressTypeLAN 局域网地址 - 局域网内可达
	AddressTypeLAN

	// AddressTypeNATMapped NAT 映射地址 - 需要 NAT 穿透
	AddressTypeNATMapped

	// AddressTypeRelay 中继地址 - 通过中继服务器到达
	AddressTypeRelay
)

// BasePriority 返回地址类型的基础优先级
func (t AddressType) BasePriority() int {
	switch t {
	case AddressTypePublic:
		return 80
	case AddressTypeLAN:
		return 70
	case AddressTypeNATMapped:
		return 60
	case AddressTypeRelay:
		return 40
	default:
		return 50
	}
}

// String 返回类型的字符串表示
func (t AddressType) String() string {
	switch t {
	case AddressTypePublic:
		return "public"
	case AddressTypeLAN:
		return "lan"
	case AddressTypeNATMapped:
		return "nat-mapped"
	case AddressTypeRelay:
		return "relay"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              地址来源
// ============================================================================

// AddressSource 地址来源
type AddressSource int

const (
	// AddressSourceDirect 直接连接获取
	AddressSourceDirect AddressSource = iota

	// AddressSourceDHT DHT 发现
	AddressSourceDHT

	// AddressSourceNeighbor 邻居通知
	AddressSourceNeighbor

	// AddressSourceMDNS mDNS 本地发现
	AddressSourceMDNS

	// AddressSourceManual 手动配置
	AddressSourceManual
)

// Priority 返回来源优先级（用于冲突解决）
func (s AddressSource) Priority() int {
	switch s {
	case AddressSourceDirect:
		return 5
	case AddressSourceDHT:
		return 4
	case AddressSourceNeighbor:
		return 3
	case AddressSourceMDNS:
		return 2
	case AddressSourceManual:
		return 1
	default:
		return 0
	}
}

// String 返回来源的字符串表示
func (s AddressSource) String() string {
	switch s {
	case AddressSourceDirect:
		return "direct"
	case AddressSourceDHT:
		return "dht"
	case AddressSourceNeighbor:
		return "neighbor"
	case AddressSourceMDNS:
		return "mdns"
	case AddressSourceManual:
		return "manual"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              地址统计
// ============================================================================

// AddressStats 地址统计信息
type AddressStats struct {
	// SuccessCount 成功连接次数
	SuccessCount int

	// FailCount 失败连接次数
	FailCount int

	// ConsecutiveFails 连续失败次数
	ConsecutiveFails int

	// LastSuccess 最后成功时间
	LastSuccess time.Time

	// LastFail 最后失败时间
	LastFail time.Time

	// LastAttempt 最后尝试时间
	LastAttempt time.Time

	// AvgRTT 平均 RTT
	AvgRTT time.Duration

	// LastRTT 最后一次 RTT
	LastRTT time.Duration
}

// RecordSuccess 记录连接成功
func (s *AddressStats) RecordSuccess(rtt time.Duration) {
	s.SuccessCount++
	s.ConsecutiveFails = 0
	s.LastSuccess = time.Now()
	s.LastAttempt = s.LastSuccess
	s.LastRTT = rtt

	// 更新平均 RTT (指数移动平均)
	if s.AvgRTT == 0 {
		s.AvgRTT = rtt
	} else {
		s.AvgRTT = (s.AvgRTT*7 + rtt*3) / 10
	}
}

// RecordFail 记录连接失败
func (s *AddressStats) RecordFail() {
	s.FailCount++
	s.ConsecutiveFails++
	s.LastFail = time.Now()
	s.LastAttempt = s.LastFail
}

// TotalAttempts 返回总尝试次数
func (s *AddressStats) TotalAttempts() int {
	return s.SuccessCount + s.FailCount
}

// SuccessRate 返回成功率 (0-1)
func (s *AddressStats) SuccessRate() float64 {
	total := s.TotalAttempts()
	if total == 0 {
		return 0
	}
	return float64(s.SuccessCount) / float64(total)
}

// ============================================================================
//                              优先级地址条目
// ============================================================================

// PrioritizedAddress 带优先级的地址条目
type PrioritizedAddress struct {
	// Address 地址
	Address endpoint.Address

	// Type 地址类型
	Type AddressType

	// Source 地址来源
	Source AddressSource

	// State 地址状态
	State AddressState

	// Stats 统计信息
	Stats AddressStats

	// AddedAt 添加时间
	AddedAt time.Time

	// TTL 有效期
	TTL time.Duration

	// Priority 计算的优先级（越大越优先）
	Priority int
}

// NewPrioritizedAddress 创建新的优先级地址条目
func NewPrioritizedAddress(addr endpoint.Address, addrType AddressType, source AddressSource) *PrioritizedAddress {
	pa := &PrioritizedAddress{
		Address: addr,
		Type:    addrType,
		Source:  source,
		State:   AddressStateUnknown,
		AddedAt: time.Now(),
		TTL:     time.Hour, // 默认 1 小时
	}
	pa.recalculatePriority()
	return pa
}

// ============================================================================
//                              优先级计算
// ============================================================================

// recalculatePriority 重新计算优先级
//
// 公式: Priority = BasePriority + SuccessBonus - FailPenalty - RTTFactor
//
// BasePriority: 基于地址类型 (公网 80, 局域网 70, NAT 映射 60, 中继 40)
// SuccessBonus: min(SuccessRate * 20, 20)
// FailPenalty:  min(ConsecutiveFails * 10, 50)
// RTTFactor:    RTT < 50ms: 0, < 100ms: 5, < 200ms: 10, >= 200ms: 20
func (pa *PrioritizedAddress) recalculatePriority() {
	// 基础优先级
	priority := pa.Type.BasePriority()

	// 成功加分
	successBonus := int(pa.Stats.SuccessRate() * 20)
	if successBonus > 20 {
		successBonus = 20
	}
	priority += successBonus

	// 失败减分
	failPenalty := pa.Stats.ConsecutiveFails * 10
	if failPenalty > 50 {
		failPenalty = 50
	}
	priority -= failPenalty

	// RTT 减分
	var rttFactor int
	switch {
	case pa.Stats.AvgRTT == 0:
		rttFactor = 0
	case pa.Stats.AvgRTT < 50*time.Millisecond:
		rttFactor = 0
	case pa.Stats.AvgRTT < 100*time.Millisecond:
		rttFactor = 5
	case pa.Stats.AvgRTT < 200*time.Millisecond:
		rttFactor = 10
	default:
		rttFactor = 20
	}
	priority -= rttFactor

	// 不可用地址优先级归零
	if !pa.State.IsUsable() {
		priority = 0
	}

	pa.Priority = priority
}

// RecordSuccess 记录成功并更新优先级
func (pa *PrioritizedAddress) RecordSuccess(rtt time.Duration) {
	pa.Stats.RecordSuccess(rtt)

	// 更新状态
	if pa.Stats.AvgRTT > 200*time.Millisecond {
		pa.State = AddressStateDegraded
	} else {
		pa.State = AddressStateAvailable
	}

	pa.recalculatePriority()
}

// RecordFail 记录失败并更新优先级
func (pa *PrioritizedAddress) RecordFail() {
	pa.Stats.RecordFail()

	// 更新状态
	if pa.Stats.ConsecutiveFails >= 3 {
		pa.State = AddressStateUnreachable
	}

	pa.recalculatePriority()
}

// IsExpired 检查是否已过期
func (pa *PrioritizedAddress) IsExpired() bool {
	if pa.TTL == 0 {
		return false
	}
	return time.Since(pa.AddedAt) > pa.TTL
}

// ============================================================================
//                              地址排序
// ============================================================================

// SortAddresses 按优先级排序地址（优先级高的在前）
func SortAddresses(addresses []*PrioritizedAddress) {
	sort.Slice(addresses, func(i, j int) bool {
		// 优先级高的在前
		if addresses[i].Priority != addresses[j].Priority {
			return addresses[i].Priority > addresses[j].Priority
		}
		// 优先级相同时，最近成功的在前
		return addresses[i].Stats.LastSuccess.After(addresses[j].Stats.LastSuccess)
	})
}

// FilterUsableAddresses 过滤出可用地址
func FilterUsableAddresses(addresses []*PrioritizedAddress) []*PrioritizedAddress {
	result := make([]*PrioritizedAddress, 0, len(addresses))
	for _, addr := range addresses {
		if addr.State.IsUsable() && !addr.IsExpired() {
			result = append(result, addr)
		}
	}
	return result
}

// SelectBestAddress 选择最佳地址
func SelectBestAddress(addresses []*PrioritizedAddress) *PrioritizedAddress {
	usable := FilterUsableAddresses(addresses)
	if len(usable) == 0 {
		return nil
	}
	SortAddresses(usable)
	return usable[0]
}

// ============================================================================
//                              地址类型检测
// ============================================================================

// DetectAddressType 检测地址类型
//
// 根据地址判断地址类型，支持：
// - Multiaddr 格式：/ip4/127.0.0.1/tcp/8000
// - IP:Port 格式：127.0.0.1:8000
func DetectAddressType(addr endpoint.Address) AddressType {
	addrStr := addr.String()

	// 检测中继地址
	if containsRelay(addrStr) {
		return AddressTypeRelay
	}

	// 尝试从地址中提取 IP
	ip := extractIPFromAddress(addrStr)
	if ip == nil {
		// 无法提取 IP，回退到字符串匹配
		if isPrivateIPString(addrStr) {
			return AddressTypeLAN
		}
		return AddressTypePublic
	}

	// 根据 IP 类型判断
	if ip.IsLoopback() {
		return AddressTypeLAN
	}
	if ip.IsPrivate() {
		return AddressTypeLAN
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return AddressTypeLAN
	}

	return AddressTypePublic
}

// extractIPFromAddress 从地址字符串中提取 IP
func extractIPFromAddress(addrStr string) net.IP {
	// Multiaddr 格式: /ip4/x.x.x.x/... 或 /ip6/xxxx::.../...
	if strings.HasPrefix(addrStr, "/") {
		parts := strings.Split(addrStr, "/")
		for i := 1; i < len(parts)-1; i += 2 {
			if parts[i] == "ip4" || parts[i] == "ip6" {
				if i+1 < len(parts) {
					ip := net.ParseIP(parts[i+1])
					if ip != nil {
						return ip
					}
				}
			}
		}
	}

	// IP:Port 格式或纯 IP
	// 尝试拆分 host:port
	host := addrStr
	if idx := strings.LastIndex(addrStr, ":"); idx != -1 {
		// 检查是否是 IPv6 带方括号
		if strings.HasPrefix(addrStr, "[") {
			if closeBracket := strings.Index(addrStr, "]"); closeBracket != -1 {
				host = addrStr[1:closeBracket]
			}
		} else if strings.Count(addrStr, ":") == 1 {
			// IPv4:Port
			host = addrStr[:idx]
		}
	}

	return net.ParseIP(host)
}

// containsRelay 检查地址是否包含中继组件
func containsRelay(addr string) bool {
	return strings.Contains(addr, "/p2p-circuit/") ||
		strings.Contains(addr, "/relay/") ||
		strings.Contains(addr, "p2p-circuit")
}

// isPrivateIPString 通过字符串匹配检查私有 IP（回退方法）
func isPrivateIPString(addr string) bool {
	// 私有 IP 范围:
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	// 127.0.0.0/8 (localhost)
	return strings.Contains(addr, "/ip4/10.") ||
		strings.Contains(addr, "/ip4/172.16.") || strings.Contains(addr, "/ip4/172.17.") ||
		strings.Contains(addr, "/ip4/172.18.") || strings.Contains(addr, "/ip4/172.19.") ||
		strings.Contains(addr, "/ip4/172.20.") || strings.Contains(addr, "/ip4/172.21.") ||
		strings.Contains(addr, "/ip4/172.22.") || strings.Contains(addr, "/ip4/172.23.") ||
		strings.Contains(addr, "/ip4/172.24.") || strings.Contains(addr, "/ip4/172.25.") ||
		strings.Contains(addr, "/ip4/172.26.") || strings.Contains(addr, "/ip4/172.27.") ||
		strings.Contains(addr, "/ip4/172.28.") || strings.Contains(addr, "/ip4/172.29.") ||
		strings.Contains(addr, "/ip4/172.30.") || strings.Contains(addr, "/ip4/172.31.") ||
		strings.Contains(addr, "/ip4/192.168.") ||
		strings.Contains(addr, "/ip4/127.") ||
		strings.Contains(addr, "10.") ||
		strings.Contains(addr, "192.168.") ||
		strings.Contains(addr, "127.0.0.1") ||
		strings.Contains(addr, "localhost")
}

