package dht

import (
	"sync"
	"time"
)

// ValueRecord 值记录
type ValueRecord struct {
	// Value 值
	Value []byte

	// ExpiresAt 过期时间
	ExpiresAt time.Time
}

// IsExpired 检查是否过期
func (vr *ValueRecord) IsExpired() bool {
	return time.Now().After(vr.ExpiresAt)
}

// ValueStore 值存储
type ValueStore struct {
	// 存储 map
	store map[string]*ValueRecord

	mu sync.RWMutex
}

// NewValueStore 创建值存储
func NewValueStore() *ValueStore {
	return &ValueStore{
		store: make(map[string]*ValueRecord),
	}
}

// Put 存储值
func (vs *ValueStore) Put(key string, value []byte, ttl time.Duration) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.store[key] = &ValueRecord{
		Value:     value,
		ExpiresAt: time.Now().Add(ttl),
	}
}

// Get 获取值
func (vs *ValueStore) Get(key string) ([]byte, bool) {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	record, exists := vs.store[key]
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
func (vs *ValueStore) Delete(key string) {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	delete(vs.store, key)
}

// Size 返回存储的值数量
func (vs *ValueStore) Size() int {
	vs.mu.RLock()
	defer vs.mu.RUnlock()

	return len(vs.store)
}

// CleanupExpired 清理过期值
func (vs *ValueStore) CleanupExpired() int {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	count := 0
	for key, record := range vs.store {
		if record.IsExpired() {
			delete(vs.store, key)
			count++
		}
	}

	return count
}

// Clear 清空存储
func (vs *ValueStore) Clear() {
	vs.mu.Lock()
	defer vs.mu.Unlock()

	vs.store = make(map[string]*ValueRecord)
}
