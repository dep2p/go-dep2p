package connmgr

import (
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              ConnectionGater 实现
// ============================================================================

// ConnectionGater 连接门控实现
type ConnectionGater struct {
	config connmgr.GaterConfig

	// 阻止列表
	blockedPeers   map[types.NodeID]struct{}
	blockedAddrs   map[string]struct{} // key: IP.String()
	blockedSubnets map[string]*net.IPNet

	mu sync.RWMutex

	// 统计
	interceptedDials   int64
	interceptedAccepts int64
}

// 确保实现接口
var _ connmgr.ConnectionGater = (*ConnectionGater)(nil)

// NewConnectionGater 创建连接门控
func NewConnectionGater(config connmgr.GaterConfig) (*ConnectionGater, error) {
	g := &ConnectionGater{
		config:         config,
		blockedPeers:   make(map[types.NodeID]struct{}),
		blockedAddrs:   make(map[string]struct{}),
		blockedSubnets: make(map[string]*net.IPNet),
	}

	// 如果有持久化存储，加载规则
	if config.Store != nil {
		if err := g.loadFromStore(); err != nil {
			return nil, err
		}
	}

	return g, nil
}

// loadFromStore 从存储加载规则
func (g *ConnectionGater) loadFromStore() error {
	store := g.config.Store
	if store == nil {
		return nil
	}

	// 加载节点黑名单
	peers, err := store.LoadPeers()
	if err != nil {
		log.Error("加载节点黑名单失败", "err", err)
		return err
	}
	for _, p := range peers {
		g.blockedPeers[p] = struct{}{}
	}

	// 加载 IP 黑名单
	addrs, err := store.LoadAddrs()
	if err != nil {
		log.Error("加载 IP 黑名单失败", "err", err)
		return err
	}
	for _, ip := range addrs {
		g.blockedAddrs[ip.String()] = struct{}{}
	}

	// 加载子网黑名单
	subnets, err := store.LoadSubnets()
	if err != nil {
		log.Error("加载子网黑名单失败", "err", err)
		return err
	}
	for _, subnet := range subnets {
		g.blockedSubnets[subnet.String()] = subnet
	}

	log.Info("从存储加载黑名单规则",
		"peers", len(peers),
		"addrs", len(addrs),
		"subnets", len(subnets))

	return nil
}

// ==================== Peer 级别阻止 ====================

// BlockPeer 阻止节点
func (g *ConnectionGater) BlockPeer(nodeID types.NodeID) error {
	// 先持久化，再更新内存（保证一致性）
	if g.config.Store != nil {
		if err := g.config.Store.SavePeer(nodeID); err != nil {
			log.Error("持久化节点黑名单失败",
				"peer", nodeID.ShortString(),
				"err", err)
			return err
		}
	}

	g.mu.Lock()
	g.blockedPeers[nodeID] = struct{}{}
	g.mu.Unlock()

	log.Info("节点已加入黑名单",
		"peer", nodeID.ShortString())

	return nil
}

// UnblockPeer 解除节点阻止
func (g *ConnectionGater) UnblockPeer(nodeID types.NodeID) error {
	g.mu.Lock()
	delete(g.blockedPeers, nodeID)
	g.mu.Unlock()

	// 持久化
	if g.config.Store != nil {
		if err := g.config.Store.DeletePeer(nodeID); err != nil {
			log.Error("删除节点黑名单持久化失败",
				"peer", nodeID.ShortString(),
				"err", err)
			return err
		}
	}

	log.Info("节点已从黑名单移除",
		"peer", nodeID.ShortString())

	return nil
}

// ListBlockedPeers 列出被阻止的节点
func (g *ConnectionGater) ListBlockedPeers() []types.NodeID {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]types.NodeID, 0, len(g.blockedPeers))
	for p := range g.blockedPeers {
		result = append(result, p)
	}
	return result
}

// IsBlocked 检查节点是否被阻止
func (g *ConnectionGater) IsBlocked(nodeID types.NodeID) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	_, blocked := g.blockedPeers[nodeID]
	return blocked
}

// ==================== IP 地址级别阻止 ====================

// BlockAddr 阻止 IP 地址
func (g *ConnectionGater) BlockAddr(ip net.IP) error {
	// 先持久化，再更新内存（保证一致性）
	if g.config.Store != nil {
		if err := g.config.Store.SaveAddr(ip); err != nil {
			log.Error("持久化 IP 黑名单失败",
				"ip", ip.String(),
				"err", err)
			return err
		}
	}

	g.mu.Lock()
	g.blockedAddrs[ip.String()] = struct{}{}
	g.mu.Unlock()

	log.Info("IP 地址已加入黑名单",
		"ip", ip.String())

	return nil
}

