// Package kv 提供带前缀隔离的 KV 存储抽象层
//
// KVStore 在底层存储引擎之上提供命名空间隔离，
// 每个组件可以使用不同的前缀来隔离数据。
//
// # 键空间设计
//
// DeP2P 使用以下前缀约定：
//   - p/a/ - Peerstore 地址
//   - p/k/ - Peerstore 密钥
//   - p/p/ - Peerstore 协议
//   - p/m/ - Peerstore 元数据
//   - d/v/ - DHT 值存储
//   - d/p/ - DHT Provider
//   - d/r/ - DHT 路由表
//   - a/   - AddressBook
//   - r/   - Rendezvous
//   - m/   - Member
//
// # 使用示例
//
//	engine := badger.New(config)
//	peerstore := kv.New(engine, []byte("p/"))
//	dht := kv.New(engine, []byte("d/"))
//
//	// 写入数据（自动添加前缀）
//	peerstore.Put([]byte("a/peer1/0"), addrBytes)  // 实际键: p/a/peer1/0
//	dht.Put([]byte("v/key1"), valueBytes)          // 实际键: d/v/key1
package kv

import (
	"encoding/binary"
	"encoding/json"
	"sync"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
)

// Store 带前缀隔离的 KV 存储
//
// Store 封装底层存储引擎，为所有键自动添加前缀，
// 实现数据命名空间隔离。
type Store struct {
	engine engine.InternalEngine
	prefix []byte
	mu     sync.RWMutex
}

// New 创建新的 KVStore
//
// 参数:
//   - eng: 底层存储引擎
//   - prefix: 键前缀（所有操作会自动添加此前缀）
//
// 返回:
//   - *Store: 新创建的 KVStore
func New(eng engine.InternalEngine, prefix []byte) *Store {
	return &Store{
		engine: eng,
		prefix: prefix,
	}
}

// prefixKey 为键添加前缀
func (s *Store) prefixKey(key []byte) []byte {
	if len(s.prefix) == 0 {
		return key
	}
	prefixed := make([]byte, len(s.prefix)+len(key))
	copy(prefixed, s.prefix)
	copy(prefixed[len(s.prefix):], key)
	return prefixed
}

// stripPrefix 从键中移除前缀
func (s *Store) stripPrefix(key []byte) []byte {
	if len(s.prefix) == 0 || len(key) < len(s.prefix) {
		return key
	}
	return key[len(s.prefix):]
}

// ============= 基础操作 (S2-01) =============

// Get 获取指定键的值
func (s *Store) Get(key []byte) ([]byte, error) {
	return s.engine.Get(s.prefixKey(key))
}

// Put 设置键值对
func (s *Store) Put(key, value []byte) error {
	return s.engine.Put(s.prefixKey(key), value)
}

// Delete 删除指定键
func (s *Store) Delete(key []byte) error {
	return s.engine.Delete(s.prefixKey(key))
}

// Has 检查键是否存在
func (s *Store) Has(key []byte) (bool, error) {
	return s.engine.Has(s.prefixKey(key))
}

// ============= 便捷方法 (S2-02) =============

// GetJSON 获取并反序列化 JSON 值
func (s *Store) GetJSON(key []byte, v interface{}) error {
	data, err := s.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// PutJSON 序列化并存储 JSON 值
func (s *Store) PutJSON(key []byte, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.Put(key, data)
}

// GetUint64 获取 uint64 值
func (s *Store) GetUint64(key []byte) (uint64, error) {
	data, err := s.Get(key)
	if err != nil {
		return 0, err
	}
	if len(data) < 8 {
		return 0, engine.ErrCorrupted
	}
	return binary.BigEndian.Uint64(data), nil
}

// PutUint64 存储 uint64 值
func (s *Store) PutUint64(key []byte, value uint64) error {
	data := make([]byte, 8)
	binary.BigEndian.PutUint64(data, value)
	return s.Put(key, data)
}

// IncrUint64 原子递增 uint64 值
//
// 如果键不存在，初始化为 0 后再递增。
// 返回递增后的值。
func (s *Store) IncrUint64(key []byte, delta uint64) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.GetUint64(key)
	if err != nil && !engine.IsNotFound(err) {
		return 0, err
	}

	newValue := current + delta
	if err := s.PutUint64(key, newValue); err != nil {
		return 0, err
	}

	return newValue, nil
}

// DecrUint64 原子递减 uint64 值
//
// 如果结果会小于 0，返回 0。
// 返回递减后的值。
func (s *Store) DecrUint64(key []byte, delta uint64) (uint64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, err := s.GetUint64(key)
	if err != nil && !engine.IsNotFound(err) {
		return 0, err
	}

	var newValue uint64
	if current > delta {
		newValue = current - delta
	}

	if err := s.PutUint64(key, newValue); err != nil {
		return 0, err
	}

	return newValue, nil
}

// GetString 获取字符串值
func (s *Store) GetString(key []byte) (string, error) {
	data, err := s.Get(key)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// PutString 存储字符串值
func (s *Store) PutString(key []byte, value string) error {
	return s.Put(key, []byte(value))
}

// ============= 前缀迭代 (S2-03) =============

// PrefixScan 扫描指定前缀的所有键值对
//
// 回调函数返回 false 时停止扫描。
// 注意：返回的 key 已去除 Store 的前缀，但保留 subPrefix。
func (s *Store) PrefixScan(subPrefix []byte, fn func(key, value []byte) bool) error {
	fullPrefix := s.prefixKey(subPrefix)
	iter := s.engine.NewPrefixIterator(fullPrefix)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := s.stripPrefix(iter.Key())
		value := iter.Value()

		if !fn(key, value) {
			break
		}
	}

	return iter.Error()
}

