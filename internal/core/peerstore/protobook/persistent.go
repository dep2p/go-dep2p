// Package protobook 实现协议簿
package protobook

import (
	"encoding/json"
	"sync"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// PersistentProtoBook 持久化协议簿
//
// 使用 BadgerDB 存储协议数据，同时保留内存缓存以支持快速查询。
type PersistentProtoBook struct {
	mu sync.RWMutex

	// store KV 存储（前缀 p/p/）
	store *kv.Store

	// 内存缓存
	protocols map[types.PeerID][]types.ProtocolID
}

// NewPersistent 创建持久化协议簿
//
// 参数:
//   - store: KV 存储实例（已带前缀 p/p/）
func NewPersistent(store *kv.Store) (*PersistentProtoBook, error) {
	pb := &PersistentProtoBook{
		store:     store,
		protocols: make(map[types.PeerID][]types.ProtocolID),
	}

	// 从存储加载已有数据
	if err := pb.loadFromStore(); err != nil {
		return nil, err
	}

	return pb, nil
}

// loadFromStore 从存储加载协议数据
func (pb *PersistentProtoBook) loadFromStore() error {
	return pb.store.PrefixScan(nil, func(key, value []byte) bool {
		peerID := types.PeerID(key)

		// 尝试从 JSON 数组解析
		var protoStrs []string
		if err := json.Unmarshal(value, &protoStrs); err != nil {
			// 跳过损坏的数据
			return true
		}

		protocols := make([]types.ProtocolID, len(protoStrs))
		for i, s := range protoStrs {
			protocols[i] = types.ProtocolID(s)
		}

		if len(protocols) > 0 {
			pb.protocols[peerID] = protocols
		}

		return true
	})
}

// persistPeer 持久化节点的协议数据
func (pb *PersistentProtoBook) persistPeer(peerID types.PeerID) error {
	protocols := pb.protocols[peerID]

	// 如果没有协议，删除存储中的数据
	if len(protocols) == 0 {
		return pb.store.Delete([]byte(peerID))
	}

	// 转换为字符串列表并序列化
	protoStrs := make([]string, len(protocols))
	for i, p := range protocols {
		protoStrs[i] = string(p)
	}

	return pb.store.PutJSON([]byte(peerID), protoStrs)
}

// GetProtocols 获取协议
func (pb *PersistentProtoBook) GetProtocols(peerID types.PeerID) ([]types.ProtocolID, error) {
	pb.mu.RLock()
	protocols := pb.protocols[peerID]
	pb.mu.RUnlock()

	if protocols != nil {
		// 返回副本
		result := make([]types.ProtocolID, len(protocols))
		copy(result, protocols)
		return result, nil
	}

	// 尝试从存储加载
	var protoStrs []string
	err := pb.store.GetJSON([]byte(peerID), &protoStrs)
	if err != nil {
		if engine.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}

	// 缓存并返回
	protocols = make([]types.ProtocolID, len(protoStrs))
	for i, s := range protoStrs {
		protocols[i] = types.ProtocolID(s)
	}

	pb.mu.Lock()
	pb.protocols[peerID] = protocols
	pb.mu.Unlock()

	// 返回副本
	result := make([]types.ProtocolID, len(protocols))
	copy(result, protocols)
	return result, nil
}

// AddProtocols 添加协议
func (pb *PersistentProtoBook) AddProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	existing := pb.protocols[peerID]

	// 避免重复
	for _, proto := range protocols {
		found := false
		for _, ep := range existing {
			if ep == proto {
				found = true
				break
			}
		}
		if !found {
			existing = append(existing, proto)
		}
	}

	pb.protocols[peerID] = existing

	// 持久化
	return pb.persistPeer(peerID)
}

// SetProtocols 设置协议（覆盖）
func (pb *PersistentProtoBook) SetProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.protocols[peerID] = append([]types.ProtocolID{}, protocols...)

	// 持久化
	return pb.persistPeer(peerID)
}

// RemoveProtocols 移除协议
func (pb *PersistentProtoBook) RemoveProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	existing := pb.protocols[peerID]
	if existing == nil {
		return nil
	}

	// 创建待移除协议的映射
	toRemove := make(map[types.ProtocolID]struct{})
	for _, proto := range protocols {
		toRemove[proto] = struct{}{}
	}

	// 过滤掉要移除的协议
	filtered := make([]types.ProtocolID, 0, len(existing))
	for _, proto := range existing {
		if _, ok := toRemove[proto]; !ok {
			filtered = append(filtered, proto)
		}
	}

	pb.protocols[peerID] = filtered

	// 持久化
	return pb.persistPeer(peerID)
}

// SupportsProtocols 查询支持的协议
func (pb *PersistentProtoBook) SupportsProtocols(peerID types.PeerID, protocols ...types.ProtocolID) ([]types.ProtocolID, error) {
	pb.mu.RLock()
	peerProtos := pb.protocols[peerID]
	pb.mu.RUnlock()

	if peerProtos == nil {
		return nil, nil
	}

	// 查找交集
	var supported []types.ProtocolID
	for _, proto := range protocols {
		for _, peerProto := range peerProtos {
			if proto == peerProto {
				supported = append(supported, proto)
				break
			}
		}
	}

	return supported, nil
}

// FirstSupportedProtocol 返回首个支持的协议
func (pb *PersistentProtoBook) FirstSupportedProtocol(peerID types.PeerID, protocols ...types.ProtocolID) (types.ProtocolID, error) {
	pb.mu.RLock()
	peerProtos := pb.protocols[peerID]
	pb.mu.RUnlock()

	if peerProtos == nil {
		return types.ProtocolID(""), nil
	}

	// 按顺序查找第一个匹配的协议
	for _, proto := range protocols {
		for _, peerProto := range peerProtos {
			if proto == peerProto {
				return proto, nil
			}
		}
	}

	return types.ProtocolID(""), nil
}

// RemovePeer 移除节点协议
func (pb *PersistentProtoBook) RemovePeer(peerID types.PeerID) {
	pb.mu.Lock()
	delete(pb.protocols, peerID)
	pb.mu.Unlock()

	// 从存储中删除
	pb.store.Delete([]byte(peerID))
}

// PeersWithProtocols 返回拥有协议的节点列表
func (pb *PersistentProtoBook) PeersWithProtocols() []types.PeerID {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	peers := make([]types.PeerID, 0, len(pb.protocols))
	for peerID := range pb.protocols {
		peers = append(peers, peerID)
	}

	return peers
}

// Ensure PersistentProtoBook implements the same interface as ProtoBook
var _ interface {
	GetProtocols(types.PeerID) ([]types.ProtocolID, error)
	AddProtocols(types.PeerID, ...types.ProtocolID) error
	SetProtocols(types.PeerID, ...types.ProtocolID) error
	RemoveProtocols(types.PeerID, ...types.ProtocolID) error
	SupportsProtocols(types.PeerID, ...types.ProtocolID) ([]types.ProtocolID, error)
	FirstSupportedProtocol(types.PeerID, ...types.ProtocolID) (types.ProtocolID, error)
	RemovePeer(types.PeerID)
} = (*PersistentProtoBook)(nil)
