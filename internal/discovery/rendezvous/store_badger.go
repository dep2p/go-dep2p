package rendezvous

import (
	"encoding/binary"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              持久化注册记录
// ============================================================================

// persistedRegistration 持久化的注册记录格式
type badgerPersistedRegistration struct {
	Namespace    string   `json:"namespace"`
	PeerID       string   `json:"peer_id"`
	Addrs        []string `json:"addrs"`
	TTLNanos     int64    `json:"ttl_nanos"`
	RegisteredAt int64    `json:"registered_at"`
	ExpiresAt    int64    `json:"expires_at"`
	SignedRecord []byte   `json:"signed_record,omitempty"`
}

// ============================================================================
//                              BadgerStore 实现
// ============================================================================

// BadgerStore BadgerDB 存储实现
//
// 键格式: r/{namespace}/{peerID}
// 值格式: JSON 序列化的 badgerPersistedRegistration
type BadgerStore struct {
	store  *kv.Store
	config StoreConfig

	// 内存缓存（加速读取）
	// registrations: namespace -> peerID -> Registration
	registrations map[string]map[types.PeerID]*Registration
	// peerNamespaces: peerID -> set of namespaces
	peerNamespaces map[types.PeerID]map[string]struct{}

	mu sync.RWMutex

	// 统计
	totalRegistrations   int
	registrationsExpired uint64
	closed               atomic.Bool
}

// NewBadgerStore 创建 BadgerDB 存储
//
// 参数:
//   - store: KV 存储实例（已带前缀 r/）
//   - config: 存储配置
func NewBadgerStore(store *kv.Store, config StoreConfig) (*BadgerStore, error) {
	s := &BadgerStore{
		store:          store,
		config:         config,
		registrations:  make(map[string]map[types.PeerID]*Registration),
		peerNamespaces: make(map[types.PeerID]map[string]struct{}),
	}

	// 从存储加载已有数据
	if err := s.loadFromStore(); err != nil {
		return nil, err
	}

	return s, nil
}

// loadFromStore 从存储加载数据到内存缓存
func (s *BadgerStore) loadFromStore() error {
	now := time.Now()

	return s.store.PrefixScan(nil, func(key, value []byte) bool {
		var persisted badgerPersistedRegistration
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		expiresAt := time.Unix(0, persisted.ExpiresAt)

		// 跳过已过期的记录
		if expiresAt.Before(now) {
			// 删除过期数据
			s.store.Delete(key)
			return true
		}

		// 转换为 Registration
		reg := s.fromPersisted(&persisted)

		// 加入缓存
		s.addToCache(reg)

		return true
	})
}

// makeKey 构建存储键
func (s *BadgerStore) makeKey(namespace string, peerID types.PeerID) []byte {
	// 格式: {namespace}/{peerID}
	return []byte(namespace + "/" + string(peerID))
}

// toPersisted 转换为持久化格式
func (s *BadgerStore) toPersisted(reg *Registration) *badgerPersistedRegistration {
	addrs := make([]string, len(reg.PeerInfo.Addrs))
	for i, addr := range reg.PeerInfo.Addrs {
		if addr != nil {
			addrs[i] = addr.String()
		}
	}

	return &badgerPersistedRegistration{
		Namespace:    reg.Namespace,
		PeerID:       string(reg.PeerInfo.ID),
		Addrs:        addrs,
		TTLNanos:     int64(reg.TTL),
		RegisteredAt: reg.RegisteredAt.UnixNano(),
		ExpiresAt:    reg.ExpiresAt.UnixNano(),
		SignedRecord: reg.SignedRecord,
	}
}

// fromPersisted 从持久化格式转换
func (s *BadgerStore) fromPersisted(p *badgerPersistedRegistration) *Registration {
	addrs := make([]types.Multiaddr, 0, len(p.Addrs))
	for _, addrStr := range p.Addrs {
		if addrStr != "" {
			addr, err := types.NewMultiaddr(addrStr)
			if err == nil {
				addrs = append(addrs, addr)
			}
		}
	}

	return &Registration{
		Namespace: p.Namespace,
		PeerInfo: types.PeerInfo{
			ID:    types.PeerID(p.PeerID),
			Addrs: addrs,
		},
		TTL:          time.Duration(p.TTLNanos),
		RegisteredAt: time.Unix(0, p.RegisteredAt),
		ExpiresAt:    time.Unix(0, p.ExpiresAt),
		SignedRecord: p.SignedRecord,
	}
}

// addToCache 添加到缓存
func (s *BadgerStore) addToCache(reg *Registration) {
	if _, exists := s.registrations[reg.Namespace]; !exists {
		s.registrations[reg.Namespace] = make(map[types.PeerID]*Registration)
	}
	s.registrations[reg.Namespace][reg.PeerInfo.ID] = reg

	if _, exists := s.peerNamespaces[reg.PeerInfo.ID]; !exists {
		s.peerNamespaces[reg.PeerInfo.ID] = make(map[string]struct{})
	}
	s.peerNamespaces[reg.PeerInfo.ID][reg.Namespace] = struct{}{}

	s.totalRegistrations++
}

// removeFromCache 从缓存移除
func (s *BadgerStore) removeFromCache(namespace string, peerID types.PeerID) {
	if nsRegs, exists := s.registrations[namespace]; exists {
		if _, exists := nsRegs[peerID]; exists {
			delete(nsRegs, peerID)
			s.totalRegistrations--

			if len(nsRegs) == 0 {
				delete(s.registrations, namespace)
			}
		}
	}

	if namespaces, exists := s.peerNamespaces[peerID]; exists {
		delete(namespaces, namespace)
		if len(namespaces) == 0 {
			delete(s.peerNamespaces, peerID)
		}
	}
}

// ============================================================================
//                              Store 接口实现
// ============================================================================

// Add 添加或更新注册
func (s *BadgerStore) Add(namespace string, peerInfo types.PeerInfo, ttl time.Duration) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证 TTL
	if ttl <= 0 {
		ttl = s.config.DefaultTTL
	}
	if ttl > s.config.MaxTTL {
		ttl = s.config.MaxTTL
	}

	// 检查命名空间数量限制
	if _, exists := s.registrations[namespace]; !exists {
		if len(s.registrations) >= s.config.MaxNamespaces {
			return ErrMaxNamespacesExceeded
		}
	}

	nsRegs := s.registrations[namespace]
	if nsRegs == nil {
		nsRegs = make(map[types.PeerID]*Registration)
		s.registrations[namespace] = nsRegs
	}

	// 检查是否是更新
	_, isUpdate := nsRegs[peerInfo.ID]

	// 检查命名空间内注册数限制
	if !isUpdate && len(nsRegs) >= s.config.MaxRegistrationsPerNamespace {
		return ErrMaxRegistrationsPerNamespaceExceeded
	}

	// 检查总注册数限制
	if !isUpdate && s.totalRegistrations >= s.config.MaxRegistrations {
		return ErrMaxRegistrationsExceeded
	}

	// 检查单个节点注册数限制
	if !isUpdate {
		if namespaces, exists := s.peerNamespaces[peerInfo.ID]; exists {
			if len(namespaces) >= s.config.MaxRegistrationsPerPeer {
				return ErrMaxRegistrationsPerPeerExceeded
			}
		}
	}

	now := time.Now()

	// 创建注册记录
	reg := &Registration{
		Namespace:    namespace,
		PeerInfo:     peerInfo,
		TTL:          ttl,
		RegisteredAt: now,
		ExpiresAt:    now.Add(ttl),
	}

	// 持久化
	persisted := s.toPersisted(reg)
	key := s.makeKey(namespace, peerInfo.ID)
	if err := s.store.PutJSON(key, persisted); err != nil {
		return err
	}

	// 更新缓存
	nsRegs[peerInfo.ID] = reg

	// 更新 peer -> namespaces 索引
	if _, exists := s.peerNamespaces[peerInfo.ID]; !exists {
		s.peerNamespaces[peerInfo.ID] = make(map[string]struct{})
	}
	s.peerNamespaces[peerInfo.ID][namespace] = struct{}{}

	// 更新统计
	if !isUpdate {
		s.totalRegistrations++
	}

	return nil
}

