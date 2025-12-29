// Package discovery 提供节点发现模块的实现
package discovery

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/discovery/dht"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// DefaultFindPeerTimeout FindPeer 的默认超时时间
const DefaultFindPeerTimeout = 30 * time.Second

// ============================================================================
//                              查找功能
// ============================================================================

// FindPeer 查找节点
//
// 查找优先级（从快到慢，本地优先）：
// 1. 内部 knownPeers 缓存
// 2. AddressBook 缓存（如果 Endpoint 已注入）
// 3. 已连接节点的远程地址（如果 Endpoint 已注入）
// 4. DHT/mDNS 等 discoverer 网络查询
//
// REQ-DISC-006: 带递归防护
//
// 参考：docs/01-design/protocols/network/01-discovery.md#findpeer-本地优先查找
func (s *DiscoveryService) FindPeer(ctx context.Context, id types.NodeID) ([]endpoint.Address, error) {
	// REQ-DISC-006: 递归防护
	if !s.enterDiscoveryContext(id) {
		return nil, ErrRecursiveDiscovery
	}
	defer s.leaveDiscoveryContext(id)

	// ==================== 1. 检查 knownPeers 缓存 ====================
	s.mu.RLock()
	if info, ok := s.knownPeers[id]; ok {
		if !s.shouldFilterByRealm(info.RealmID) {
			s.mu.RUnlock()
			log.Debug("FindPeer 命中 knownPeers 缓存",
				"nodeID", id.ShortString(),
				"addrs", len(info.Addresses))
			return info.Addresses, nil
		}
	}
	s.mu.RUnlock()

	// ==================== 2. 检查 AddressBook 缓存 ====================
	s.epMu.RLock()
	ep := s.ep
	s.epMu.RUnlock()

	if ep != nil {
		if ab := ep.AddressBook(); ab != nil {
			addrs := ab.Get(endpoint.NodeID(id))
			if len(addrs) > 0 {
				log.Debug("FindPeer 命中 AddressBook 缓存",
					"nodeID", id.ShortString(),
					"addrs", len(addrs))
				return addrs, nil
			}
		}
	}

	// ==================== 3. 检查已连接节点 ====================
	if ep != nil {
		if conn, ok := ep.Connection(endpoint.NodeID(id)); ok && conn != nil {
			// 从连接获取远程地址
			remoteAddrs := conn.RemoteAddrs()
			if len(remoteAddrs) > 0 {
				log.Debug("FindPeer 命中已连接节点",
					"nodeID", id.ShortString(),
					"addrs", len(remoteAddrs))
				return remoteAddrs, nil
			}
		}
	}

	// ==================== 4. 网络查询 ====================
	// 创建等待通道
	resultCh := make(chan lookupResult, 1)

	s.lookupMu.Lock()
	s.pendingLookups[id] = append(s.pendingLookups[id], resultCh)
	s.lookupMu.Unlock()

	// 确保超时后清理 pending lookup
	defer func() {
		s.lookupMu.Lock()
		waiters := s.pendingLookups[id]
		// 移除当前的 resultCh
		newWaiters := make([]chan lookupResult, 0, len(waiters))
		for _, ch := range waiters {
			if ch != resultCh {
				newWaiters = append(newWaiters, ch)
			}
		}
		if len(newWaiters) == 0 {
			delete(s.pendingLookups, id)
		} else {
			s.pendingLookups[id] = newWaiters
		}
		s.lookupMu.Unlock()
	}()

	// 向各发现器查询
	s.mu.RLock()
	// 收集完整的 Discoverer
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	// 收集细粒度的 PeerFinder
	peerFinders := make(map[string]discoveryif.PeerFinder, len(s.peerFinders))
	for k, v := range s.peerFinders {
		peerFinders[k] = v
	}
	s.mu.RUnlock()

	// 使用完整发现器
	for _, discoverer := range discoverers {
		go func(d discoveryif.Discoverer) {
			addrs, err := d.FindPeer(ctx, id)
			if err != nil || len(addrs) == 0 {
				return
			}

			addrStrs := make([]string, len(addrs))
			for i, addr := range addrs {
				addrStrs[i] = addr.String()
			}

			s.handleDiscoveredPeer(discoveryif.PeerInfo{
				ID:    id,
				Addrs: types.StringsToMultiaddrs(addrStrs),
			})
		}(discoverer)
	}

	// 使用细粒度 PeerFinder
	for _, finder := range peerFinders {
		go func(f discoveryif.PeerFinder) {
			addrs, err := f.FindPeer(ctx, id)
			if err != nil || len(addrs) == 0 {
				return
			}

			addrStrs := make([]string, len(addrs))
			for i, addr := range addrs {
				addrStrs[i] = addr.String()
			}

			s.handleDiscoveredPeer(discoveryif.PeerInfo{
				ID:    id,
				Addrs: types.StringsToMultiaddrs(addrStrs),
			})
		}(finder)
	}

	// 如果 context 有 deadline，直接使用 context 的 deadline
	// 否则使用默认超时时间
	hasDeadline := false
	if _, ok := ctx.Deadline(); ok {
		hasDeadline = true
	}

	// 等待结果或超时
	if hasDeadline {
		// 有 deadline，直接使用 context 的 Done channel
		// 这样当 context 超时时会返回 ctx.Err()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-resultCh:
			return result.addrs, result.err
		}
	} else {
		// 没有 deadline，使用默认超时时间
		timeoutTimer := time.NewTimer(DefaultFindPeerTimeout)
		defer timeoutTimer.Stop()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.addrs, result.err
		case <-timeoutTimer.C:
		return nil, fmt.Errorf("查找节点超时: %s", id.ShortString())
		}
	}
}

