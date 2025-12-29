// Package discovery 提供节点发现模块的实现
package discovery

import (
	"context"
	"strings"
	"sync"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              统一发现 API 实现
// ============================================================================

// Discover 统一发现入口
//
// 根据 DiscoveryQuery 参数从多个来源（provider、rendezvous、local）
// 并发查询并聚合去重结果。
//
// 聚合策略：
//  1. 按 Sources 数组顺序并发查询各来源
//  2. 按来源优先级排序（Sources 数组顺序）
//  3. 按 NodeID 去重，保留优先级更高的来源结果
//  4. 在 Timeout 前尽量聚合结果
func (s *DiscoveryService) Discover(ctx context.Context, query discoveryif.DiscoveryQuery) (<-chan discoveryif.DiscoveryResult, error) {
	// 应用默认值
	if len(query.Sources) == 0 {
		query.Sources = discoveryif.DefaultSources
	}
	if query.Timeout <= 0 {
		query.Timeout = 30 * time.Second
	}

	// 解析作用域和命名空间
	resolvedScope, resolvedNS := s.resolveScope(query.Namespace, query.Scope, query.RealmID)
	fullKey := s.buildDiscoveryKey(resolvedScope, resolvedNS, query.RealmID)

	ch := make(chan discoveryif.DiscoveryResult, 20)

	go func() {
		defer close(ch)

		// 创建带超时的上下文
		queryCtx, cancel := context.WithTimeout(ctx, query.Timeout)
		defer cancel()

		// 用于去重和排序
		var (
			results  = make(map[types.NodeID]peerResult)
			resultMu sync.Mutex
			wg       sync.WaitGroup
		)

		// 构建来源优先级映射
		sourcePriority := make(map[discoveryif.Source]int)
		for i, src := range query.Sources {
			sourcePriority[src] = i
		}

		// 并发查询各来源
		for _, source := range query.Sources {
			switch source {
			case discoveryif.SourceProvider:
				wg.Add(1)
				go func() {
					defer wg.Done()
					s.queryProvider(queryCtx, fullKey, func(peer discoveryif.PeerInfo) {
						resultMu.Lock()
						defer resultMu.Unlock()
						s.mergeResult(results, peer, discoveryif.SourceProvider, sourcePriority)
					})
				}()

			case discoveryif.SourceRendezvous:
				wg.Add(1)
				go func() {
					defer wg.Done()
					s.queryRendezvous(queryCtx, fullKey, func(peer discoveryif.PeerInfo) {
						resultMu.Lock()
						defer resultMu.Unlock()
						s.mergeResult(results, peer, discoveryif.SourceRendezvous, sourcePriority)
					})
				}()

			case discoveryif.SourceLocal:
				if query.IncludeLocal {
					wg.Add(1)
					go func() {
						defer wg.Done()
						s.queryLocal(queryCtx, query.RealmID, func(peer discoveryif.PeerInfo) {
							resultMu.Lock()
							defer resultMu.Unlock()
							s.mergeResult(results, peer, discoveryif.SourceLocal, sourcePriority)
						})
					}()
				}
			}
		}

		// 等待所有查询完成或超时
		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-queryCtx.Done():
		case <-done:
		}

		// 发送结果（按优先级排序）
		resultMu.Lock()
		sortedResults := make([]peerResult, 0, len(results))
		for _, r := range results {
			sortedResults = append(sortedResults, r)
		}
		resultMu.Unlock()

		// 按优先级排序
		for i := 0; i < len(sortedResults); i++ {
			for j := i + 1; j < len(sortedResults); j++ {
				if sortedResults[i].priority > sortedResults[j].priority {
					sortedResults[i], sortedResults[j] = sortedResults[j], sortedResults[i]
				}
			}
		}

		// 应用 Limit
		count := 0
		for _, r := range sortedResults {
			if query.Limit > 0 && count >= query.Limit {
				break
			}

			select {
			case <-ctx.Done():
				return
			case ch <- discoveryif.DiscoveryResult{
				PeerInfo: r.info,
				Source:   r.source,
			}:
				count++
			}
		}
	}()

	return ch, nil
}