// UnblockAddr 解除 IP 地址阻止
func (g *ConnectionGater) UnblockAddr(ip net.IP) error {
	g.mu.Lock()
	delete(g.blockedAddrs, ip.String())
	g.mu.Unlock()

	// 持久化
	if g.config.Store != nil {
		if err := g.config.Store.DeleteAddr(ip); err != nil {
			log.Error("删除 IP 黑名单持久化失败",
				"ip", ip.String(),
				"err", err)
			return err
		}
	}

	log.Info("IP 地址已从黑名单移除",
		"ip", ip.String())

	return nil
}

// ListBlockedAddrs 列出被阻止的 IP 地址
func (g *ConnectionGater) ListBlockedAddrs() []net.IP {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]net.IP, 0, len(g.blockedAddrs))
	for ipStr := range g.blockedAddrs {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			result = append(result, ip)
		}
	}
	return result
}

// IsAddrBlocked 检查 IP 地址是否被阻止
func (g *ConnectionGater) IsAddrBlocked(ip net.IP) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// 检查精确匹配
	if _, blocked := g.blockedAddrs[ip.String()]; blocked {
		return true
	}

	// 检查子网匹配
	for _, subnet := range g.blockedSubnets {
		if subnet.Contains(ip) {
			return true
		}
	}

	return false
}

// ==================== 子网级别阻止 ====================

// BlockSubnet 阻止子网
func (g *ConnectionGater) BlockSubnet(ipnet *net.IPNet) error {
	// 先持久化，再更新内存（保证一致性）
	if g.config.Store != nil {
		if err := g.config.Store.SaveSubnet(ipnet); err != nil {
			log.Error("持久化子网黑名单失败",
				"subnet", ipnet.String(),
				"err", err)
			return err
		}
	}

	g.mu.Lock()
	g.blockedSubnets[ipnet.String()] = ipnet
	g.mu.Unlock()

	log.Info("子网已加入黑名单",
		"subnet", ipnet.String())

	return nil
}

// UnblockSubnet 解除子网阻止
func (g *ConnectionGater) UnblockSubnet(ipnet *net.IPNet) error {
	g.mu.Lock()
	delete(g.blockedSubnets, ipnet.String())
	g.mu.Unlock()

	// 持久化
	if g.config.Store != nil {
		if err := g.config.Store.DeleteSubnet(ipnet); err != nil {
			log.Error("删除子网黑名单持久化失败",
				"subnet", ipnet.String(),
				"err", err)
			return err
		}
	}

	log.Info("子网已从黑名单移除",
		"subnet", ipnet.String())

	return nil
}

// ListBlockedSubnets 列出被阻止的子网
func (g *ConnectionGater) ListBlockedSubnets() []*net.IPNet {
	g.mu.RLock()
	defer g.mu.RUnlock()

	result := make([]*net.IPNet, 0, len(g.blockedSubnets))
	for _, subnet := range g.blockedSubnets {
		result = append(result, subnet)
	}
	return result
}

// ==================== 连接拦截点 ====================

// InterceptPeerDial 拦截出站连接
func (g *ConnectionGater) InterceptPeerDial(nodeID types.NodeID) bool {
	if !g.config.Enabled {
		return true
	}

	g.mu.RLock()
	_, blocked := g.blockedPeers[nodeID]
	g.mu.RUnlock()

	if blocked {
		atomic.AddInt64(&g.interceptedDials, 1)
		log.Debug("拦截出站连接 - 节点在黑名单",
			"peer", nodeID.ShortString())
		return false
	}

	return true
}

