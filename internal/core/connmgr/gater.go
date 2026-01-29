package connmgr

import (
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Gater 连接门控器
type Gater struct {
	mu             sync.RWMutex
	blocked        map[string]struct{}   // 节点黑名单
	blockedIPs     map[string]struct{}   // IP 黑名单
	blockedSubnets map[string]*net.IPNet // 子网黑名单
	blockedPorts   map[int]struct{}      // 端口黑名单

	// 统计
	interceptedDials   int64
	interceptedAccepts int64
}

var _ pkgif.ConnGater = (*Gater)(nil)

// NewGater 创建连接门控器
func NewGater() *Gater {
	return &Gater{
		blocked:        make(map[string]struct{}),
		blockedIPs:     make(map[string]struct{}),
		blockedSubnets: make(map[string]*net.IPNet),
		blockedPorts:   make(map[int]struct{}),
	}
}

// BlockPeer 阻止节点
func (g *Gater) BlockPeer(peer string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.blocked[peer] = struct{}{}
}

// UnblockPeer 解除节点阻止
func (g *Gater) UnblockPeer(peer string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	delete(g.blocked, peer)
}

// IsBlocked 检查节点是否被阻止
func (g *Gater) IsBlocked(peer string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	_, blocked := g.blocked[peer]
	return blocked
}

// InterceptPeerDial 在拨号前检查是否允许连接到目标节点
// 返回 true 表示允许，false 表示拒绝
func (g *Gater) InterceptPeerDial(peerID string) bool {
	if g.IsBlocked(peerID) {
		atomic.AddInt64(&g.interceptedDials, 1)
		return false
	}
	return true
}

// InterceptAddrDial 在拨号前检查是否允许连接到目标地址
// 返回 true 表示允许，false 表示拒绝
func (g *Gater) InterceptAddrDial(peerID string, addr string) bool {
	// 1. 基于节点 ID 判断
	if g.IsBlocked(peerID) {
		atomic.AddInt64(&g.interceptedDials, 1)
		return false
	}

	// 2. 基于地址的过滤逻辑
	ma, err := types.NewMultiaddr(addr)
	if err != nil {
		// 无效地址，拒绝
		atomic.AddInt64(&g.interceptedDials, 1)
		return false
	}

	// 3. IP 黑名单检查
	if ip, err := ma.ValueForProtocol(types.ProtocolIP4); err == nil {
		if g.isIPBlocked(ip) {
			atomic.AddInt64(&g.interceptedDials, 1)
			return false
		}
	}
	if ip, err := ma.ValueForProtocol(types.ProtocolIP6); err == nil {
		if g.isIPBlocked(ip) {
			atomic.AddInt64(&g.interceptedDials, 1)
			return false
		}
	}

	// 4. 端口过滤
	if portStr, err := ma.ValueForProtocol(types.ProtocolTCP); err == nil {
		if port, err := strconv.Atoi(portStr); err == nil {
			if g.isPortBlocked(port) {
				atomic.AddInt64(&g.interceptedDials, 1)
				return false
			}
		}
	}

	return true
}

// InterceptAccept 在接受连接前检查是否允许
// 返回 true 表示允许，false 表示拒绝
func (g *Gater) InterceptAccept(conn pkgif.Connection) bool {
	// 1. 获取远端地址
	remoteAddr := conn.RemoteMultiaddr()
	if remoteAddr == nil {
		return true
	}

	// 2. IP 限制检查
	if ip, err := remoteAddr.ValueForProtocol(types.ProtocolIP4); err == nil {
		if g.isIPBlocked(ip) {
			atomic.AddInt64(&g.interceptedAccepts, 1)
			return false
		}
	}
	if ip, err := remoteAddr.ValueForProtocol(types.ProtocolIP6); err == nil {
		if g.isIPBlocked(ip) {
			atomic.AddInt64(&g.interceptedAccepts, 1)
			return false
		}
	}

	// 3. 连接数限制和速率限制由 ResourceManager 处理
	// 这里仅做简单过滤

	return true
}

// InterceptSecured 在安全握手后检查是否允许
// 返回 true 表示允许，false 表示拒绝
func (g *Gater) InterceptSecured(_ pkgif.Direction, peerID string, _ pkgif.Connection) bool {
	// 在握手后可以获取到 PeerID，可以做更精确的过滤
	if g.IsBlocked(peerID) {
		atomic.AddInt64(&g.interceptedAccepts, 1)
		return false
	}
	return true
}

// InterceptUpgraded 在连接升级后检查是否允许
// 返回 (true, nil) 表示允许，(false, error) 表示拒绝
func (g *Gater) InterceptUpgraded(conn pkgif.Connection) (bool, error) {
	// 连接升级后的最后检查点
	// 此时连接已完全建立，可以进行最终决策

	// 1. 检查节点是否在黑名单
	remotePeer := string(conn.RemotePeer())
	if g.IsBlocked(remotePeer) {
		return false, nil
	}

	// 2. 节点身份验证结果（已在安全握手中验证）
	// 3. 协议协商结果（由 Protocol Router 处理）
	// 4. 资源配额（由 ResourceManager 处理）

	// 默认允许
	return true, nil
}

// Clear 清空黑名单（用于测试）
func (g *Gater) Clear() {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.blocked = make(map[string]struct{})
	g.blockedIPs = make(map[string]struct{})
	g.blockedSubnets = make(map[string]*net.IPNet)
	g.blockedPorts = make(map[int]struct{})
}

// BlockedPeers 返回所有被阻止的节点列表（用于调试）
func (g *Gater) BlockedPeers() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	peers := make([]string, 0, len(g.blocked))
	for peer := range g.blocked {
		peers = append(peers, peer)
	}
	return peers
}

