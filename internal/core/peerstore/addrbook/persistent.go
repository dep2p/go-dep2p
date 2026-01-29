// Package addrbook 实现地址簿
package addrbook

import (
	"container/heap"
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// persistedAddr 持久化的地址数据
type persistedAddr struct {
	Addr    string        `json:"addr"`
	TTL     int64         `json:"ttl"`     // 纳秒
	Expiry  int64         `json:"expiry"`  // Unix 纳秒时间戳
	Source  AddressSource `json:"source"`
}

// persistedPeerAddrs 节点的所有持久化地址
type persistedPeerAddrs struct {
	Addrs []persistedAddr `json:"addrs"`
}

// PersistentAddrBook 持久化地址簿
//
// 使用 BadgerDB 存储地址数据，同时保留内存缓存以支持快速查询和 TTL 管理。
type PersistentAddrBook struct {
	mu sync.RWMutex

	// store KV 存储（前缀 p/a/）
	store *kv.Store

	// 内存缓存：PeerID → (addr string → *expiringAddr)
	addrs map[types.PeerID]map[string]*expiringAddr

	// expiringHeap 过期地址堆
	expiringHeap expiringAddrHeap

	// gcCtx GC 上下文
	gcCtx    context.Context
	gcCancel context.CancelFunc

	// eventbus 事件总线（可选）
	eventbus pkgif.EventBus
}

// NewPersistent 创建持久化地址簿
//
// 参数:
//   - store: KV 存储实例（已带前缀 p/a/）
func NewPersistent(store *kv.Store) (*PersistentAddrBook, error) {
	ctx, cancel := context.WithCancel(context.Background())
	ab := &PersistentAddrBook{
		store:        store,
		addrs:        make(map[types.PeerID]map[string]*expiringAddr),
		expiringHeap: make(expiringAddrHeap, 0),
		gcCtx:        ctx,
		gcCancel:     cancel,
	}

	// 从存储加载已有数据
	if err := ab.loadFromStore(); err != nil {
		cancel()
		return nil, err
	}

	// 启动 GC
	go ab.gc()

	return ab, nil
}

// loadFromStore 从存储加载地址数据
func (ab *PersistentAddrBook) loadFromStore() error {
	now := time.Now()

	return ab.store.PrefixScan(nil, func(key, value []byte) bool {
		peerID := types.PeerID(key)

		var persisted persistedPeerAddrs
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		// 创建内存映射
		ab.addrs[peerID] = make(map[string]*expiringAddr)

		for _, pa := range persisted.Addrs {
			expiry := time.Unix(0, pa.Expiry)

			// 跳过已过期的地址
			if expiry.Before(now) {
				continue
			}

			addr, err := types.NewMultiaddr(pa.Addr)
			if err != nil {
				continue
			}

			ea := &expiringAddr{
				Addr:   addr,
				TTL:    time.Duration(pa.TTL),
				Expiry: expiry,
				PeerID: peerID,
				Source: pa.Source,
			}
			ab.addrs[peerID][pa.Addr] = ea
			heap.Push(&ab.expiringHeap, ea)
		}

		// 如果所有地址都过期了，删除节点条目
		if len(ab.addrs[peerID]) == 0 {
			delete(ab.addrs, peerID)
		}

		return true
	})
}

// persistPeer 持久化节点的地址数据
func (ab *PersistentAddrBook) persistPeer(peerID types.PeerID) error {
	peerAddrs := ab.addrs[peerID]

	// 如果没有地址，删除存储中的数据
	if len(peerAddrs) == 0 {
		return ab.store.Delete([]byte(peerID))
	}

	// 序列化地址
	persisted := persistedPeerAddrs{
		Addrs: make([]persistedAddr, 0, len(peerAddrs)),
	}

	for _, ea := range peerAddrs {
		persisted.Addrs = append(persisted.Addrs, persistedAddr{
			Addr:   ea.Addr.String(),
			TTL:    int64(ea.TTL),
			Expiry: ea.Expiry.UnixNano(),
			Source: ea.Source,
		})
	}

	return ab.store.PutJSON([]byte(peerID), &persisted)
}

// SetEventBus 设置事件总线
func (ab *PersistentAddrBook) SetEventBus(eventbus pkgif.EventBus) {
	ab.mu.Lock()
	defer ab.mu.Unlock()
	ab.eventbus = eventbus
}

// AddAddr 添加单个地址
func (ab *PersistentAddrBook) AddAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	ab.AddAddrs(peerID, []types.Multiaddr{addr}, ttl)
}

