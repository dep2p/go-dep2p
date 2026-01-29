// Package types 定义 DeP2P 的基础类型
//
// 本文件定义节点发现相关类型。
package types

import (
	"fmt"
	"time"
)

// ============================================================================
//                              PeerInfo - 节点信息
// ============================================================================

// PeerInfo 节点信息
//
// 用于发现服务返回的节点信息，包含节点 ID 和地址列表。
type PeerInfo struct {
	// ID 节点 ID
	ID PeerID

	// Addrs 地址列表（Multiaddr 格式）
	Addrs []Multiaddr

	// Source 发现来源（如 "dht", "mdns", "bootstrap"）
	Source DiscoverySource

	// DiscoveredAt 发现时间
	DiscoveredAt time.Time
}

// String 返回 PeerInfo 的字符串表示
func (pi PeerInfo) String() string {
	return fmt.Sprintf("{%s: %v}", pi.ID.ShortString(), pi.Addrs)
}

// HasAddrs 检查是否有地址
func (pi PeerInfo) HasAddrs() bool {
	return len(pi.Addrs) > 0
}

// IsExpired 检查是否过期（基于发现时间和 TTL）
func (pi PeerInfo) IsExpired(ttl time.Duration) bool {
	return time.Since(pi.DiscoveredAt) > ttl
}

// AddrsToStrings 返回地址的字符串切片
func (pi PeerInfo) AddrsToStrings() []string {
	strs := make([]string, len(pi.Addrs))
	for i, ma := range pi.Addrs {
		strs[i] = ma.String()
	}
	return strs
}

// NewPeerInfo 创建 PeerInfo
func NewPeerInfo(id PeerID, addrs []Multiaddr) PeerInfo {
	return PeerInfo{
		ID:           id,
		Addrs:        addrs,
		DiscoveredAt: time.Now(),
	}
}

// NewPeerInfoWithSource 创建带来源的 PeerInfo
func NewPeerInfoWithSource(id PeerID, addrs []Multiaddr, source DiscoverySource) PeerInfo {
	return PeerInfo{
		ID:           id,
		Addrs:        addrs,
		Source:       source,
		DiscoveredAt: time.Now(),
	}
}

// NewPeerInfoFromStrings 从字符串地址创建 PeerInfo
//
// 忽略无法解析的地址。
func NewPeerInfoFromStrings(id PeerID, addrStrs []string) PeerInfo {
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

// ============================================================================
//                              AddrInfo - 地址信息
// ============================================================================

// AddrInfo 节点地址信息
//
// 与 PeerInfo 类似，但不包含发现元数据。
// 用于简单的节点地址表示。
type AddrInfo struct {
	// ID 节点 ID
	ID PeerID

	// Addrs 地址列表
	Addrs []Multiaddr
}

// String 返回 AddrInfo 的字符串表示
func (ai AddrInfo) String() string {
	return fmt.Sprintf("{%s: %v}", ai.ID.ShortString(), ai.Addrs)
}

// HasAddrs 检查是否有地址
func (ai AddrInfo) HasAddrs() bool {
	return len(ai.Addrs) > 0
}

// ToPeerInfo 转换为 PeerInfo
func (ai AddrInfo) ToPeerInfo() PeerInfo {
	return PeerInfo{
		ID:           ai.ID,
		Addrs:        ai.Addrs,
		DiscoveredAt: time.Now(),
	}
}

// NewAddrInfo 创建 AddrInfo
func NewAddrInfo(id PeerID, addrs []Multiaddr) AddrInfo {
	return AddrInfo{
		ID:    id,
		Addrs: addrs,
	}
}

// AddrInfoFromString 从多地址字符串解析 AddrInfo
//
// 多地址格式：/ip4/1.2.3.4/tcp/4001/p2p/QmYyQ...
func AddrInfoFromString(s string) (*AddrInfo, error) {
	ma, err := ParseMultiaddr(s)
	if err != nil {
		return nil, err
	}
	return AddrInfoFromP2pAddr(ma)
}

// AddrInfoFromP2pAddr 从 P2P 多地址解析 AddrInfo
func AddrInfoFromP2pAddr(m Multiaddr) (*AddrInfo, error) {
	if IsEmpty(m) {
		return nil, ErrNoAddresses
	}

	// 提取 PeerID（从 /p2p/<peerID> 组件）
	peerIDStr, err := ValueForProtocolName(m, "p2p")
	if err != nil {
		return nil, ErrInvalidPeerID
	}

	peerID, err := ParsePeerID(peerIDStr)
	if err != nil {
		return nil, err
	}

	// 提取传输地址（移除 /p2p/<peerID> 部分）
	transport := m.Decapsulate(P2PMultiaddr(peerID))

	info := &AddrInfo{ID: peerID}
	if !IsEmpty(transport) {
		info.Addrs = []Multiaddr{transport}
	}

	return info, nil
}

// AddrInfoToP2pAddrs 将 AddrInfo 转换为完整的 P2P 多地址列表
func AddrInfoToP2pAddrs(ai *AddrInfo) ([]Multiaddr, error) {
	p2pPart := P2PMultiaddr(ai.ID)

	if len(ai.Addrs) == 0 {
		return []Multiaddr{p2pPart}, nil
	}

	addrs := make([]Multiaddr, 0, len(ai.Addrs))
	for _, addr := range ai.Addrs {
		full := addr.Encapsulate(p2pPart)
		addrs = append(addrs, full)
	}

	return addrs, nil
}

// AddrInfosFromP2pAddrs 从多个 P2P 多地址解析 AddrInfo 列表
func AddrInfosFromP2pAddrs(mas ...Multiaddr) ([]AddrInfo, error) {
	// 按 PeerID 分组
	peerMap := make(map[PeerID]*AddrInfo)

	for _, ma := range mas {
		info, err := AddrInfoFromP2pAddr(ma)
		if err != nil {
			// 跳过无效地址
			continue
		}

		if existing, ok := peerMap[info.ID]; ok {
			// 合并地址
			existing.Addrs = append(existing.Addrs, info.Addrs...)
		} else {
			peerMap[info.ID] = info
		}
	}

	// 转换为切片
	infos := make([]AddrInfo, 0, len(peerMap))
	for _, info := range peerMap {
		infos = append(infos, *info)
	}

	return infos, nil
}

// ============================================================================
//                              辅助函数
// ============================================================================

// PeerInfoSlice 用于排序的 PeerInfo 切片
type PeerInfoSlice []PeerInfo

func (s PeerInfoSlice) Len() int           { return len(s) }
func (s PeerInfoSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s PeerInfoSlice) Less(i, j int) bool { return string(s[i].ID) < string(s[j].ID) }

// AddrInfoSlice 用于排序的 AddrInfo 切片
type AddrInfoSlice []AddrInfo

func (s AddrInfoSlice) Len() int           { return len(s) }
func (s AddrInfoSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s AddrInfoSlice) Less(i, j int) bool { return string(s[i].ID) < string(s[j].ID) }

// ExtractPeerIDs 从 PeerInfo 切片提取 PeerID
func ExtractPeerIDs(infos []PeerInfo) []PeerID {
	ids := make([]PeerID, len(infos))
	for i, info := range infos {
		ids[i] = info.ID
	}
	return ids
}

// ExtractAddrInfoIDs 从 AddrInfo 切片提取 PeerID
func ExtractAddrInfoIDs(infos []AddrInfo) []PeerID {
	ids := make([]PeerID, len(infos))
	for i, info := range infos {
		ids[i] = info.ID
	}
	return ids
}