// RegisterService 统一注册入口
//
// 根据 Registration 参数向指定来源（provider、rendezvous）注册。
// 服务层会自动续约直到调用 UnregisterService。
func (s *DiscoveryService) RegisterService(ctx context.Context, reg discoveryif.Registration) error {
	// 应用默认值
	if len(reg.Sources) == 0 {
		reg.Sources = []discoveryif.Source{discoveryif.SourceProvider, discoveryif.SourceRendezvous}
	}
	if reg.TTL <= 0 {
		reg.TTL = 2 * time.Hour
	}
	// 应用 TTL 上限
	if reg.TTL > s.config.RendezvousMaxTTL {
		reg.TTL = s.config.RendezvousMaxTTL
	}

	// 解析作用域和命名空间
	resolvedScope, resolvedNS := s.resolveScope(reg.Namespace, reg.Scope, reg.RealmID)
	fullKey := s.buildDiscoveryKey(resolvedScope, resolvedNS, reg.RealmID)

	var lastErr error

	for _, source := range reg.Sources {
		switch source {
		case discoveryif.SourceProvider:
			if err := s.registerToProvider(ctx, fullKey, reg.TTL); err != nil {
				log.Debug("注册到 Provider 失败", "key", fullKey, "err", err)
				lastErr = err
			}

		case discoveryif.SourceRendezvous:
			if err := s.registerToRendezvous(ctx, fullKey, reg.TTL); err != nil {
				log.Debug("注册到 Rendezvous 失败", "key", fullKey, "err", err)
				lastErr = err
			}
		}
	}

	// 记录活跃注册以便自动续约
	s.recordActiveRegistration(reg)

	return lastErr
}

// UnregisterService 统一注销入口
//
// 取消注册并停止自动续约。
func (s *DiscoveryService) UnregisterService(ctx context.Context, reg discoveryif.Registration) error {
	// 应用默认值
	if len(reg.Sources) == 0 {
		reg.Sources = []discoveryif.Source{discoveryif.SourceProvider, discoveryif.SourceRendezvous}
	}

	// 解析作用域和命名空间
	resolvedScope, resolvedNS := s.resolveScope(reg.Namespace, reg.Scope, reg.RealmID)
	fullKey := s.buildDiscoveryKey(resolvedScope, resolvedNS, reg.RealmID)

	var lastErr error

	for _, source := range reg.Sources {
		switch source {
		case discoveryif.SourceProvider:
			// DHT Provider 无主动取消机制，等待 TTL 过期即可
			log.Debug("Provider 注册将在 TTL 后过期", "key", fullKey)

		case discoveryif.SourceRendezvous:
			if err := s.unregisterFromRendezvous(ctx, fullKey); err != nil {
				log.Debug("从 Rendezvous 注销失败", "key", fullKey, "err", err)
				lastErr = err
			}
		}
	}

	// 移除活跃注册
	s.removeActiveRegistration(reg)

	return lastErr
}

// ============================================================================
//                              内部辅助方法
// ============================================================================

// resolveScope 解析实际作用域
//
// 规则：
//  1. 如果 Namespace 以 "sys:" 前缀开头，强制使用 ScopeSys
//  2. 否则若当前已 JoinRealm，默认使用 ScopeRealm
//  3. 否则默认使用 ScopeSys
func (s *DiscoveryService) resolveScope(namespace string, scope discoveryif.Scope, explicitRealmID types.RealmID) (discoveryif.Scope, string) {
	// 检查 sys: 前缀
	if strings.HasPrefix(namespace, "sys:") {
		return discoveryif.ScopeSys, strings.TrimPrefix(namespace, "sys:")
	}

	if scope != discoveryif.ScopeAuto {
		return scope, namespace
	}

	// Auto 模式：检查是否有 Realm
	s.mu.RLock()
	currentRealm := s.currentRealm
	s.mu.RUnlock()

	if explicitRealmID != "" {
		return discoveryif.ScopeRealm, namespace
	}

	if currentRealm != "" && currentRealm != types.DefaultRealmID {
		return discoveryif.ScopeRealm, namespace
	}

	return discoveryif.ScopeSys, namespace
}

