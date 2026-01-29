// Package metadata 实现元数据存储
package metadata

import (
	"encoding/json"
	"sync"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// PersistentMetadataStore 持久化元数据存储
//
// 使用 BadgerDB 存储元数据，键格式为 {peerID}/{key}。
type PersistentMetadataStore struct {
	mu sync.RWMutex

	// store KV 存储（前缀 p/m/）
	store *kv.Store

	// 内存缓存
	data map[types.PeerID]map[string]interface{}
}

// NewPersistent 创建持久化元数据存储
//
// 参数:
//   - store: KV 存储实例（已带前缀 p/m/）
func NewPersistent(store *kv.Store) (*PersistentMetadataStore, error) {
	ms := &PersistentMetadataStore{
		store: store,
		data:  make(map[types.PeerID]map[string]interface{}),
	}

	// 从存储加载已有数据
	if err := ms.loadFromStore(); err != nil {
		return nil, err
	}

	return ms, nil
}

// loadFromStore 从存储加载元数据
func (ms *PersistentMetadataStore) loadFromStore() error {
	return ms.store.PrefixScan(nil, func(key, value []byte) bool {
		// 解析键: {peerID}/{metaKey}
		peerID, metaKey := parseMetaKey(key)
		if peerID == "" || metaKey == "" {
			return true
		}

		// 反序列化值
		var val interface{}
		if err := json.Unmarshal(value, &val); err != nil {
			// 跳过损坏的数据
			return true
		}

		// 存入缓存
		if ms.data[peerID] == nil {
			ms.data[peerID] = make(map[string]interface{})
		}
		ms.data[peerID][metaKey] = val

		return true
	})
}

// parseMetaKey 解析元数据键
//
// 格式: {peerID}/{metaKey}
func parseMetaKey(key []byte) (types.PeerID, string) {
	for i, b := range key {
		if b == '/' {
			return types.PeerID(key[:i]), string(key[i+1:])
		}
	}
	return "", ""
}

// makeKey 生成存储键
func makeKey(peerID types.PeerID, key string) []byte {
	return []byte(string(peerID) + "/" + key)
}

// Get 获取元数据
func (ms *PersistentMetadataStore) Get(peerID types.PeerID, key string) (interface{}, error) {
	ms.mu.RLock()
	if peerData, ok := ms.data[peerID]; ok {
		if val, ok := peerData[key]; ok {
			ms.mu.RUnlock()
			return val, nil
		}
	}
	ms.mu.RUnlock()

	// 尝试从存储加载
	storeKey := makeKey(peerID, key)
	data, err := ms.store.Get(storeKey)
	if err != nil {
		if engine.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	// 反序列化并缓存
	var val interface{}
	if err := json.Unmarshal(data, &val); err != nil {
		return nil, err
	}

	ms.mu.Lock()
	if ms.data[peerID] == nil {
		ms.data[peerID] = make(map[string]interface{})
	}
	ms.data[peerID][key] = val
	ms.mu.Unlock()

	return val, nil
}

// Put 存储元数据
func (ms *PersistentMetadataStore) Put(peerID types.PeerID, key string, val interface{}) error {
	ms.mu.Lock()
	if ms.data[peerID] == nil {
		ms.data[peerID] = make(map[string]interface{})
	}
	ms.data[peerID][key] = val
	ms.mu.Unlock()

	// 持久化
	storeKey := makeKey(peerID, key)
	return ms.store.PutJSON(storeKey, val)
}

// Delete 删除元数据
func (ms *PersistentMetadataStore) Delete(peerID types.PeerID, key string) error {
	ms.mu.Lock()
	if peerData, ok := ms.data[peerID]; ok {
		delete(peerData, key)
		if len(peerData) == 0 {
			delete(ms.data, peerID)
		}
	}
	ms.mu.Unlock()

	// 从存储中删除
	storeKey := makeKey(peerID, key)
	return ms.store.Delete(storeKey)
}

// RemovePeer 移除节点元数据
func (ms *PersistentMetadataStore) RemovePeer(peerID types.PeerID) {
	ms.mu.Lock()
	keys := make([]string, 0)
	if peerData, ok := ms.data[peerID]; ok {
		for key := range peerData {
			keys = append(keys, key)
		}
		delete(ms.data, peerID)
	}
	ms.mu.Unlock()

	// 从存储中删除所有键
	for _, key := range keys {
		storeKey := makeKey(peerID, key)
		ms.store.Delete(storeKey)
	}

	// 也可以使用前缀删除
	ms.store.DeletePrefix([]byte(string(peerID) + "/"))
}

// GetAll 获取节点的所有元数据
func (ms *PersistentMetadataStore) GetAll(peerID types.PeerID) map[string]interface{} {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	peerData := ms.data[peerID]
	if peerData == nil {
		return nil
	}

	// 返回副本
	result := make(map[string]interface{}, len(peerData))
	for k, v := range peerData {
		result[k] = v
	}

	return result
}

// PeersWithMetadata 返回拥有元数据的节点列表
func (ms *PersistentMetadataStore) PeersWithMetadata() []types.PeerID {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	peers := make([]types.PeerID, 0, len(ms.data))
	for peerID := range ms.data {
		peers = append(peers, peerID)
	}

	return peers
}

// Ensure PersistentMetadataStore implements the same interface as MetadataStore
var _ interface {
	Get(types.PeerID, string) (interface{}, error)
	Put(types.PeerID, string, interface{}) error
	RemovePeer(types.PeerID)
} = (*PersistentMetadataStore)(nil)