// AddAddrs 添加地址（默认来源为 SourceUnknown）
func (ab *PersistentAddrBook) AddAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {
	ab.addAddrsWithSource(peerID, addrs, ttl, SourceUnknown)
}

// AddAddrsWithSource 添加带来源的地址
func (ab *PersistentAddrBook) AddAddrsWithSource(peerID types.PeerID, addrs []AddressWithSource) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.addrs[peerID] == nil {
		ab.addrs[peerID] = make(map[string]*expiringAddr)
	}

	for _, aws := range addrs {
		ab.addAddrLocked(peerID, aws.Addr, aws.TTL, aws.Source)
	}

	// 持久化
	ab.persistPeer(peerID)
}

// addAddrsWithSource 内部方法：添加带来源的地址
func (ab *PersistentAddrBook) addAddrsWithSource(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration, source AddressSource) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	if ab.addrs[peerID] == nil {
		ab.addrs[peerID] = make(map[string]*expiringAddr)
	}

	for _, addr := range addrs {
		ab.addAddrLocked(peerID, addr, ttl, source)
	}

	// 持久化
	ab.persistPeer(peerID)
}

// addAddrLocked 内部方法：添加单个地址（已持有锁）
func (ab *PersistentAddrBook) addAddrLocked(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration, source AddressSource) {
	// 修复 B29: 防止 nil 地址导致 panic
	if addr == nil {
		return
	}
	addrStr := addr.String()
	expiry := time.Now().Add(ttl)

	// 如果地址已存在，更新 TTL 和来源
	if existing, ok := ab.addrs[peerID][addrStr]; ok {
		existing.TTL = ttl
		existing.Expiry = expiry
		existing.Source = source
		heap.Fix(&ab.expiringHeap, existing.heapIndex)
		return
	}

	// 添加新地址
	ea := &expiringAddr{
		Addr:   addr,
		TTL:    ttl,
		Expiry: expiry,
		PeerID: peerID,
		Source: source,
	}
	ab.addrs[peerID][addrStr] = ea

	// 添加到堆中
	heap.Push(&ab.expiringHeap, ea)

	// 发布地址添加事件
	if ab.eventbus != nil {
		go ab.emitAddrAdded(peerID, addr, ttl)
	}
}

// SetAddr 设置单个地址
func (ab *PersistentAddrBook) SetAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	ab.SetAddrs(peerID, []types.Multiaddr{addr}, ttl)
}

// SetAddrs 设置地址（覆盖）
func (ab *PersistentAddrBook) SetAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {
	ab.setAddrsWithSource(peerID, addrs, ttl, SourceUnknown)
}

// SetAddrsWithSource 设置带来源的地址（覆盖）
func (ab *PersistentAddrBook) SetAddrsWithSource(peerID types.PeerID, addrs []AddressWithSource) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	// 清除旧地址
	ab.clearAddrsLocked(peerID)

	// 设置新地址
	ab.addrs[peerID] = make(map[string]*expiringAddr)
	for _, aws := range addrs {
		expiry := time.Now().Add(aws.TTL)
		ea := &expiringAddr{
			Addr:   aws.Addr,
			TTL:    aws.TTL,
			Expiry: expiry,
			PeerID: peerID,
			Source: aws.Source,
		}
		ab.addrs[peerID][aws.Addr.String()] = ea
		heap.Push(&ab.expiringHeap, ea)
	}

	// 持久化
	ab.persistPeer(peerID)
}

