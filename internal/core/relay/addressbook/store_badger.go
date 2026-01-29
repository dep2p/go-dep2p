package addressbook

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// entryWithTTL 带 TTL 的缓存条目
type entryWithTTL struct {
	entry     realmif.MemberEntry
	ttl       time.Duration
	createdAt time.Time
	expiresAt time.Time
}

// isExpired 检查条目是否过期
func (e *entryWithTTL) isExpired() bool {
	if e.ttl <= 0 {
		return false // 无限期
	}
	return time.Now().After(e.expiresAt)
}

// persistedMemberEntry 持久化的成员条目
type persistedMemberEntry struct {
	NodeID       string   `json:"node_id"`
	DirectAddrs  []string `json:"direct_addrs"`
	NATType      int      `json:"nat_type"`
	Capabilities []string `json:"capabilities"`
	Online       bool     `json:"online"`
	LastSeen     int64    `json:"last_seen"`  // Unix 纳秒
	LastUpdate   int64    `json:"last_update"` // Unix 纳秒
	TTL          int64    `json:"ttl"`         // 纳秒
	ExpiresAt    int64    `json:"expires_at"`  // Unix 纳秒
}

// BadgerStore BadgerDB 存储实现
//
// 实现 realmif.AddressBookStore 接口，使用 BadgerDB 持久化存储。
type BadgerStore struct {
	store      *kv.Store
	defaultTTL time.Duration
	mu         sync.RWMutex
	closed     atomic.Bool

	// 内存缓存（可选，加速读取）
	cache map[types.NodeID]*entryWithTTL
}

// NewBadgerStore 创建 BadgerDB 存储
//
// 参数:
//   - store: KV 存储实例（已带前缀 a/）
func NewBadgerStore(store *kv.Store) (*BadgerStore, error) {
	return NewBadgerStoreWithTTL(store, 24*time.Hour)
}

// NewBadgerStoreWithTTL 创建带自定义 TTL 的 BadgerDB 存储
func NewBadgerStoreWithTTL(store *kv.Store, defaultTTL time.Duration) (*BadgerStore, error) {
	s := &BadgerStore{
		store:      store,
		defaultTTL: defaultTTL,
		cache:      make(map[types.NodeID]*entryWithTTL),
	}

	// 从存储加载已有数据到缓存
	if err := s.loadFromStore(); err != nil {
		return nil, err
	}

	return s, nil
}

// loadFromStore 从存储加载数据
func (s *BadgerStore) loadFromStore() error {
	now := time.Now()

	return s.store.PrefixScan(nil, func(key, value []byte) bool {
		var persisted persistedMemberEntry
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		expiresAt := time.Unix(0, persisted.ExpiresAt)

		// 跳过已过期的记录
		if persisted.TTL > 0 && expiresAt.Before(now) {
			// 删除过期数据
			s.store.Delete(key)
			return true
		}

		// 转换为 MemberEntry
		entry, err := s.fromPersisted(&persisted)
		if err != nil {
			return true
		}

		s.cache[entry.NodeID] = &entryWithTTL{
			entry:     entry,
			ttl:       time.Duration(persisted.TTL),
			createdAt: time.Unix(0, persisted.LastUpdate),
			expiresAt: expiresAt,
		}

		return true
	})
}

// toPersisted 转换为持久化格式
func (s *BadgerStore) toPersisted(entry realmif.MemberEntry, ttl time.Duration, expiresAt time.Time) *persistedMemberEntry {
	addrs := make([]string, len(entry.DirectAddrs))
	for i, addr := range entry.DirectAddrs {
		if addr != nil {
			addrs[i] = addr.String()
		}
	}

	return &persistedMemberEntry{
		NodeID:       string(entry.NodeID),
		DirectAddrs:  addrs,
		NATType:      int(entry.NATType),
		Capabilities: entry.Capabilities,
		Online:       entry.Online,
		LastSeen:     entry.LastSeen.UnixNano(),
		LastUpdate:   entry.LastUpdate.UnixNano(),
		TTL:          int64(ttl),
		ExpiresAt:    expiresAt.UnixNano(),
	}
}