// buildDiscoveryKey 构建完整的发现 Key
//
// sys key：dep2p/v1/sys/{namespace}
// realm key：dep2p/v1/realm/{realmID}/{namespace}
func (s *DiscoveryService) buildDiscoveryKey(scope discoveryif.Scope, namespace string, explicitRealmID types.RealmID) string {
	switch scope {
	case discoveryif.ScopeSys:
		return "dep2p/v1/sys/" + namespace

	case discoveryif.ScopeRealm:
		realmID := explicitRealmID
		if realmID == "" {
			s.mu.RLock()
			realmID = s.currentRealm
			s.mu.RUnlock()
		}
		return "dep2p/v1/realm/" + string(realmID) + "/" + namespace

	default:
		return "dep2p/v1/sys/" + namespace
	}
}

// peerResult 发现结果（内部使用）
type peerResult struct {
	info     discoveryif.PeerInfo
	source   discoveryif.Source
	priority int // 数值越小优先级越高
}

// mergeResult 合并结果（保留高优先级）
func (s *DiscoveryService) mergeResult(
	results map[types.NodeID]peerResult,
	peer discoveryif.PeerInfo,
	source discoveryif.Source,
	sourcePriority map[discoveryif.Source]int,
) {
	priority := sourcePriority[source]

	existing, exists := results[peer.ID]
	if !exists || priority < existing.priority {
		results[peer.ID] = peerResult{
			info:     peer,
			source:   source,
			priority: priority,
		}
	}
}

// queryProvider 从 DHT Provider 查询
func (s *DiscoveryService) queryProvider(ctx context.Context, key string, emit func(discoveryif.PeerInfo)) {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	nsDiscoverers := make(map[string]discoveryif.NamespaceDiscoverer, len(s.namespaceDiscoverers))
	for k, v := range s.namespaceDiscoverers {
		nsDiscoverers[k] = v
	}
	s.mu.RUnlock()

	// 优先使用 DHT
	for name, d := range discoverers {
		if strings.Contains(strings.ToLower(name), "dht") {
			if nsd, ok := d.(discoveryif.NamespaceDiscoverer); ok {
				subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				ch, err := nsd.DiscoverPeers(subCtx, key)
				if err != nil {
					cancel()
					continue
				}
				for peer := range ch {
					emit(peer)
				}
				cancel()
				return
			}
		}
	}

	// 回退：查询所有 namespace discoverers
	for _, nsd := range nsDiscoverers {
		subCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		ch, err := nsd.DiscoverPeers(subCtx, key)
		if err != nil {
			cancel()
			continue
		}
		for peer := range ch {
			emit(peer)
		}
		cancel()
	}
}

// queryRendezvous 从 Rendezvous 查询
func (s *DiscoveryService) queryRendezvous(ctx context.Context, key string, emit func(discoveryif.PeerInfo)) {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	// 查找 Rendezvous discoverer
	for name, d := range discoverers {
		if strings.Contains(strings.ToLower(name), "rendezvous") {
			if rv, ok := d.(discoveryif.Rendezvous); ok {
				peers, err := rv.Discover(ctx, key, 0)
				if err != nil {
					log.Debug("Rendezvous 查询失败", "key", key, "err", err)
					continue
				}
				for _, peer := range peers {
					emit(peer)
				}
				return
			}
		}
	}
}