// setAddrsWithSource 内部方法：设置带来源的地址
func (ab *PersistentAddrBook) setAddrsWithSource(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration, source AddressSource) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	// 清除旧地址
	ab.clearAddrsLocked(peerID)

	// 设置新地址
	ab.addrs[peerID] = make(map[string]*expiringAddr)
	expiry := time.Now().Add(ttl)
	for _, addr := range addrs {
		ea := &expiringAddr{
			Addr:   addr,
			TTL:    ttl,
			Expiry: expiry,
			PeerID: peerID,
			Source: source,
		}
		ab.addrs[peerID][addr.String()] = ea
		heap.Push(&ab.expiringHeap, ea)
	}

	// 持久化
	ab.persistPeer(peerID)
}

// clearAddrsLocked 内部方法：清除地址（已持有锁）
func (ab *PersistentAddrBook) clearAddrsLocked(peerID types.PeerID) {
	if old := ab.addrs[peerID]; old != nil {
		for _, ea := range old {
			if ea.heapIndex >= 0 {
				heap.Remove(&ab.expiringHeap, ea.heapIndex)
			}
		}
	}
}

// UpdateAddrs 更新地址 TTL
func (ab *PersistentAddrBook) UpdateAddrs(peerID types.PeerID, oldTTL time.Duration, newTTL time.Duration) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		return
	}

	newExpiry := time.Now().Add(newTTL)
	updated := false
	for _, ea := range peerAddrs {
		if ea.TTL == oldTTL {
			ea.TTL = newTTL
			ea.Expiry = newExpiry
			heap.Fix(&ab.expiringHeap, ea.heapIndex)
			updated = true
		}
	}

	// 持久化
	if updated {
		ab.persistPeer(peerID)
	}
}

// ReduceAddrsTTL 降低所有地址的 TTL
//
// 
// 加速过期地址的清理。如果地址当前 TTL 已小于 maxTTL，则保持不变。
func (ab *PersistentAddrBook) ReduceAddrsTTL(peerID types.PeerID, maxTTL time.Duration) int {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		return 0
	}

	now := time.Now()
	newExpiry := now.Add(maxTTL)
	reducedCount := 0

	for _, ea := range peerAddrs {
		// 只有当当前过期时间大于新过期时间时才更新
		if ea.Expiry.After(newExpiry) {
			ea.TTL = maxTTL
			ea.Expiry = newExpiry
			heap.Fix(&ab.expiringHeap, ea.heapIndex)
			reducedCount++
		}
	}

	// 持久化
	if reducedCount > 0 {
		ab.persistPeer(peerID)
	}

	return reducedCount
}

// Addrs 获取地址（过滤过期地址）
func (ab *PersistentAddrBook) Addrs(peerID types.PeerID) []types.Multiaddr {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		return nil
	}

	now := time.Now()
	var result []types.Multiaddr
	for _, ea := range peerAddrs {
		if ea.Expiry.After(now) {
			result = append(result, ea.Addr)
		}
	}

	return result
}

// AddrsWithSource 获取带来源的地址（过滤过期地址）
func (ab *PersistentAddrBook) AddrsWithSource(peerID types.PeerID) []AddressWithSource {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		return nil
	}

	now := time.Now()
	var result []AddressWithSource
	for _, ea := range peerAddrs {
		if ea.Expiry.After(now) {
			result = append(result, AddressWithSource{
				Addr:   ea.Addr,
				Source: ea.Source,
				TTL:    ea.TTL,
			})
		}
	}

	return result
}