// Remove 移除注册
func (s *BadgerStore) Remove(namespace string, peerID types.PeerID) {
	if s.closed.Load() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return
	}

	if _, exists := nsRegs[peerID]; !exists {
		return
	}

	// 从存储删除
	key := s.makeKey(namespace, peerID)
	s.store.Delete(key)

	// 从缓存移除
	s.removeFromCache(namespace, peerID)
}

// Get 查询命名空间中的注册（支持分页）
func (s *BadgerStore) Get(namespace string, limit int, cookie []byte) ([]*Registration, []byte, error) {
	if s.closed.Load() {
		return nil, nil, ErrStoreClosed
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return nil, nil, nil
	}

	// 解析 cookie（偏移量）
	offset := 0
	if len(cookie) >= 4 {
		offset = int(binary.BigEndian.Uint32(cookie))
	}

	// 收集有效注册
	var results []*Registration
	count := 0
	skipped := 0

	for _, reg := range nsRegs {
		if reg.IsExpired() {
			continue
		}

		// 跳过 offset 个
		if skipped < offset {
			skipped++
			continue
		}

		results = append(results, reg)
		count++

		// 检查限制
		if limit > 0 && count >= limit {
			break
		}
	}

	// 生成下一页 cookie
	var nextCookie []byte
	if limit > 0 && count >= limit && offset+count < len(nsRegs) {
		nextCookie = make([]byte, 4)
		binary.BigEndian.PutUint32(nextCookie, uint32(offset+count))
	}

	return results, nextCookie, nil
}