// InterceptAccept 拦截入站连接
func (g *ConnectionGater) InterceptAccept(remoteAddr string) bool {
	if !g.config.Enabled {
		return true
	}

	// multiaddr（以 / 开头）可能不是传统 IP:Port（例如 relay-circuit / dnsaddr 等）。
	// 对于 relay-circuit 地址，必须放行，否则会导致中继入站连接被拒绝，进而出现 TLS 握手超时/取消。
	// 对于 multiaddr 且可提取 IP 的情况，仍然执行 IP 黑名单检查。
	if strings.HasPrefix(remoteAddr, "/") {
		ma := types.Multiaddr(remoteAddr)

		// 1) relay-circuit：直接放行，后续由 InterceptSecured 做节点级黑名单
		if ma.IsRelay() {
			log.Debug("放行 relay 入站连接", "addr", remoteAddr)
			return true
		}

		// 2) 非 relay 的 multiaddr：尝试提取 IP 做黑名单检查
		if ip := ma.IP(); ip != nil {
			if g.IsAddrBlocked(ip) {
				atomic.AddInt64(&g.interceptedAccepts, 1)
				log.Debug("拦截入站连接 - IP 在黑名单", "addr", remoteAddr)
				return false
			}
			return true
		}

		// 3) multiaddr 但无法提取 IP（如 /dns4/...）：放行（避免误伤），交给 InterceptSecured
		log.Debug("multiaddr 无法提取 IP，放行入站连接", "addr", remoteAddr)
		return true
	}

	// 解析远程地址
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// 可能没有端口，尝试直接解析
		host = remoteAddr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// 无法解析 IP，记录警告并拒绝（安全策略：拒绝未知格式）
		log.Warn("无法解析远程地址，拒绝连接",
			"addr", remoteAddr)
		atomic.AddInt64(&g.interceptedAccepts, 1)
		return false
	}

	if g.IsAddrBlocked(ip) {
		atomic.AddInt64(&g.interceptedAccepts, 1)
		log.Debug("拦截入站连接 - IP 在黑名单",
			"addr", remoteAddr)
		return false
	}

	return true
}

// InterceptSecured 拦截已认证连接
func (g *ConnectionGater) InterceptSecured(direction types.Direction, nodeID types.NodeID) bool {
	if !g.config.Enabled {
		return true
	}

	// 出站连接已经在 InterceptPeerDial 检查过
	if direction == types.DirOutbound {
		return true
	}

	// 入站连接检查节点 ID
	g.mu.RLock()
	_, blocked := g.blockedPeers[nodeID]
	g.mu.RUnlock()

	if blocked {
		atomic.AddInt64(&g.interceptedAccepts, 1)
		log.Debug("拦截入站连接 - 节点在黑名单",
			"peer", nodeID.ShortString())
		return false
	}

	return true
}

// ==================== 管理 ====================

// Clear 清除所有阻止规则
func (g *ConnectionGater) Clear() {
	g.mu.Lock()
	g.blockedPeers = make(map[types.NodeID]struct{})
	g.blockedAddrs = make(map[string]struct{})
	g.blockedSubnets = make(map[string]*net.IPNet)
	g.mu.Unlock()

	log.Info("已清除所有黑名单规则")
}

// Stats 返回阻止统计
func (g *ConnectionGater) Stats() connmgr.GaterStats {
	g.mu.RLock()
	defer g.mu.RUnlock()

	return connmgr.GaterStats{
		BlockedPeers:       len(g.blockedPeers),
		BlockedAddrs:       len(g.blockedAddrs),
		BlockedSubnets:     len(g.blockedSubnets),
		InterceptedDials:   atomic.LoadInt64(&g.interceptedDials),
		InterceptedAccepts: atomic.LoadInt64(&g.interceptedAccepts),
	}
}

// ==================== 辅助方法 ====================

// BlockPeerAndCloseConns 阻止节点并关闭现有连接
//
// 需要提供关闭连接的回调函数
func (g *ConnectionGater) BlockPeerAndCloseConns(nodeID types.NodeID, closeFunc func(types.NodeID) error) error {
	if err := g.BlockPeer(nodeID); err != nil {
		return err
	}

	if closeFunc != nil {
		if err := closeFunc(nodeID); err != nil {
			log.Warn("关闭被阻止节点的连接失败",
				"peer", nodeID.ShortString(),
				"err", err)
		}
	}

	return nil
}

// ImportPeers 批量导入被阻止的节点
func (g *ConnectionGater) ImportPeers(peers []types.NodeID) error {
	for _, p := range peers {
		if err := g.BlockPeer(p); err != nil {
			return err
		}
	}
	return nil
}

// ExportRules 导出所有规则
func (g *ConnectionGater) ExportRules() (peers []types.NodeID, addrs []net.IP, subnets []*net.IPNet) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	peers = make([]types.NodeID, 0, len(g.blockedPeers))
	for p := range g.blockedPeers {
		peers = append(peers, p)
	}

	addrs = make([]net.IP, 0, len(g.blockedAddrs))
	for ipStr := range g.blockedAddrs {
		ip := net.ParseIP(ipStr)
		if ip != nil {
			addrs = append(addrs, ip)
		}
	}

	subnets = make([]*net.IPNet, 0, len(g.blockedSubnets))
	for _, subnet := range g.blockedSubnets {
		subnets = append(subnets, subnet)
	}

	return
}

