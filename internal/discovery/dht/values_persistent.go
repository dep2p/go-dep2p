package dht

import (
	"encoding/base64"
	"encoding/json"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
)

// persistedValueRecord 持久化的值记录
type persistedValueRecord struct {
	// Value Base64 编码的值
	Value string `json:"value"`
	// ExpiresAt 过期时间（Unix 纳秒）
	ExpiresAt int64 `json:"expires_at"`
}

// PersistentValueStore 持久化值存储
//
// 使用 BadgerDB 存储 DHT 值，同时保留内存缓存以支持快速查询。
type PersistentValueStore struct {
	// store KV 存储（前缀 d/v/）
	store *kv.Store

	// 内存缓存
	cache map[string]*ValueRecord

	mu sync.RWMutex
}

// NewPersistentValueStore 创建持久化值存储
//
// 参数:
//   - store: KV 存储实例（已带前缀 d/v/）
func NewPersistentValueStore(store *kv.Store) (*PersistentValueStore, error) {
	vs := &PersistentValueStore{
		store: store,
		cache: make(map[string]*ValueRecord),
	}

	// 从存储加载已有数据
	if err := vs.loadFromStore(); err != nil {
		return nil, err
	}

	return vs, nil
}

// loadFromStore 从存储加载值数据
func (vs *PersistentValueStore) loadFromStore() error {
	now := time.Now()

	return vs.store.PrefixScan(nil, func(key, value []byte) bool {
		var persisted persistedValueRecord
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		expiresAt := time.Unix(0, persisted.ExpiresAt)

		// 跳过已过期的记录
		if expiresAt.Before(now) {
			// 删除过期数据
			vs.store.Delete(key)
			return true
		}

		// 解码值
		valueBytes, err := base64.StdEncoding.DecodeString(persisted.Value)
		if err != nil {
			return true
		}

		vs.cache[string(key)] = &ValueRecord{
			Value:     valueBytes,
			ExpiresAt: expiresAt,
		}

		return true
	})
}

// Put 存储值
func (vs *PersistentValueStore) Put(key string, value []byte, ttl time.Duration) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	expiresAt := time.Now().Add(ttl)

	vs.cache[key] = &ValueRecord{
		Value:     value,
		ExpiresAt: expiresAt,
	}

	// 持久化
	persisted := persistedValueRecord{
		Value:     base64.StdEncoding.EncodeToString(value),
		ExpiresAt: expiresAt.UnixNano(),
	}

	vs.store.PutJSON([]byte(key), &persisted)
}

// Get 获取值
func (vs *PersistentValueStore) Get(key string) ([]byte, bool) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	record, exists := vs.cache[key]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if record.IsExpired() {
		return nil, false
	}

	return record.Value, true
}

// Delete 删除值
func (vs *PersistentValueStore) Delete(key string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	delete(vs.cache, key)

	// 从存储中删除
	vs.store.Delete([]byte(key))
}

// Size 返回存储的值数量
func (vs *PersistentValueStore) Size() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return len(vs.cache)
}

// CleanupExpired 清理过期值
func (vs *PersistentValueStore) CleanupExpired() int {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	count := 0
	for key, record := range vs.cache {
		if record.IsExpired() {
			delete(vs.cache, key)
			vs.store.Delete([]byte(key))
			count++
		}
	}

	return count
}

// Clear 清空存储
func (vs *PersistentValueStore) Clear() {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	// 删除所有键
	for key := range vs.cache {
		vs.store.Delete([]byte(key))
	}

	vs.cache = make(map[string]*ValueRecord)
}

// Ensure PersistentValueStore implements the same interface as ValueStore
var _ interface {
	Put(string, []byte, time.Duration)
	Get(string) ([]byte, bool)
	Delete(string)
	Size() int
	CleanupExpired() int
	Clear()
} = (*PersistentValueStore)(nil)

// IsNotFound 检查是否为 key not found 错误
func IsNotFound(err error) bool {
	return engine.IsNotFound(err)
}
