// Package protobook 实现协议簿
package protobook

import (
	"sync"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ProtoBook 协议簿
type ProtoBook struct {
	mu sync.RWMutex

	// protocols 协议映射
	protocols map[types.PeerID][]types.ProtocolID
}

// New 创建协议簿
func New() *ProtoBook {
	return &ProtoBook{
		protocols: make(map[types.PeerID][]types.ProtocolID),
	}
}

// GetProtocols 获取协议
func (pb *ProtoBook) GetProtocols(peerID types.PeerID) ([]types.ProtocolID, error) {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	protocols := pb.protocols[peerID]
	if protocols == nil {
		return nil, nil
	}

	// 返回副本
	result := make([]types.ProtocolID, len(protocols))
	copy(result, protocols)

	return result, nil
}

// AddProtocols 添加协议
func (pb *ProtoBook) AddProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
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

	return nil
}

// SetProtocols 设置协议（覆盖）
func (pb *ProtoBook) SetProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	pb.protocols[peerID] = append([]types.ProtocolID{}, protocols...)

	return nil
}

// RemoveProtocols 移除协议
func (pb *ProtoBook) RemoveProtocols(peerID types.PeerID, protocols ...types.ProtocolID) error {
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

	return nil
}

// SupportsProtocols 查询支持的协议
func (pb *ProtoBook) SupportsProtocols(peerID types.PeerID, protocols ...types.ProtocolID) ([]types.ProtocolID, error) {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	peerProtos := pb.protocols[peerID]
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
func (pb *ProtoBook) FirstSupportedProtocol(peerID types.PeerID, protocols ...types.ProtocolID) (types.ProtocolID, error) {
	pb.mu.RLock()
	defer pb.mu.RUnlock()

	peerProtos := pb.protocols[peerID]
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
func (pb *ProtoBook) RemovePeer(peerID types.PeerID) {
	pb.mu.Lock()
	defer pb.mu.Unlock()

	delete(pb.protocols, peerID)
}
