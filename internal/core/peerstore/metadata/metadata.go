// Package metadata 实现元数据存储
package metadata

import (
	"sync"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// MetadataStore 元数据存储
type MetadataStore struct {
	mu sync.RWMutex

	// data 元数据映射：PeerID → key → value
	data map[types.PeerID]map[string]interface{}
}

// New 创建元数据存储
func New() *MetadataStore {
	return &MetadataStore{
		data: make(map[types.PeerID]map[string]interface{}),
	}
}

// Get 获取元数据
func (ms *MetadataStore) Get(peerID types.PeerID, key string) (interface{}, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	if peerData, ok := ms.data[peerID]; ok {
		return peerData[key], nil
	}

	return nil, nil
}

// Put 存储元数据
func (ms *MetadataStore) Put(peerID types.PeerID, key string, val interface{}) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	if ms.data[peerID] == nil {
		ms.data[peerID] = make(map[string]interface{})
	}
	ms.data[peerID][key] = val

	return nil
}

// RemovePeer 移除节点元数据
func (ms *MetadataStore) RemovePeer(peerID types.PeerID) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	delete(ms.data, peerID)
}
