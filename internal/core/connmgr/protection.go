// Package connmgr 提供连接管理模块的实现
package connmgr

import (
	"time"

	connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              保护机制
// ============================================================================

// Protect 保护连接
func (m *ConnectionManager) Protect(nodeID types.NodeID, tag string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	peer, ok := m.peers[nodeID]
	if !ok {
		// 如果节点不存在，创建一个完整的 peerInfo
		now := time.Now()
		peer = &peerInfo{
			nodeID: nodeID,
			tags:   make(map[string]struct{}),
			connInfo: connmgrif.ConnectionInfo{
				NodeID:    nodeID,
				Direction: types.DirUnknown, // 未知方向
				CreatedAt: now,
			},
			createdAt:  now,
			lastActive: now,
		}
		m.peers[nodeID] = peer
	}
	peer.tags[tag] = struct{}{}
	peer.protected = true

	log.Debug("保护连接",
		"peer", nodeID.ShortString(),
		"tag", tag)
}

// Unprotect 取消保护
func (m *ConnectionManager) Unprotect(nodeID types.NodeID, tag string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	peer, ok := m.peers[nodeID]
	if !ok {
		return
	}
	delete(peer.tags, tag)
	if len(peer.tags) == 0 {
		peer.protected = false
	}

	log.Debug("取消保护",
		"peer", nodeID.ShortString(),
		"tag", tag)
}

// IsProtected 检查是否受保护
func (m *ConnectionManager) IsProtected(nodeID types.NodeID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peer, ok := m.peers[nodeID]
	if !ok {
		return false
	}
	return peer.protected
}

// TagsForPeer 返回节点的标签
func (m *ConnectionManager) TagsForPeer(nodeID types.NodeID) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peer, ok := m.peers[nodeID]
	if !ok {
		return nil
	}

	tags := make([]string, 0, len(peer.tags))
	for tag := range peer.tags {
		tags = append(tags, tag)
	}
	return tags
}

