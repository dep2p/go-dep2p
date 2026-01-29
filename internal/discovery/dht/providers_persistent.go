package dht

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// persistedProviderRecord 持久化的 Provider 记录
type persistedProviderRecord struct {
	// PeerID 节点 ID
	PeerID string `json:"peer_id"`
	// Addrs 节点地址
	Addrs []string `json:"addrs"`
	// ExpiresAt 过期时间（Unix 纳秒）
	ExpiresAt int64 `json:"expires_at"`
}

// PersistentProviderStore 持久化 Provider 存储
//
// 使用 BadgerDB 存储 Provider 数据。
// 键格式: {key}/{peerID}
type PersistentProviderStore struct {
	// store KV 存储（前缀 d/p/）
	store *kv.Store

	// 内存缓存: key -> []ProviderRecord
	cache map[string][]*ProviderRecord

	mu sync.RWMutex
}

// NewPersistentProviderStore 创建持久化 Provider 存储
//
// 参数:
//   - store: KV 存储实例（已带前缀 d/p/）
func NewPersistentProviderStore(store *kv.Store) (*PersistentProviderStore, error) {
	ps := &PersistentProviderStore{
		store: store,
		cache: make(map[string][]*ProviderRecord),
	}

	// 从存储加载已有数据
	if err := ps.loadFromStore(); err != nil {
		return nil, err
	}

	return ps, nil
}

// makeProviderKey 生成 provider 存储键
func makeProviderKey(key string, peerID types.PeerID) []byte {
	return []byte(key + "/" + string(peerID))
}

// parseProviderKey 解析 provider 存储键
func parseProviderKey(storeKey []byte) (key string, peerID types.PeerID) {
	s := string(storeKey)
	idx := strings.LastIndex(s, "/")
	if idx == -1 {
		return s, ""
	}
	return s[:idx], types.PeerID(s[idx+1:])
}

// loadFromStore 从存储加载 Provider 数据
func (ps *PersistentProviderStore) loadFromStore() error {
	now := time.Now()

	return ps.store.PrefixScan(nil, func(storeKey, value []byte) bool {
		var persisted persistedProviderRecord
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		expiresAt := time.Unix(0, persisted.ExpiresAt)

		// 跳过已过期的记录
		if expiresAt.Before(now) {
			// 删除过期数据
			ps.store.Delete(storeKey)
			return true
		}

		key, _ := parseProviderKey(storeKey)

		record := &ProviderRecord{
			PeerID:    types.PeerID(persisted.PeerID),
			Addrs:     persisted.Addrs,
			ExpiresAt: expiresAt,
		}

		ps.cache[key] = append(ps.cache[key], record)

		return true
	})
}

// AddProvider 添加 Provider
func (ps *PersistentProviderStore) AddProvider(key string, peerID types.NodeID, addrs []string, ttl time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	expiresAt := time.Now().Add(ttl)
	providers, exists := ps.cache[key]
	if !exists {
		providers = make([]*ProviderRecord, 0)
	}

	// 检查是否已存在
	for i, p := range providers {
		if p.PeerID == types.PeerID(peerID) {
			// 更新记录
			providers[i] = &ProviderRecord{
				PeerID:    types.PeerID(peerID),
				Addrs:     addrs,
				ExpiresAt: expiresAt,
			}
			ps.cache[key] = providers

			// 持久化
			ps.persistProvider(key, providers[i])
			return
		}
	}

	// 添加新记录
	record := &ProviderRecord{
		PeerID:    types.PeerID(peerID),
		Addrs:     addrs,
		ExpiresAt: expiresAt,
	}
	providers = append(providers, record)
	ps.cache[key] = providers

	// 持久化
	ps.persistProvider(key, record)
}

// persistProvider 持久化单个 Provider 记录
func (ps *PersistentProviderStore) persistProvider(key string, record *ProviderRecord) {
	storeKey := makeProviderKey(key, record.PeerID)

	persisted := persistedProviderRecord{
		PeerID:    string(record.PeerID),
		Addrs:     record.Addrs,
		ExpiresAt: record.ExpiresAt.UnixNano(),
	}

	ps.store.PutJSON(storeKey, &persisted)
}

// GetProviders 获取 Providers
func (ps *PersistentProviderStore) GetProviders(key string) []*ProviderRecord {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	providers, exists := ps.cache[key]
	if !exists {
		return nil
	}

	// 过滤过期记录
	var validProviders []*ProviderRecord
	for _, p := range providers {
		if !p.IsExpired() {
			validProviders = append(validProviders, p)
		}
	}

	return validProviders
}

// RemoveProvider 移除 Provider
func (ps *PersistentProviderStore) RemoveProvider(key string, peerID types.NodeID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	providers, exists := ps.cache[key]
	if !exists {
		return
	}

	// 过滤掉指定 peerID
	var newProviders []*ProviderRecord
	for _, p := range providers {
		if p.PeerID != types.PeerID(peerID) {
			newProviders = append(newProviders, p)
		}
	}

	if len(newProviders) == 0 {
		delete(ps.cache, key)
	} else {
		ps.cache[key] = newProviders
	}

	// 从存储中删除
	storeKey := makeProviderKey(key, types.PeerID(peerID))
	ps.store.Delete(storeKey)
}

// CleanupExpired 清理过期 Providers
func (ps *PersistentProviderStore) CleanupExpired() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	count := 0
	for key, providers := range ps.cache {
		var validProviders []*ProviderRecord
		for _, p := range providers {
			if !p.IsExpired() {
				validProviders = append(validProviders, p)
			} else {
				// 从存储中删除
				storeKey := makeProviderKey(key, p.PeerID)
				ps.store.Delete(storeKey)
				count++
			}
		}

		if len(validProviders) == 0 {
			delete(ps.cache, key)
		} else {
			ps.cache[key] = validProviders
		}
	}

	return count
}

// Size 返回 Provider 数量
func (ps *PersistentProviderStore) Size() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	total := 0
	for _, providers := range ps.cache {
		total += len(providers)
	}

	return total
}

// Clear 清空存储
func (ps *PersistentProviderStore) Clear() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 删除所有键
	for key, providers := range ps.cache {
		for _, p := range providers {
			storeKey := makeProviderKey(key, p.PeerID)
			ps.store.Delete(storeKey)
		}
	}

	ps.cache = make(map[string][]*ProviderRecord)
}

// UpdateLocalAddrs 更新本地节点的地址
func (ps *PersistentProviderStore) UpdateLocalAddrs(localID types.NodeID, addrs []string, ttl time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	expiresAt := time.Now().Add(ttl)

	for key, providers := range ps.cache {
		for i, p := range providers {
			if p.PeerID == types.PeerID(localID) {
				// 更新地址和过期时间
				ps.cache[key][i].Addrs = addrs
				ps.cache[key][i].ExpiresAt = expiresAt

				// 持久化
				ps.persistProvider(key, ps.cache[key][i])
			}
		}
	}
}

// Ensure PersistentProviderStore implements the same interface as ProviderStore
var _ interface {
	AddProvider(string, types.NodeID, []string, time.Duration)
	GetProviders(string) []*ProviderRecord
	RemoveProvider(string, types.NodeID)
	CleanupExpired() int
	Size() int
	Clear()
	UpdateLocalAddrs(types.NodeID, []string, time.Duration)
} = (*PersistentProviderStore)(nil)