// fromPersisted 从持久化格式转换
func (s *BadgerStore) fromPersisted(p *persistedMemberEntry) (realmif.MemberEntry, error) {
	addrs := make([]types.Multiaddr, 0, len(p.DirectAddrs))
	for _, addrStr := range p.DirectAddrs {
		if addrStr != "" {
			addr, err := types.NewMultiaddr(addrStr)
			if err != nil {
				continue
			}
			addrs = append(addrs, addr)
		}
	}

	return realmif.MemberEntry{
		NodeID:       types.NodeID(p.NodeID),
		DirectAddrs:  addrs,
		NATType:      types.NATType(p.NATType),
		Capabilities: p.Capabilities,
		Online:       p.Online,
		LastSeen:     time.Unix(0, p.LastSeen),
		LastUpdate:   time.Unix(0, p.LastUpdate),
	}, nil
}

// Put 存储成员条目
func (s *BadgerStore) Put(ctx context.Context, entry realmif.MemberEntry) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	if entry.NodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	ttl := s.defaultTTL
	expiresAt := now.Add(ttl)

	// 更新缓存
	s.cache[entry.NodeID] = &entryWithTTL{
		entry:     entry,
		ttl:       ttl,
		createdAt: now,
		expiresAt: expiresAt,
	}

	// 持久化
	persisted := s.toPersisted(entry, ttl, expiresAt)
	return s.store.PutJSON([]byte(entry.NodeID), persisted)
}

// Get 获取成员条目
func (s *BadgerStore) Get(ctx context.Context, nodeID types.NodeID) (realmif.MemberEntry, bool, error) {
	if s.closed.Load() {
		return realmif.MemberEntry{}, false, ErrStoreClosed
	}

	if nodeID.IsEmpty() {
		return realmif.MemberEntry{}, false, ErrInvalidNodeID
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return realmif.MemberEntry{}, false, ctx.Err()
	default:
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 从缓存获取
	e, ok := s.cache[nodeID]
	if !ok {
		return realmif.MemberEntry{}, false, nil
	}

	// 检查是否过期
	if e.isExpired() {
		return realmif.MemberEntry{}, false, nil
	}

	return e.entry, true, nil
}

// Delete 删除成员条目
func (s *BadgerStore) Delete(ctx context.Context, nodeID types.NodeID) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	if nodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.cache, nodeID)

	// 从存储中删除
	return s.store.Delete([]byte(nodeID))
}

// List 列出所有成员条目
func (s *BadgerStore) List(ctx context.Context) ([]realmif.MemberEntry, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	entries := make([]realmif.MemberEntry, 0, len(s.cache))
	for _, e := range s.cache {
		if !e.isExpired() {
			entries = append(entries, e.entry)
		}
	}

	return entries, nil
}

// SetTTL 设置条目过期时间
func (s *BadgerStore) SetTTL(ctx context.Context, nodeID types.NodeID, ttl time.Duration) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	if nodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	e, ok := s.cache[nodeID]
	if !ok {
		return ErrMemberNotFound
	}

	e.ttl = ttl
	if ttl > 0 {
		e.expiresAt = time.Now().Add(ttl)
	} else {
		e.expiresAt = time.Time{} // 永不过期
	}

	// 更新持久化
	persisted := s.toPersisted(e.entry, e.ttl, e.expiresAt)
	return s.store.PutJSON([]byte(nodeID), persisted)
}

// CleanExpired 清理过期条目
func (s *BadgerStore) CleanExpired(ctx context.Context) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for nodeID, e := range s.cache {
		if e.isExpired() {
			delete(s.cache, nodeID)
			s.store.Delete([]byte(nodeID))
		}
	}

	return nil
}

// Close 关闭存储
func (s *BadgerStore) Close() error {
	if s.closed.Swap(true) {
		return nil // 已经关闭
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 清空缓存
	s.cache = make(map[types.NodeID]*entryWithTTL)

	return nil
}

// Len 返回条目数量（用于测试）
func (s *BadgerStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// SetDefaultTTL 设置默认 TTL（用于测试）
func (s *BadgerStore) SetDefaultTTL(ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.defaultTTL = ttl
}

// 确保实现了接口
var _ realmif.AddressBookStore = (*BadgerStore)(nil)
