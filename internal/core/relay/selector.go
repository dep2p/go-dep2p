package relay

import (
	"net"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/relay/geoip"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Selector 中继选择器
//
// 实现智能中继选择策略，包括：
//   - 延迟评分
//   - 容量评分
//   - 可靠性评分
//   - 区域感知评分（需要 GeoIP 解析器）
type Selector struct {
	// geoResolver 可选的 GeoIP 解析器，用于区域感知选路
	geoResolver geoip.Resolver
	// peerstore 可选的 Peerstore，用于获取节点地址
	peerstore pkgif.Peerstore
}

// NewSelector 创建选择器
func NewSelector() *Selector {
	return &Selector{}
}

// SetGeoResolver 设置 GeoIP 解析器（可选）
func (s *Selector) SetGeoResolver(resolver geoip.Resolver) {
	s.geoResolver = resolver
}

// SetPeerstore 设置 Peerstore（可选，用于从节点 ID 获取地址）
func (s *Selector) SetPeerstore(ps pkgif.Peerstore) {
	s.peerstore = ps
}

// SelectBest 选择最佳中继
func (s *Selector) SelectBest(relays []RelayInfo, target string) RelayInfo {
	if len(relays) == 0 {
		return RelayInfo{}
	}
	
	var best RelayInfo
	var bestScore int
	
	for _, relay := range relays {
		score := s.calculateScore(relay, target)
		if score > bestScore {
			bestScore = score
			best = relay
		}
	}
	
	return best
}

// calculateScore 计算中继评分
func (s *Selector) calculateScore(relay RelayInfo, target string) int {
	score := 100 // 基础分
	
	// 延迟评分（延迟越低，分数越高）
	latency := time.Duration(relay.Latency) * time.Millisecond
	switch {
	case latency < 50*time.Millisecond:
		score += 30
	case latency < 100*time.Millisecond:
		score += 20
	case latency < 200*time.Millisecond:
		score += 10
	}
	
	// 容量评分（0-1 → 0-20分）
	score += int(relay.Capacity * 20)
	
	// 可靠性评分（0-1 → 0-20分）
	score += int(relay.Reliability * 20)
	
	// 区域感知评分（如果启用了 GeoIP）
	if s.geoResolver != nil && s.geoResolver.IsAvailable() {
		targetRegion := s.getRegion(target)
		relayRegion := s.getRegionFromRelay(relay)
		if targetRegion != "" && relayRegion != "" && targetRegion == relayRegion {
			score += 15 // 同区域加分
		}
	}
	
	return score
}

// getRegion 从地址字符串提取区域信息
func (s *Selector) getRegion(addr string) string {
	if s.geoResolver == nil || !s.geoResolver.IsAvailable() {
		return ""
	}
	
	// 从地址字符串提取 IP
	ip := s.extractIP(addr)
	if ip == nil {
		return ""
	}
	
	info, err := s.geoResolver.Lookup(ip)
	if err != nil || info == nil {
		return ""
	}
	
	return info.ToRegionString()
}

// getRegionFromRelay 从 RelayInfo 获取区域
//
// 优先使用 RelayInfo 中的 Addrs 字段，如果没有则尝试从 Peerstore 获取。
func (s *Selector) getRegionFromRelay(relay RelayInfo) string {
	// 1. 优先使用 RelayInfo 中的地址
	if len(relay.Addrs) > 0 {
		for _, addr := range relay.Addrs {
			region := s.getRegion(addr)
			if region != "" {
				return region
			}
		}
	}
	
	// 2. 尝试从 Peerstore 获取地址
	if s.peerstore != nil && relay.ID != "" {
		addrs := s.peerstore.Addrs(types.PeerID(relay.ID))
		for _, addr := range addrs {
			if addr == nil {
				continue
			}
			region := s.getRegion(addr.String())
			if region != "" {
				return region
			}
		}
	}
	
	return ""
}

// extractIP 从地址字符串提取 IP
func (s *Selector) extractIP(addr string) net.IP {
	// 处理 multiaddr 格式：/ip4/1.2.3.4/tcp/4001
	parts := strings.Split(addr, "/")
	for i, part := range parts {
		if part == "ip4" && i+1 < len(parts) {
			ip := net.ParseIP(parts[i+1])
			if ip != nil {
				return ip
			}
		}
		if part == "ip6" && i+1 < len(parts) {
			ip := net.ParseIP(parts[i+1])
			if ip != nil {
				return ip
			}
		}
	}
	
	// 尝试直接解析为 IP
	return net.ParseIP(addr)
}

// SelectMultiple 选择多个中继（用于备份）
func (s *Selector) SelectMultiple(relays []RelayInfo, target string, count int) []RelayInfo {
	if len(relays) == 0 || count <= 0 {
		return nil
	}
	
	// 计算所有中继的评分
	type scored struct {
		relay RelayInfo
		score int
	}
	
	scored_relays := make([]scored, len(relays))
	for i, relay := range relays {
		scored_relays[i] = scored{
			relay: relay,
			score: s.calculateScore(relay, target),
		}
	}
	
	// 简单排序（冒泡排序）
	for i := 0; i < len(scored_relays); i++ {
		for j := i + 1; j < len(scored_relays); j++ {
			if scored_relays[j].score > scored_relays[i].score {
				scored_relays[i], scored_relays[j] = scored_relays[j], scored_relays[i]
			}
		}
	}
	
	// 返回前 N 个
	result := make([]RelayInfo, 0, count)
	for i := 0; i < count && i < len(scored_relays); i++ {
		result = append(result, scored_relays[i].relay)
	}
	
	return result
}
