package host

import (
	"strings"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// addrsManager 地址管理器
// 负责管理 Host 的监听地址和外部观测地址
type addrsManager struct {
	swarm        pkgif.Swarm
	addrsFactory AddrsFactory
	localPeerID  types.PeerID

	mu          sync.RWMutex
	listenAddrs []types.Multiaddr

	// 观测地址管理 (TD-HOST-001)
	observedAddrs *ObservedAddrManager

	// 可达性协调器（用于获取已验证的外部地址和 Relay 地址）
	coordinator pkgif.ReachabilityCoordinator
}

// ObservedAddrManager 观测地址管理器
//
// 管理从远程节点收到的观测地址（外部地址）。
// 使用评分机制选择最可信的地址。
type ObservedAddrManager struct {
	mu sync.RWMutex

	// addrs 观测地址映射：addr -> *observedAddrEntry
	addrs map[string]*observedAddrEntry
}

// observedAddrEntry 观测地址条目
type observedAddrEntry struct {
	addr       types.Multiaddr
	seenCount  int       // 被观测次数
	lastSeen   time.Time // 最后观测时间
	confidence int       // 置信度分数
}

// newObservedAddrManager 创建观测地址管理器
func newObservedAddrManager() *ObservedAddrManager {
	return &ObservedAddrManager{
		addrs: make(map[string]*observedAddrEntry),
	}
}

// newAddrsManager 创建地址管理器
func newAddrsManager(swarm pkgif.Swarm, localPeerID types.PeerID, factory AddrsFactory) *addrsManager {
	if factory == nil {
		factory = DefaultAddrsFactory
	}

	return &addrsManager{
		swarm:         swarm,
		localPeerID:   localPeerID,
		addrsFactory:  factory,
		listenAddrs:   make([]types.Multiaddr, 0),
		observedAddrs: newObservedAddrManager(),
	}
}

// Addrs 返回 Host 的地址列表
// 该方法组合监听地址和观测地址，并应用过滤器
func (m *addrsManager) Addrs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// 1. 获取监听地址
	listenAddrs := m.getListenAddrs()

	// 2. 应用过滤器
	filtered := m.addrsFactory(listenAddrs)

	// 3. 转换为字符串
	result := make([]string, len(filtered))
	for i, addr := range filtered {
		result[i] = addr.String()
	}

	return result
}

// getListenAddrs 获取监听地址
//
// 完整实现：
//  1. 返回缓存的监听地址（由 updateListenAddrs 更新）
//  2. 添加 /p2p/<peerID> 后缀
func (m *addrsManager) getListenAddrs() []types.Multiaddr {
	// 添加 /p2p/<peerID> 后缀
	result := make([]types.Multiaddr, 0, len(m.listenAddrs))
	for _, addr := range m.listenAddrs {
		// 检查是否已有 /p2p 后缀
		if _, err := addr.ValueForProtocol(types.ProtocolP2P); err == nil {
			// 已有后缀，直接使用
			result = append(result, addr)
		} else {
			// 添加 /p2p/<peerID> 后缀
			fullAddr, err := types.NewMultiaddr(addr.String() + "/p2p/" + string(m.localPeerID))
			if err == nil {
				result = append(result, fullAddr)
			}
		}
	}

	return result
}

// updateListenAddrs 更新监听地址（内部使用）
func (m *addrsManager) updateListenAddrs(addrs []types.Multiaddr) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.listenAddrs = addrs
}

// addObservedAddr 添加观测地址
// AllAddrs 返回所有地址（监听 + 观测）
func (m *addrsManager) AllAddrs() []types.Multiaddr {
	m.mu.RLock()
	defer m.mu.RUnlock()

	listenAddrs := m.getListenAddrs()

	// 合并观测地址
	if m.observedAddrs != nil {
		observedAddrs := m.observedAddrs.TopAddrs(5) // 取前 5 个最可信的观测地址
		return append(listenAddrs, observedAddrs...)
	}

	return listenAddrs
}

// SetReachabilityCoordinator 设置可达性协调器
//
// 用于获取已验证的外部地址和 Relay 地址。
// 应在 Host 初始化时调用。
func (m *addrsManager) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	m.mu.Lock()
	m.coordinator = coordinator
	m.mu.Unlock()
}