// queryLocal 从本地缓存查询
func (s *DiscoveryService) queryLocal(ctx context.Context, realmID types.RealmID, emit func(discoveryif.PeerInfo)) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for id, info := range s.knownPeers {
		// 如果指定了 RealmID，过滤不匹配的节点
		if realmID != "" && info.RealmID != realmID {
			continue
		}

		// 应用 Realm 隔离过滤
		if s.shouldFilterByRealm(info.RealmID) {
			continue
		}

		addrs := make([]string, len(info.Addresses))
		for i, addr := range info.Addresses {
			addrs[i] = addr.String()
		}

		select {
		case <-ctx.Done():
			return
		default:
			emit(discoveryif.PeerInfo{
				ID:    id,
				Addrs: types.StringsToMultiaddrs(addrs),
			})
		}
	}

	// 补充路由表节点（如果有 DHT）
	s.mu.RUnlock()
	s.mu.RLock()
	for _, d := range s.discoverers {
		if dht, ok := d.(discoveryif.DHT); ok {
			rt := dht.RoutingTable()
			if rt == nil {
				continue
			}
			for _, nodeID := range rt.Peers() {
				select {
				case <-ctx.Done():
					return
				default:
					emit(discoveryif.PeerInfo{
						ID:    nodeID,
						Addrs: nil, // 路由表不一定有地址
					})
				}
			}
		}
	}
}

// registerToProvider 注册到 DHT Provider
func (s *DiscoveryService) registerToProvider(ctx context.Context, key string, ttl time.Duration) error {
	s.mu.RLock()
	announcers := make(map[string]discoveryif.Announcer, len(s.announcers))
	for k, v := range s.announcers {
		announcers[k] = v
	}
	s.mu.RUnlock()

	// 使用 DHT announcer
	for name, a := range announcers {
		if strings.Contains(strings.ToLower(name), "dht") {
			return a.AnnounceWithTTL(ctx, key, ttl)
		}
	}

	// 回退：使用任何 announcer
	for _, a := range announcers {
		return a.AnnounceWithTTL(ctx, key, ttl)
	}

	return nil
}

// registerToRendezvous 注册到 Rendezvous
func (s *DiscoveryService) registerToRendezvous(ctx context.Context, key string, ttl time.Duration) error {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	// 查找 Rendezvous discoverer
	for name, d := range discoverers {
		if strings.Contains(strings.ToLower(name), "rendezvous") {
			if rv, ok := d.(discoveryif.Rendezvous); ok {
				return rv.Register(ctx, key, ttl)
			}
		}
	}

	return nil
}

// unregisterFromRendezvous 从 Rendezvous 注销
func (s *DiscoveryService) unregisterFromRendezvous(ctx context.Context, key string) error {
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	// 查找 Rendezvous discoverer
	for name, d := range discoverers {
		if strings.Contains(strings.ToLower(name), "rendezvous") {
			if rv, ok := d.(discoveryif.Rendezvous); ok {
				return rv.Unregister(ctx, key)
			}
		}
	}

	return nil
}

// activeRegistration 活跃注册记录
type activeRegistration struct {
	reg       discoveryif.Registration
	fullKey   string
	expiresAt time.Time
}

// recordActiveRegistration 记录活跃注册
func (s *DiscoveryService) recordActiveRegistration(reg discoveryif.Registration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeRegistrations == nil {
		s.activeRegistrations = make(map[string]*activeRegistration)
	}

	resolvedScope, resolvedNS := s.resolveScope(reg.Namespace, reg.Scope, reg.RealmID)
	fullKey := s.buildDiscoveryKey(resolvedScope, resolvedNS, reg.RealmID)

	s.activeRegistrations[fullKey] = &activeRegistration{
		reg:       reg,
		fullKey:   fullKey,
		expiresAt: time.Now().Add(reg.TTL),
	}
}

// removeActiveRegistration 移除活跃注册
func (s *DiscoveryService) removeActiveRegistration(reg discoveryif.Registration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.activeRegistrations == nil {
		return
	}

	resolvedScope, resolvedNS := s.resolveScope(reg.Namespace, reg.Scope, reg.RealmID)
	fullKey := s.buildDiscoveryKey(resolvedScope, resolvedNS, reg.RealmID)

	delete(s.activeRegistrations, fullKey)
}

