package dht

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ProviderRecord Provider 记录
type ProviderRecord struct {
	// PeerID 节点 ID
	PeerID types.PeerID

	// Addrs 节点地址
	Addrs []string

	// ExpiresAt 过期时间
	ExpiresAt time.Time
}

// IsExpired 检查是否过期
func (pr *ProviderRecord) IsExpired() bool {
	return time.Now().After(pr.ExpiresAt)
}

// ProviderStore Provider 存储
type ProviderStore struct {
	// 存储 map: key -> []ProviderRecord
	store map[string][]*ProviderRecord

	mu sync.RWMutex
}

// NewProviderStore 创建 Provider 存储
func NewProviderStore() *ProviderStore {
	return &ProviderStore{
		store: make(map[string][]*ProviderRecord),
	}
}

// AddProvider 添加 Provider
func (ps *ProviderStore) AddProvider(key string, peerID types.NodeID, addrs []string, ttl time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	providers, exists := ps.store[key]
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
				ExpiresAt: time.Now().Add(ttl),
			}
			ps.store[key] = providers
			return
		}
	}

	// 添加新记录
	providers = append(providers, &ProviderRecord{
		PeerID:    types.PeerID(peerID),
		Addrs:     addrs,
		ExpiresAt: time.Now().Add(ttl),
	})

	ps.store[key] = providers
}

// GetProviders 获取 Providers
func (ps *ProviderStore) GetProviders(key string) []*ProviderRecord {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	providers, exists := ps.store[key]
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
func (ps *ProviderStore) RemoveProvider(key string, peerID types.NodeID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	providers, exists := ps.store[key]
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
		delete(ps.store, key)
	} else {
		ps.store[key] = newProviders
	}
}

// CleanupExpired 清理过期 Providers
func (ps *ProviderStore) CleanupExpired() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	count := 0
	for key, providers := range ps.store {
		var validProviders []*ProviderRecord
		for _, p := range providers {
			if !p.IsExpired() {
				validProviders = append(validProviders, p)
			} else {
				count++
			}
		}

		if len(validProviders) == 0 {
			delete(ps.store, key)
		} else {
			ps.store[key] = validProviders
		}
	}

	return count
}

// Size 返回 Provider 数量
func (ps *ProviderStore) Size() int {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	total := 0
	for _, providers := range ps.store {
		total += len(providers)
	}

	return total
}

// Clear 清空存储
func (ps *ProviderStore) Clear() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.store = make(map[string][]*ProviderRecord)
}

// UpdateLocalAddrs 更新本地节点的地址
//
// 当网络变化时调用，更新所有记录中本地节点的地址。
func (ps *ProviderStore) UpdateLocalAddrs(localID types.NodeID, addrs []string, ttl time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	for key, providers := range ps.store {
		for i, p := range providers {
			if p.PeerID == types.PeerID(localID) {
				// 更新地址和过期时间
				ps.store[key][i].Addrs = addrs
				ps.store[key][i].ExpiresAt = time.Now().Add(ttl)
			}
		}
	}
}
