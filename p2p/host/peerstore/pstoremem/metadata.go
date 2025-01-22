package pstoremem

import (
	"sync"

	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
)

// memoryPeerMetadata 内存对等节点元数据实现
type memoryPeerMetadata struct {
	// ds 存储对等节点的元数据,key为对等节点ID,value为该节点的元数据映射
	ds map[peer.ID]map[string]interface{}
	// dslock 用于并发控制的读写锁
	dslock sync.RWMutex
}

// 确保 memoryPeerMetadata 实现了 PeerMetadata 接口
var _ pstore.PeerMetadata = (*memoryPeerMetadata)(nil)

// NewPeerMetadata 创建新的内存对等节点元数据实例
// 返回:
//   - *memoryPeerMetadata: 内存对等节点元数据实例
func NewPeerMetadata() *memoryPeerMetadata {
	return &memoryPeerMetadata{
		ds: make(map[peer.ID]map[string]interface{}), // 初始化元数据存储映射
	}
}

// Put 存储对等节点的元数据
// 参数:
//   - p: 对等节点ID
//   - key: 元数据键
//   - val: 元数据值
//
// 返回:
//   - error: 错误信息
func (ps *memoryPeerMetadata) Put(p peer.ID, key string, val interface{}) error {
	ps.dslock.Lock()         // 加写锁
	defer ps.dslock.Unlock() // 延迟解锁

	m, ok := ps.ds[p] // 获取节点的元数据映射
	if !ok {          // 如果不存在
		m = make(map[string]interface{}) // 创建新的映射
		ps.ds[p] = m                     // 保存映射
	}
	m[key] = val // 存储元数据
	return nil
}

// Get 获取对等节点的元数据
// 参数:
//   - p: 对等节点ID
//   - key: 元数据键
//
// 返回:
//   - interface{}: 元数据值
//   - error: 错误信息
func (ps *memoryPeerMetadata) Get(p peer.ID, key string) (interface{}, error) {
	ps.dslock.RLock()         // 加读锁
	defer ps.dslock.RUnlock() // 延迟解锁

	m, ok := ps.ds[p] // 获取节点的元数据映射
	if !ok {          // 如果不存在
		log.Debugf("对等节点不存在: %v", p)
		return nil, pstore.ErrNotFound // 返回未找到错误
	}
	val, ok := m[key] // 获取元数据值
	if !ok {          // 如果不存在
		log.Debugf("元数据不存在: %v", key)
		return nil, pstore.ErrNotFound // 返回未找到错误
	}
	return val, nil // 返回元数据值
}

// RemovePeer 移除对等节点的所有元数据
// 参数:
//   - p: 对等节点ID
func (ps *memoryPeerMetadata) RemovePeer(p peer.ID) {
	ps.dslock.Lock()   // 加写锁
	delete(ps.ds, p)   // 删除节点的所有元数据
	ps.dslock.Unlock() // 解锁
}