// AdvertisedAddrs 返回对外公告地址
//
// 整合多个地址来源，按优先级排序：
//  1. 已验证的直连地址（来自 Reachability Coordinator）
//  2. Relay 地址（来自 Reachability Coordinator）
//  3. 监听地址（作为兜底）
//
// 返回的地址格式包含 /p2p/<peerID> 后缀。
func (m *addrsManager) AdvertisedAddrs() []string {
	m.mu.RLock()
	coordinator := m.coordinator
	m.mu.RUnlock()

	var result []string
	seen := make(map[string]struct{})

	// 1. 从 Coordinator 获取已验证的外部地址
	if coordinator != nil {
		for _, addr := range coordinator.VerifiedDirectAddresses() {
			fullAddr := m.appendPeerID(addr)
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}

		// 2. 获取 Relay 地址
		for _, addr := range coordinator.RelayAddresses() {
			fullAddr := m.appendPeerID(addr)
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	// 3. 监听地址作为兜底
	listenAddrs := m.Addrs()
	for _, addr := range listenAddrs {
		if !isConnectableAddr(addr) {
			continue
		}
		if _, ok := seen[addr]; !ok {
			seen[addr] = struct{}{}
			result = append(result, addr)
		}
	}

	return result
}

// HolePunchAddrs 返回用于打洞协商的地址列表
//
// 
// 这些地址是 NAT 网关分配的公网端点，虽未通过 dial-back 验证，
// 但对于打洞协商是必需的——打洞的本质就是双方同时向对方的外部地址发包。
//
// 地址优先级：
//  1. STUN/UPnP/NAT-PMP 发现的候选地址（★ 打洞核心）
//  2. 已验证的直连地址
//
// 注意：不返回 Relay 地址。打洞的目标是建立直连，
// 如果只有 Relay 地址，说明 STUN 发现失败，应该使用 Relay 兜底。
func (m *addrsManager) HolePunchAddrs() []string {
	m.mu.RLock()
	coordinator := m.coordinator
	m.mu.RUnlock()

	var result []string
	seen := make(map[string]struct{})

	if coordinator != nil {
		// 1. 首先获取 STUN 候选地址（★ 打洞核心）
		// 
		for _, addr := range coordinator.CandidateDirectAddresses() {
			// 
			if !isDirectConnectableAddr(addr) {
				continue
			}
			fullAddr := m.appendPeerID(addr)
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}

		// 2. 然后获取已验证的直连地址
		for _, addr := range coordinator.VerifiedDirectAddresses() {
			// 
			if !isDirectConnectableAddr(addr) {
				continue
			}
			fullAddr := m.appendPeerID(addr)
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}

		// 注意：不添加 Relay 地址！
		// 打洞的目的是建立直连，Relay 地址只作为信令通道
	}

	// 3. 如果什么地址都没有，返回空列表
	// 调用者会收到空列表，知道无法打洞，应该使用 Relay 兜底

	return result
}

// ShareableAddrs 返回可分享的可连接地址
//
// 返回适合分享给其他节点用于连接的地址，按优先级排序：
//  1. 已验证的公网地址（STUN/网卡发现/用户配置）
//  2. Relay 中继地址（保证可达）
//  3. 局域网可连接地址（过滤 0.0.0.0 等）
//
// 所有返回的地址都保证是实际可连接的，不包含：
//   - 0.0.0.0 或 :: 等未指定地址
//   - 127.0.0.1 回环地址
func (m *addrsManager) ShareableAddrs() []string {
	m.mu.RLock()
	coordinator := m.coordinator
	m.mu.RUnlock()

	var result []string
	seen := make(map[string]struct{})

	// 1. 从 Coordinator 获取已验证的直连地址
	if coordinator != nil {
		for _, addr := range coordinator.VerifiedDirectAddresses() {
			if !isConnectableAddr(addr) {
				continue
			}
			fullAddr := m.appendPeerID(addr)
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}

		// 2. 获取 Relay 地址（保证可达的兜底）
		for _, addr := range coordinator.RelayAddresses() {
			// BUG FIX: 对 Relay 地址也进行过滤，防止 0.0.0.0 等地址泄露
			if !isConnectableAddr(addr) {
				continue
			}
			fullAddr := m.appendPeerID(addr)
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	// 3. 如果没有已验证地址和 Relay，从监听地址中提取可连接的局域网地址
	if len(result) == 0 {
		listenAddrs := m.Addrs()
		for _, addr := range listenAddrs {
			if !isConnectableAddr(addr) {
				continue
			}
			if _, ok := seen[addr]; !ok {
				seen[addr] = struct{}{}
				result = append(result, addr)
			}
		}
	}

	return result
}

// isConnectableAddr 判断地址是否是可连接的
//
// 过滤掉以下不可连接的地址：
//   - 0.0.0.0 (IPv4 未指定)
//   - :: (IPv6 未指定)
//   - 127.0.0.1 / ::1 (回环地址)
//
// 注意：此函数不过滤 /p2p-circuit/ 地址，因为 Relay 地址是合法的可连接地址。
// 对于打洞场景，使用 isDirectConnectableAddr() 进行更严格的过滤。
func isConnectableAddr(addr string) bool {
	if addr == "" {
		return false
	}

	// 检查不可连接的地址模式
	unconnectablePatterns := []string{
		"/ip4/0.0.0.0/",
		"/ip6/::/",
		"/ip4/127.0.0.1/",
		"/ip4/127.",
		"/ip6/::1/",
	}

	for _, pattern := range unconnectablePatterns {
		if strings.Contains(addr, pattern) {
			return false
		}
	}

	return true
}

// isDirectConnectableAddr 判断地址是否可用于直连/打洞
//
// 
// 因为打洞的目标是建立直连，Relay 地址不能用于打洞
func isDirectConnectableAddr(addr string) bool {
	if !isConnectableAddr(addr) {
		return false
	}

	// 
	if strings.Contains(addr, "/p2p-circuit/") {
		return false
	}

	return true
}

// appendPeerID 为地址添加 /p2p/<peerID> 后缀
func (m *addrsManager) appendPeerID(addr string) string {
	if addr == "" {
		return ""
	}

	// 检查是否已有 /p2p/ 后缀
	if containsP2PSuffix(addr) {
		return addr
	}

	return addr + "/p2p/" + string(m.localPeerID)
}

// containsP2PSuffix 检查地址是否包含 /p2p/ 后缀
func containsP2PSuffix(addr string) bool {
	for i := 0; i < len(addr)-4; i++ {
		if addr[i:i+4] == "/p2p" {
			return true
		}
	}
	return false
}

// Start 启动地址管理器
//
// 实现 TD-HOST-001：
//  1. 初始化观测地址管理器
//  2. 启动观测地址清理
func (m *addrsManager) Start() error {
	// 启动观测地址清理（后台任务）
	if m.observedAddrs != nil {
		go m.observedAddrs.gc()
	}

	return nil
}

// Stop 停止地址管理器
func (m *addrsManager) Stop() error {
	// 清理观测地址
	if m.observedAddrs != nil {
		m.observedAddrs.Clear()
	}
	return nil
}

// ============================================================================
// ObservedAddrManager 方法
// ============================================================================

// Add 添加观测地址
func (o *ObservedAddrManager) Add(addr types.Multiaddr) {
	o.mu.Lock()
	defer o.mu.Unlock()

	addrStr := addr.String()
	entry, exists := o.addrs[addrStr]

	if exists {
		// 更新现有条目
		entry.seenCount++
		entry.lastSeen = time.Now()
		entry.confidence = calculateConfidence(entry.seenCount, entry.lastSeen)
	} else {
		// 创建新条目
		o.addrs[addrStr] = &observedAddrEntry{
			addr:       addr,
			seenCount:  1,
			lastSeen:   time.Now(),
			confidence: 1,
		}
	}
}

// TopAddrs 返回置信度最高的 N 个地址
func (o *ObservedAddrManager) TopAddrs(n int) []types.Multiaddr {
	o.mu.RLock()
	defer o.mu.RUnlock()

	if len(o.addrs) == 0 {
		return nil
	}

	// 收集所有条目
	entries := make([]*observedAddrEntry, 0, len(o.addrs))
	for _, entry := range o.addrs {
		entries = append(entries, entry)
	}

	// 简单排序：按置信度降序
	for i := 0; i < len(entries) && i < n; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].confidence > entries[i].confidence {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}

	// 取前 N 个
	limit := n
	if len(entries) < limit {
		limit = len(entries)
	}

	result := make([]types.Multiaddr, limit)
	for i := 0; i < limit; i++ {
		result[i] = entries[i].addr
	}

	return result
}

// Clear 清空所有观测地址
func (o *ObservedAddrManager) Clear() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.addrs = make(map[string]*observedAddrEntry)
}

// gc 后台清理过期观测地址
func (o *ObservedAddrManager) gc() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		o.cleanExpired()
	}
}

// cleanExpired 清理过期地址
func (o *ObservedAddrManager) cleanExpired() {
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now()
	expiryThreshold := 30 * time.Minute

	for addrStr, entry := range o.addrs {
		if now.Sub(entry.lastSeen) > expiryThreshold {
			delete(o.addrs, addrStr)
		}
	}
}

// calculateConfidence 计算地址置信度
//
// 置信度计算规则：
//   - 基础分数：观测次数
//   - 时间衰减：最近观测的地址分数更高
func calculateConfidence(seenCount int, lastSeen time.Time) int {
	baseScore := seenCount * 10

	// 时间衰减因子
	age := time.Since(lastSeen)
	if age < 5*time.Minute {
		baseScore += 20 // 最近观测，加分
	} else if age < 30*time.Minute {
		baseScore += 10
	}

	return baseScore
}
