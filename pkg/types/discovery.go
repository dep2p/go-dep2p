package types

import "time"

// ============================================================================
//                              PeerInfo - 节点信息
// ============================================================================

// PeerInfo 节点信息
// 用于发现服务返回的节点信息
type PeerInfo struct {
	// ID 节点 ID
	ID NodeID

	// Addrs 地址列表（Multiaddr 格式）
	//
	// 注意：此字段类型已从 []string 升级为 []Multiaddr
	// 如需字符串格式，使用 AddrsToStrings() 方法
	Addrs []Multiaddr

	// Source 发现来源（如 "dht", "mdns", "bootstrap"）
	Source string

	// DiscoveredAt 发现时间
	DiscoveredAt time.Time
}

// HasAddrs 检查是否有地址
func (pi PeerInfo) HasAddrs() bool {
	return len(pi.Addrs) > 0
}

// IsExpired 检查是否过期（基于发现时间和 TTL）
func (pi PeerInfo) IsExpired(ttl time.Duration) bool {
	return time.Since(pi.DiscoveredAt) > ttl
}

// AddrsToStrings 返回地址的字符串切片（用于兼容旧 API）
func (pi PeerInfo) AddrsToStrings() []string {
	strs := make([]string, len(pi.Addrs))
	for i, ma := range pi.Addrs {
		strs[i] = ma.String()
	}
	return strs
}

// NewPeerInfo 创建 PeerInfo
func NewPeerInfo(id NodeID, addrs []Multiaddr) PeerInfo {
	return PeerInfo{
		ID:           id,
		Addrs:        addrs,
		DiscoveredAt: time.Now(),
	}
}

// NewPeerInfoFromStrings 从字符串地址创建 PeerInfo
//
// 忽略无法解析的地址。
func NewPeerInfoFromStrings(id NodeID, addrStrs []string) PeerInfo {
	addrs := make([]Multiaddr, 0, len(addrStrs))
	for _, s := range addrStrs {
		ma, err := ParseMultiaddr(s)
		if err == nil {
			addrs = append(addrs, ma)
		}
	}
	return PeerInfo{
		ID:           id,
		Addrs:        addrs,
		DiscoveredAt: time.Now(),
	}
}