// ============================================================================
// IP 和端口过滤
// ============================================================================

// BlockIP 阻止 IP 地址
func (g *Gater) BlockIP(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockedIPs[ip] = struct{}{}
}

// UnblockIP 解除 IP 地址阻止
func (g *Gater) UnblockIP(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.blockedIPs, ip)
}

// isIPBlocked 检查 IP 是否被阻止
func (g *Gater) isIPBlocked(ip string) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	
	// 检查完整 IP
	if _, blocked := g.blockedIPs[ip]; blocked {
		return true
	}
	
	// 检查 IP 段（简化实现：检查前缀）
	for blockedIP := range g.blockedIPs {
		if strings.HasPrefix(ip, blockedIP+".") || strings.HasPrefix(ip, blockedIP+":") {
			return true
		}
	}

	// 检查子网（需要解析 IP）
	parsedIP := net.ParseIP(ip)
	if parsedIP != nil {
		for _, ipnet := range g.blockedSubnets {
			if ipnet.Contains(parsedIP) {
				return true
			}
		}
	}
	
	return false
}

// BlockPort 阻止端口
func (g *Gater) BlockPort(port int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockedPorts[port] = struct{}{}
}

// UnblockPort 解除端口阻止
func (g *Gater) UnblockPort(port int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.blockedPorts, port)
}

// isPortBlocked 检查端口是否被阻止
func (g *Gater) isPortBlocked(port int) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, blocked := g.blockedPorts[port]
	return blocked
}

// ============================================================================
// 子网过滤
// ============================================================================

// BlockSubnet 阻止子网
func (g *Gater) BlockSubnet(cidr string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	g.mu.Lock()
	defer g.mu.Unlock()
	g.blockedSubnets[cidr] = ipnet
	return nil
}

// UnblockSubnet 解除子网阻止
func (g *Gater) UnblockSubnet(cidr string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.blockedSubnets, cidr)
}

// BlockedSubnets 返回所有被阻止的子网
func (g *Gater) BlockedSubnets() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	subnets := make([]string, 0, len(g.blockedSubnets))
	for cidr := range g.blockedSubnets {
		subnets = append(subnets, cidr)
	}
	return subnets
}

// ============================================================================
// 统计
// ============================================================================

// GaterStats 门控统计
type GaterStats struct {
	BlockedPeers       int
	BlockedIPs         int
	BlockedSubnets     int
	BlockedPorts       int
	InterceptedDials   int64
	InterceptedAccepts int64
}

// Stats 返回统计信息
func (g *Gater) Stats() GaterStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return GaterStats{
		BlockedPeers:       len(g.blocked),
		BlockedIPs:         len(g.blockedIPs),
		BlockedSubnets:     len(g.blockedSubnets),
		BlockedPorts:       len(g.blockedPorts),
		InterceptedDials:   atomic.LoadInt64(&g.interceptedDials),
		InterceptedAccepts: atomic.LoadInt64(&g.interceptedAccepts),
	}
}

// BlockedIPs 返回所有被阻止的 IP
func (g *Gater) BlockedIPs() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	ips := make([]string, 0, len(g.blockedIPs))
	for ip := range g.blockedIPs {
		ips = append(ips, ip)
	}
	return ips
}

// BlockedPorts 返回所有被阻止的端口
func (g *Gater) BlockedPortList() []int {
	g.mu.RLock()
	defer g.mu.RUnlock()

	ports := make([]int, 0, len(g.blockedPorts))
	for port := range g.blockedPorts {
		ports = append(ports, port)
	}
	return ports
}