// FindPeers 批量查找节点
func (s *DiscoveryService) FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error) {
	result := make(map[types.NodeID][]endpoint.Address)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, id := range ids {
		wg.Add(1)
		go func(nodeID types.NodeID) {
			defer wg.Done()

			addrs, err := s.FindPeer(ctx, nodeID)
			if err != nil || len(addrs) == 0 {
				return
			}

			mu.Lock()
			result[nodeID] = addrs
			mu.Unlock()
		}(id)
	}

	wg.Wait()
	return result, nil
}

// FindClosestPeers 查找最近的节点
func (s *DiscoveryService) FindClosestPeers(_ context.Context, key []byte, count int) ([]types.NodeID, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	realmKey := s.getRealmAwareKey(key)

	type nodeDistance struct {
		id       types.NodeID
		distance []byte
	}

	var candidates []nodeDistance
	for id, info := range s.knownPeers {
		if s.shouldFilterByRealm(info.RealmID) {
			continue
		}

		nodeKey := dht.RealmAwareDHTKey(s.currentRealm, id)
		distance := dht.XORDistance(realmKey, nodeKey)

		candidates = append(candidates, nodeDistance{
			id:       id,
			distance: distance,
		})
	}

	// 按距离排序
	sort.Slice(candidates, func(i, j int) bool {
		return compareBytes(candidates[i].distance, candidates[j].distance) < 0
	})

	result := make([]types.NodeID, 0, count)
	for i := 0; i < count && i < len(candidates); i++ {
		result = append(result, candidates[i].id)
	}

	return result, nil
}