// GetPeerNamespaces 获取节点注册的所有命名空间
func (s *BadgerStore) GetPeerNamespaces(peerID types.PeerID) []string {
	if s.closed.Load() {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	namespaces, exists := s.peerNamespaces[peerID]
	if !exists {
		return nil
	}

	result := make([]string, 0, len(namespaces))
	for ns := range namespaces {
		result = append(result, ns)
	}
	return result
}

// CleanupExpired 清理过期注册
func (s *BadgerStore) CleanupExpired() int {
	if s.closed.Load() {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	expired := 0

	for namespace, nsRegs := range s.registrations {
		for peerID, reg := range nsRegs {
			if now.After(reg.ExpiresAt) {
				// 从存储删除
				key := s.makeKey(namespace, peerID)
				s.store.Delete(key)

				// 从缓存移除
				delete(nsRegs, peerID)
				s.totalRegistrations--
				expired++

				// 更新 peer -> namespaces 索引
				if namespaces, exists := s.peerNamespaces[peerID]; exists {
					delete(namespaces, namespace)
					if len(namespaces) == 0 {
						delete(s.peerNamespaces, peerID)
					}
				}
			}
		}

		// 清理空的命名空间
		if len(nsRegs) == 0 {
			delete(s.registrations, namespace)
		}
	}

	if expired > 0 {
		atomic.AddUint64(&s.registrationsExpired, uint64(expired))
	}

	return expired
}

// Stats 返回统计信息
func (s *BadgerStore) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Stats{
		TotalRegistrations:   s.totalRegistrations,
		TotalNamespaces:      len(s.registrations),
		RegistrationsExpired: atomic.LoadUint64(&s.registrationsExpired),
	}
}

// Close 关闭存储
func (s *BadgerStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.registrations = nil
	s.peerNamespaces = nil

	return nil
}
