// Package dht 提供分布式哈希表实现
//
// 本文件实现 PeerRecord 存储器，用于 DHT 权威目录。
package dht

import (
	"sync"
	"time"
)

// ============================================================================
//                              PeerRecordStore 存储器
// ============================================================================

// PeerRecordStore PeerRecord 存储器
//
// 存储签名的 RealmPeerRecord，支持验证和冲突解决。
// 线程安全。
type PeerRecordStore struct {
	mu        sync.RWMutex
	records   map[string]*SignedRealmPeerRecord // key -> record
	validator PeerRecordValidator
}

// NewPeerRecordStore 创建 PeerRecord 存储器
func NewPeerRecordStore() *PeerRecordStore {
	return &PeerRecordStore{
		records:   make(map[string]*SignedRealmPeerRecord),
		validator: NewDefaultPeerRecordValidator(),
	}
}

// NewPeerRecordStoreWithValidator 创建带自定义验证器的存储器
func NewPeerRecordStoreWithValidator(validator PeerRecordValidator) *PeerRecordStore {
	return &PeerRecordStore{
		records:   make(map[string]*SignedRealmPeerRecord),
		validator: validator,
	}
}

// Put 存储 PeerRecord
//
// 验证规则：
//  1. 验证签名
//  2. 验证 Key 匹配
//  3. 验证 TTL
//  4. 冲突解决（seq 更大的获胜）
//
// 返回：
//   - replaced: 是否替换了旧记录
//   - error: 验证失败时返回错误
func (s *PeerRecordStore) Put(key string, signed *SignedRealmPeerRecord) (replaced bool, err error) {
	// 验证
	if err := s.validator.Validate(key, signed); err != nil {
		return false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查是否存在旧记录
	oldRecord, exists := s.records[key]
	if !exists {
		s.records[key] = signed
		return false, nil
	}

	// 冲突解决
	shouldReplace, err := ShouldReplace(key, oldRecord, signed)
	if err != nil {
		return false, err
	}

	if shouldReplace {
		s.records[key] = signed
		return true, nil
	}

	return false, ErrSeqTooOld
}

// Get 获取 PeerRecord
//
// 返回：
//   - record: 找到的记录（未过期）
//   - exists: 是否存在
func (s *PeerRecordStore) Get(key string) (*SignedRealmPeerRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if record.Record.IsExpired() {
		return nil, false
	}

	return record, true
}

// GetWithExpired 获取 PeerRecord（包括已过期的）
func (s *PeerRecordStore) GetWithExpired(key string) (*SignedRealmPeerRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, exists := s.records[key]
	return record, exists
}

// Delete 删除 PeerRecord
func (s *PeerRecordStore) Delete(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.records[key]
	if exists {
		delete(s.records, key)
	}
	return exists
}

// Size 返回存储的记录数
func (s *PeerRecordStore) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.records)
}

// Keys 返回所有 Key
func (s *PeerRecordStore) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.records))
	for k := range s.records {
		keys = append(keys, k)
	}
	return keys
}

// CleanupExpired 清理过期记录
//
// 返回清理的记录数
func (s *PeerRecordStore) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for key, record := range s.records {
		if record.Record.IsExpired() {
			delete(s.records, key)
			count++
		}
	}
	return count
}

// GetAllValidRecords 获取所有有效（未过期）记录
func (s *PeerRecordStore) GetAllValidRecords() map[string]*SignedRealmPeerRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*SignedRealmPeerRecord)
	for key, record := range s.records {
		if !record.Record.IsExpired() {
			result[key] = record
		}
	}
	return result
}

// ============================================================================
//                              统计信息
// ============================================================================

// Stats 存储统计信息
type PeerRecordStoreStats struct {
	// TotalRecords 总记录数
	TotalRecords int

	// ValidRecords 有效记录数
	ValidRecords int

	// ExpiredRecords 过期记录数
	ExpiredRecords int

	// OldestTimestamp 最老记录的时间戳
	OldestTimestamp time.Time

	// NewestTimestamp 最新记录的时间戳
	NewestTimestamp time.Time
}

// Stats 获取统计信息
func (s *PeerRecordStore) Stats() PeerRecordStoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := PeerRecordStoreStats{
		TotalRecords: len(s.records),
	}

	var oldest, newest int64

	for _, record := range s.records {
		if record.Record.IsExpired() {
			stats.ExpiredRecords++
		} else {
			stats.ValidRecords++
		}

		ts := record.Record.Timestamp
		if oldest == 0 || ts < oldest {
			oldest = ts
		}
		if ts > newest {
			newest = ts
		}
	}

	if oldest > 0 {
		stats.OldestTimestamp = time.Unix(0, oldest)
	}
	if newest > 0 {
		stats.NewestTimestamp = time.Unix(0, newest)
	}

	return stats
}

// ============================================================================
//                              迭代器
// ============================================================================

// ForEach 遍历所有记录
//
// 如果回调返回 false，则停止遍历
func (s *PeerRecordStore) ForEach(fn func(key string, record *SignedRealmPeerRecord) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for key, record := range s.records {
		if !fn(key, record) {
			break
		}
	}
}

// ForEachValid 遍历所有有效记录
func (s *PeerRecordStore) ForEachValid(fn func(key string, record *SignedRealmPeerRecord) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for key, record := range s.records {
		if !record.Record.IsExpired() {
			if !fn(key, record) {
				break
			}
		}
	}
}