// compareBytes 比较字节数组
func compareBytes(a, b []byte) int {
	minLen := len(a)
	if len(b) < minLen {
		minLen = len(b)
	}
	for i := 0; i < minLen; i++ {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return len(a) - len(b)
}

// DiscoverPeers 发现新节点
//
// 修正后的语义：
// 1. 查询 namespaceDiscoverers（包括 DHT）
// 2. 补充本地缓存的节点
// 3. 返回聚合结果
func (s *DiscoveryService) DiscoverPeers(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	ch := make(chan discoveryif.PeerInfo, 20)

	go func() {
		defer close(ch)

		seen := make(map[types.NodeID]struct{})

		// 1. 首先从 namespaceDiscoverers 获取（包括 DHT）
		s.mu.RLock()
		nsDiscoverers := make(map[string]discoveryif.NamespaceDiscoverer, len(s.namespaceDiscoverers))
		for k, v := range s.namespaceDiscoverers {
			nsDiscoverers[k] = v
		}
		// 同时检查完整 Discoverer 是否也实现了 NamespaceDiscoverer
		for name, d := range s.discoverers {
			if _, already := nsDiscoverers[name]; !already {
				if nsd, ok := d.(discoveryif.NamespaceDiscoverer); ok {
					nsDiscoverers[name] = nsd
				}
			}
		}
		s.mu.RUnlock()

		// 并发查询所有 namespace discoverers
		var wg sync.WaitGroup
		var mu sync.Mutex

		for name, nsd := range nsDiscoverers {
			wg.Add(1)
			go func(discovererName string, discoverer discoveryif.NamespaceDiscoverer) {
				defer wg.Done()

				subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				defer cancel()

				subCh, err := discoverer.DiscoverPeers(subCtx, namespace)
				if err != nil {
					log.Debug("namespace discoverer 查询失败",
						"discoverer", discovererName,
						"namespace", namespace,
						"err", err)
					return
				}

				for peer := range subCh {
					select {
					case <-ctx.Done():
						return
					default:
						mu.Lock()
						if _, exists := seen[peer.ID]; !exists {
							seen[peer.ID] = struct{}{}
							mu.Unlock()

							select {
							case <-ctx.Done():
								return
							case ch <- peer:
							}
						} else {
							mu.Unlock()
						}
					}
				}
			}(name, nsd)
		}

		// 等待所有 namespace discoverers 完成
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-ctx.Done():
			return
		case <-done:
		}

		// 2. 补充本地缓存的节点（如果 namespace 为空或特定系统命名空间）
		s.mu.RLock()
		for id, info := range s.knownPeers {
			if s.shouldFilterByRealm(info.RealmID) {
				continue
			}

			mu.Lock()
			if _, exists := seen[id]; exists {
				mu.Unlock()
				continue
			}
			seen[id] = struct{}{}
			mu.Unlock()

			addrs := make([]string, len(info.Addresses))
			for i, addr := range info.Addresses {
				addrs[i] = addr.String()
			}

			select {
			case <-ctx.Done():
				s.mu.RUnlock()
				return
			case ch <- discoveryif.PeerInfo{
				ID:    id,
				Addrs: types.StringsToMultiaddrs(addrs),
			}:
			}
		}
		s.mu.RUnlock()
	}()

	return ch, nil
}

// ============================================================================
//                              Realm 过滤
// ============================================================================

// shouldFilterByRealm 检查是否应该过滤
func (s *DiscoveryService) shouldFilterByRealm(peerRealm types.RealmID) bool {
	if s.realmManager == nil || !s.config.EnableRealmIsolation {
		return false
	}

	filter := dht.NewRealmFilter(s.currentRealm)
	return !filter(peerRealm)
}

// getRealmAwareKey 获取 Realm 感知的 Key
func (s *DiscoveryService) getRealmAwareKey(key []byte) []byte {
	if s.currentRealm == types.DefaultRealmID || s.currentRealm == "" {
		return key
	}

	var nodeID types.NodeID
	if len(key) >= 32 {
		copy(nodeID[:], key[:32])
	}
	return dht.RealmAwareDHTKey(s.currentRealm, nodeID)
}

// KnownPeersInRealm 获取指定 Realm 的节点数量
func (s *DiscoveryService) KnownPeersInRealm(realmID types.RealmID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	filter := dht.NewRealmFilter(realmID)
	for _, info := range s.knownPeers {
		if filter(info.RealmID) {
			count++
		}
	}
	return count
}

