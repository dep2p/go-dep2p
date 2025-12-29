// Package discovery 提供节点发现模块的实现
package discovery

import (
	"context"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              发现循环
// ============================================================================

// discoveryLoop 发现循环
func (s *DiscoveryService) discoveryLoop() {
	// 首次立即执行
	s.runDiscovery()

	for {
		interval := s.GetDiscoveryInterval()
		timer := time.NewTimer(interval)

		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			s.runDiscovery()
		}
	}
}

// runDiscovery 执行一次发现
func (s *DiscoveryService) runDiscovery() {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for name, discoverer := range discoverers {
		go func(n string, d discoveryif.Discoverer) {
			// 使用 DiscoverPeers 进行发现
			ch, err := d.DiscoverPeers(ctx, "")
			if err != nil {
				log.Debug("发现器查找失败",
					"name", n,
					"err", err)
				return
			}

			for info := range ch {
				s.handleDiscoveredPeer(info)
			}
		}(name, discoverer)
	}
}

// handleDiscoveredPeer 处理发现的节点
func (s *DiscoveryService) handleDiscoveredPeer(info discoveryif.PeerInfo) {
	// 检查私有 Realm 过滤
	if !s.shouldAcceptDiscoveredPeer(info) {
		log.Debug("私有 Realm 过滤：拒绝外部节点发现",
			"peer", info.ID.ShortString())
		return
	}

	// 转换地址
	addrs := make([]endpoint.Address, len(info.Addrs))
	for i, addr := range info.Addrs {
		addrs[i] = &stringAddr{s: addr.String()}
	}

	// 更新已知节点（使用独立的锁范围）
	s.mu.Lock()
	existing, ok := s.knownPeers[info.ID]
	if ok {
		existing.Addresses = addrs
		existing.LastSeen = time.Now()
		s.knownPeers[info.ID] = existing
	} else {
		s.knownPeers[info.ID] = peerRealmInfo{
			RealmID:   s.currentRealm,
			Addresses: addrs,
			LastSeen:  time.Now(),
		}
	}
	s.mu.Unlock()

	log.Debug("发现节点",
		"peer", info.ID.ShortString(),
		"addrs", len(addrs))

	// 通知等待中的查找（使用独立的锁范围，避免嵌套锁）
	s.lookupMu.Lock()
	if waiters, ok := s.pendingLookups[info.ID]; ok {
		for _, ch := range waiters {
			select {
			case ch <- lookupResult{addrs: addrs}:
			default:
			}
		}
		delete(s.pendingLookups, info.ID)
	}
	s.lookupMu.Unlock()
}

// shouldAcceptDiscoveredPeer 检查是否应该接受发现的节点
//
// 如果当前 Realm 是私有的，只接受 Realm 成员的发现
func (s *DiscoveryService) shouldAcceptDiscoveredPeer(info discoveryif.PeerInfo) bool {
	// 没有访问控制器，接受所有节点
	if s.accessController == nil {
		return true
	}

	// 没有当前 Realm，接受所有节点
	if s.currentRealm == "" {
		return true
	}

	// 检查当前 Realm 的访问级别
	accessLevel := s.accessController.GetAccess(s.currentRealm)

	// 如果不是私有 Realm，接受所有节点
	if accessLevel != types.AccessLevelPrivate {
		return true
	}

	// 私有 Realm：只接受 Realm 成员
	if s.realmManager == nil {
		return true
	}

	// IMPL-1227: 检查节点是否是 Realm 成员
	realm := s.realmManager.CurrentRealm()
	if realm == nil {
		return true // 未加入 Realm，允许所有节点
	}
	for _, peer := range realm.Members() {
		if peer == info.ID {
			return true
		}
	}

	return false
}

// ============================================================================
//                              通告循环
// ============================================================================

// announceLoop 通告循环
func (s *DiscoveryService) announceLoop() {
	// 首次等待一小段时间
	time.Sleep(5 * time.Second)

	// 使用 RefreshInterval 作为通告间隔，如果未设置则默认 10 分钟
	announceInterval := s.config.RefreshInterval
	if announceInterval == 0 {
		announceInterval = 10 * time.Minute
	}

	ticker := time.NewTicker(announceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runAnnounce()
		}
	}
}