// GetAddrSource 获取特定地址的来源
func (ab *PersistentAddrBook) GetAddrSource(peerID types.PeerID, addr types.Multiaddr) AddressSource {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		return SourceUnknown
	}

	if ea, ok := peerAddrs[addr.String()]; ok {
		return ea.Source
	}

	return SourceUnknown
}

// AddrStream 返回地址流
func (ab *PersistentAddrBook) AddrStream(ctx context.Context, peerID types.PeerID) <-chan types.Multiaddr {
	ch := make(chan types.Multiaddr, 16)

	go func() {
		defer close(ch)

		// 发送现有地址
		addrs := ab.Addrs(peerID)
		for _, addr := range addrs {
			select {
			case ch <- addr:
			case <-ctx.Done():
				return
			}
		}

		// 监听新地址（通过订阅 AddrAddedEvent）
		if ab.eventbus != nil {
			sub, err := ab.eventbus.Subscribe(&AddrAddedEvent{})
			if err == nil {
				defer sub.Close()
				for {
					select {
					case ev := <-sub.Out():
						if addedEv, ok := ev.(*AddrAddedEvent); ok {
							if addedEv.PeerID == peerID {
								select {
								case ch <- addedEv.Addr:
								case <-ctx.Done():
									return
								}
							}
						}
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return ch
}

// emitAddrAdded 发布地址添加事件
func (ab *PersistentAddrBook) emitAddrAdded(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration) {
	emitter, err := ab.eventbus.Emitter(&AddrAddedEvent{})
	if err != nil {
		return
	}
	defer emitter.Close()

	emitter.Emit(&AddrAddedEvent{
		PeerID: peerID,
		Addr:   addr,
		TTL:    ttl,
	})
}

// emitAddrRemoved 发布地址移除事件
func (ab *PersistentAddrBook) emitAddrRemoved(peerID types.PeerID, addr types.Multiaddr) {
	emitter, err := ab.eventbus.Emitter(&AddrRemovedEvent{})
	if err != nil {
		return
	}
	defer emitter.Close()

	emitter.Emit(&AddrRemovedEvent{
		PeerID: peerID,
		Addr:   addr,
	})
}

// ClearAddrs 清除地址
func (ab *PersistentAddrBook) ClearAddrs(peerID types.PeerID) {
	ab.mu.Lock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		ab.mu.Unlock()
		return
	}

	// 从堆中移除
	for _, ea := range peerAddrs {
		if ea.heapIndex >= 0 {
			heap.Remove(&ab.expiringHeap, ea.heapIndex)
		}

		// 发布地址移除事件
		if ab.eventbus != nil {
			go ab.emitAddrRemoved(peerID, ea.Addr)
		}
	}

	delete(ab.addrs, peerID)
	ab.mu.Unlock()

	// 从存储中删除
	ab.store.Delete([]byte(peerID))
}

// ClearAddrsBySource 清除指定来源的地址
//
// 只移除来源匹配的地址，保留其他来源的地址。
func (ab *PersistentAddrBook) ClearAddrsBySource(peerID types.PeerID, source AddressSource) {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	peerAddrs := ab.addrs[peerID]
	if peerAddrs == nil {
		return
	}

	// 收集需要移除的地址键
	toRemove := make([]string, 0)
	for addrKey, ea := range peerAddrs {
		if ea.Source == source {
			toRemove = append(toRemove, addrKey)

			// 从堆中移除
			if ea.heapIndex >= 0 {
				heap.Remove(&ab.expiringHeap, ea.heapIndex)
			}

			// 发布地址移除事件
			if ab.eventbus != nil {
				go ab.emitAddrRemoved(peerID, ea.Addr)
			}
		}
	}

	// 从映射中移除
	for _, addrKey := range toRemove {
		delete(peerAddrs, addrKey)
	}

	// 如果该 peer 已没有地址，清理映射
	if len(peerAddrs) == 0 {
		delete(ab.addrs, peerID)
		// 从存储中删除
		ab.store.Delete([]byte(peerID))
	} else {
		// 否则持久化更新后的地址
		ab.persistPeer(peerID)
	}
}

// PeersWithAddrs 返回拥有地址的节点列表
func (ab *PersistentAddrBook) PeersWithAddrs() []types.PeerID {
	ab.mu.RLock()
	defer ab.mu.RUnlock()

	peers := make([]types.PeerID, 0, len(ab.addrs))
	for peerID := range ab.addrs {
		peers = append(peers, peerID)
	}

	return peers
}

// gc GC 清理过期地址
func (ab *PersistentAddrBook) gc() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ab.gcOnce()
		case <-ab.gcCtx.Done():
			return
		}
	}
}

// gcOnce 执行一次 GC
func (ab *PersistentAddrBook) gcOnce() {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	now := time.Now()
	removedPeers := make(map[types.PeerID]struct{})

	// 从堆中移除过期地址
	for ab.expiringHeap.Len() > 0 {
		ea := ab.expiringHeap[0]
		if ea.Expiry.After(now) {
			break
		}

		heap.Pop(&ab.expiringHeap)

		// 从 addrs map 中删除
		if peerAddrs := ab.addrs[ea.PeerID]; peerAddrs != nil {
			delete(peerAddrs, ea.Addr.String())
			removedPeers[ea.PeerID] = struct{}{}

			// 如果节点没有任何地址了，删除节点
			if len(peerAddrs) == 0 {
				delete(ab.addrs, ea.PeerID)
			}
		}
	}

	// 更新存储
	for peerID := range removedPeers {
		ab.persistPeer(peerID)
	}
}

// GCNow 立即执行一次 GC
func (ab *PersistentAddrBook) GCNow() {
	ab.gcOnce()
}

// ResetTemporaryAddrs 重置临时地址
func (ab *PersistentAddrBook) ResetTemporaryAddrs() int {
	ab.mu.Lock()
	defer ab.mu.Unlock()

	now := time.Now()
	threshold := 10 * time.Minute
	removedCount := 0
	modifiedPeers := make(map[types.PeerID]struct{})

	for peerID, peerAddrs := range ab.addrs {
		for addrKey, ea := range peerAddrs {
			// 检查地址的 TTL 是否较短（临时地址）
			ttl := ea.Expiry.Sub(now)
			if ttl > 0 && ttl < threshold {
				// 从堆中移除
				if ea.heapIndex >= 0 {
					heap.Remove(&ab.expiringHeap, ea.heapIndex)
				}
				delete(peerAddrs, addrKey)
				removedCount++
				modifiedPeers[peerID] = struct{}{}
			}
		}

		// 如果节点没有任何地址了，删除节点条目
		if len(peerAddrs) == 0 {
			delete(ab.addrs, peerID)
		}
	}

	// 更新存储
	for peerID := range modifiedPeers {
		ab.persistPeer(peerID)
	}

	return removedCount
}

// Close 关闭地址簿
func (ab *PersistentAddrBook) Close() error {
	ab.gcCancel()
	return nil
}

// Ensure PersistentAddrBook implements the same interface as AddrBook
var _ interface {
	AddAddr(types.PeerID, types.Multiaddr, time.Duration)
	AddAddrs(types.PeerID, []types.Multiaddr, time.Duration)
	SetAddr(types.PeerID, types.Multiaddr, time.Duration)
	SetAddrs(types.PeerID, []types.Multiaddr, time.Duration)
	UpdateAddrs(types.PeerID, time.Duration, time.Duration)
	Addrs(types.PeerID) []types.Multiaddr
	AddrStream(context.Context, types.PeerID) <-chan types.Multiaddr
	ClearAddrs(types.PeerID)
	PeersWithAddrs() []types.PeerID
	Close() error
} = (*PersistentAddrBook)(nil)

// IsNotFound 检查是否为 key not found 错误
func IsNotFound(err error) bool {
	return engine.IsNotFound(err)
}