// RangeScan 扫描指定范围的键值对
//
// 范围为 [startKey, endKey)，即包含 startKey，不包含 endKey。
// 如果 endKey 为 nil，则扫描到末尾。
func (s *Store) RangeScan(startKey, endKey []byte, fn func(key, value []byte) bool) error {
	opts := &engine.IteratorOptions{
		StartKey: s.prefixKey(startKey),
		EndKey:   nil,
	}

	if endKey != nil {
		opts.EndKey = s.prefixKey(endKey)
	}

	iter := s.engine.NewIterator(opts)
	defer iter.Close()

	for iter.First(); iter.Valid(); iter.Next() {
		key := s.stripPrefix(iter.Key())
		value := iter.Value()

		if !fn(key, value) {
			break
		}
	}

	return iter.Error()
}

// Keys 返回指定前缀的所有键
func (s *Store) Keys(subPrefix []byte) ([][]byte, error) {
	var keys [][]byte

	err := s.PrefixScan(subPrefix, func(key, _ []byte) bool {
		keyCopy := make([]byte, len(key))
		copy(keyCopy, key)
		keys = append(keys, keyCopy)
		return true
	})

	return keys, err
}

// Count 统计指定前缀的键数量
func (s *Store) Count(subPrefix []byte) (int64, error) {
	var count int64

	err := s.PrefixScan(subPrefix, func(_, _ []byte) bool {
		count++
		return true
	})

	return count, err
}

// DeletePrefix 删除指定前缀的所有键
func (s *Store) DeletePrefix(subPrefix []byte) error {
	keys, err := s.Keys(subPrefix)
	if err != nil {
		return err
	}

	batch := s.engine.NewBatch()
	for _, key := range keys {
		batch.Delete(s.prefixKey(key))
	}

	return s.engine.Write(batch)
}

// ============= 批量操作 =============

// Batch 带前缀的批量操作
type Batch struct {
	store *Store
	batch engine.Batch
}

// NewBatch 创建新的批量操作
func (s *Store) NewBatch() *Batch {
	return &Batch{
		store: s,
		batch: s.engine.NewBatch(),
	}
}

// Put 添加写入操作
func (b *Batch) Put(key, value []byte) {
	b.batch.Put(b.store.prefixKey(key), value)
}

// Delete 添加删除操作
func (b *Batch) Delete(key []byte) {
	b.batch.Delete(b.store.prefixKey(key))
}

// PutJSON 添加 JSON 写入操作
func (b *Batch) PutJSON(key []byte, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b.Put(key, data)
	return nil
}

// Write 执行批量操作
func (b *Batch) Write() error {
	return b.batch.Write()
}

// Reset 重置批量操作
func (b *Batch) Reset() {
	b.batch.Reset()
}

// Size 返回操作数量
func (b *Batch) Size() int {
	return b.batch.Size()
}

// ============= 事务操作 (S2-04) =============

// Transaction 带前缀的事务
type Transaction struct {
	store *Store
	txn   engine.Transaction
}

// NewTransaction 创建新的事务
//
// 参数:
//   - writable: true 表示读写事务，false 表示只读事务
func (s *Store) NewTransaction(writable bool) *Transaction {
	return &Transaction{
		store: s,
		txn:   s.engine.NewTransaction(writable),
	}
}

// Get 在事务中获取值
func (t *Transaction) Get(key []byte) ([]byte, error) {
	return t.txn.Get(t.store.prefixKey(key))
}

// Set 在事务中设置值
func (t *Transaction) Set(key, value []byte) error {
	return t.txn.Set(t.store.prefixKey(key), value)
}

// Delete 在事务中删除键
func (t *Transaction) Delete(key []byte) error {
	return t.txn.Delete(t.store.prefixKey(key))
}

// GetJSON 在事务中获取并反序列化 JSON
func (t *Transaction) GetJSON(key []byte, v interface{}) error {
	data, err := t.Get(key)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// SetJSON 在事务中序列化并存储 JSON
func (t *Transaction) SetJSON(key []byte, v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return t.Set(key, data)
}

// Commit 提交事务
func (t *Transaction) Commit() error {
	return t.txn.Commit()
}

// Discard 丢弃事务
func (t *Transaction) Discard() {
	t.txn.Discard()
}

// ============= 辅助方法 =============

// Prefix 返回当前 Store 的前缀
func (s *Store) Prefix() []byte {
	return s.prefix
}

// SubStore 创建子存储（在当前前缀基础上添加子前缀）
func (s *Store) SubStore(subPrefix []byte) *Store {
	newPrefix := make([]byte, len(s.prefix)+len(subPrefix))
	copy(newPrefix, s.prefix)
	copy(newPrefix[len(s.prefix):], subPrefix)

	return &Store{
		engine: s.engine,
		prefix: newPrefix,
	}
}

// Engine 返回底层存储引擎（仅供内部使用）
func (s *Store) Engine() engine.InternalEngine {
	return s.engine
}