// runAnnounce 执行一次通告
//
// Layer1 修复：不再自动注册到 "default" 或 "realmID" namespace。
//
// 原因：
//   - 节点地址发布由 PeerRecord (sys/peer/<nodeID>) 处理，无需 provider 机制
//   - 服务提供者注册（如 sys/relay, sys/bootstrap）应通过 Register() API 显式调用
//   - 自动注册 "default/realmID" 会污染 provider registry，影响可扩展性
//
// 现在 runAnnounce 只负责刷新 PeerRecord（如果配置了 DHT），
// 显式服务注册的续约由 renewalLoop 处理。
func (s *DiscoveryService) runAnnounce() {
	// 检查是否应该通告（私有 Realm 不通告到公共网络）
	if !s.shouldAnnounce() {
		log.Debug("私有 Realm：跳过公共网络通告")
		return
	}

	// Layer1 修复：只刷新 PeerRecord，不注册 provider
	// PeerRecord 刷新由 DHT 的 republishLoop 自动处理
	// 这里只做日志记录
	log.Debug("announceLoop tick：PeerRecord 刷新由 DHT republishLoop 处理")
}

// shouldAnnounce 检查是否应该通告
//
// 私有 Realm 不应该通告到公共 DHT/mDNS
func (s *DiscoveryService) shouldAnnounce() bool {
	// 没有访问控制器，允许通告
	if s.accessController == nil {
		return true
	}

	// 没有当前 Realm，允许通告
	if s.currentRealm == "" {
		return true
	}

	// 检查当前 Realm 的访问级别
	accessLevel := s.accessController.GetAccess(s.currentRealm)

	// 私有 Realm 不通告到公共网络
	if accessLevel == types.AccessLevelPrivate {
		return false
	}

	return true
}

// ============================================================================
//                              清理循环
// ============================================================================

// cleanupLoop 清理循环
func (s *DiscoveryService) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupStalePeers()
		}
	}
}

// cleanupStalePeers 清理过期节点
func (s *DiscoveryService) cleanupStalePeers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 默认 1 小时过期
	peerTTL := 1 * time.Hour
	cutoff := time.Now().Add(-peerTTL)

	for id, info := range s.knownPeers {
		if info.LastSeen.Before(cutoff) {
			delete(s.knownPeers, id)
			log.Debug("移除过期节点",
				"peer", id.ShortString())
		}
	}
}

// ============================================================================
//                              统一 API 续约循环
// ============================================================================

// renewalLoop 统一 API 注册续约循环
//
// 自动续约通过 RegisterService 注册的服务，间隔为 TTL/2。
func (s *DiscoveryService) renewalLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runRenewals()
		}
	}
}

// runRenewals 执行续约
func (s *DiscoveryService) runRenewals() {
	s.mu.RLock()
	if s.activeRegistrations == nil || len(s.activeRegistrations) == 0 {
		s.mu.RUnlock()
		return
	}

	// 复制需要续约的注册
	now := time.Now()
	toRenew := make([]*activeRegistration, 0)

	for _, reg := range s.activeRegistrations {
		// 在过期前 TTL/2 时续约
		renewAt := reg.expiresAt.Add(-reg.reg.TTL / 2)
		if now.After(renewAt) {
			toRenew = append(toRenew, reg)
		}
	}
	s.mu.RUnlock()

	if len(toRenew) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for _, reg := range toRenew {
		if err := s.RegisterService(ctx, reg.reg); err != nil {
			log.Debug("续约失败", "key", reg.fullKey, "err", err)
		} else {
			log.Debug("续约成功", "key", reg.fullKey)
		}
	}
}

// ============================================================================
//                              辅助类型
// ============================================================================

// stringAddr 字符串地址
type stringAddr struct {
	s string
}

// Network 返回网络类型
// 根据地址格式推断网络类型
func (a *stringAddr) Network() string {
	// 检查 multiaddr 格式
	if strings.Contains(a.s, "/ip4/") {
		if strings.Contains(a.s, "/udp/") {
			return "udp4"
		}
		if strings.Contains(a.s, "/tcp/") {
			return "tcp4"
		}
		return "ip4"
	}
	if strings.Contains(a.s, "/ip6/") {
		if strings.Contains(a.s, "/udp/") {
			return "udp6"
		}
		if strings.Contains(a.s, "/tcp/") {
			return "tcp6"
		}
		return "ip6"
	}
	// 检查传统地址格式 (host:port)
	if strings.Contains(a.s, ":") && !strings.Contains(a.s, "/") {
		// IPv6 地址通常包含多个冒号或被方括号包围
		if strings.Count(a.s, ":") > 1 || strings.HasPrefix(a.s, "[") {
			return "tcp6"
		}
		return "tcp4"
	}
	return "unknown"
}

func (a *stringAddr) String() string   { return a.s }
func (a *stringAddr) Bytes() []byte    { return []byte(a.s) }
func (a *stringAddr) IsPublic() bool   { return types.Multiaddr(a.s).IsPublic() }
func (a *stringAddr) IsPrivate() bool  { return types.Multiaddr(a.s).IsPrivate() }
func (a *stringAddr) IsLoopback() bool { return types.Multiaddr(a.s).IsLoopback() }
func (a *stringAddr) Multiaddr() string { return a.s } // 已经是 multiaddr 格式
func (a *stringAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.s == other.String()
}

