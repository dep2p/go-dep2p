// Package pubsub 实现发布订阅协议
package pubsub

import (
	"math/rand"
	"sync"
)

// meshPeers Mesh 节点管理
type meshPeers struct {
	mu    sync.RWMutex
	peers map[string]map[string]bool // topic -> peer -> true
	d     int                         // 目标度数
	dlo   int                         // 度数下限
	dhi   int                         // 度数上限
}

// newMeshPeers 创建 Mesh 节点管理
func newMeshPeers(d, dlo, dhi int) *meshPeers {
	return &meshPeers{
		peers: make(map[string]map[string]bool),
		d:     d,
		dlo:   dlo,
		dhi:   dhi,
	}
}

// Add 添加节点到 Mesh
func (mp *meshPeers) Add(topic, peerID string) bool {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.peers[topic] == nil {
		mp.peers[topic] = make(map[string]bool)
	}

	// 检查是否已满
	if len(mp.peers[topic]) >= mp.dhi {
		return false
	}

	mp.peers[topic][peerID] = true
	return true
}

// Remove 从 Mesh 移除节点
func (mp *meshPeers) Remove(topic, peerID string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	if mp.peers[topic] != nil {
		delete(mp.peers[topic], peerID)
	}
}

// Has 检查节点是否在 Mesh 中
func (mp *meshPeers) Has(topic, peerID string) bool {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if mp.peers[topic] == nil {
		return false
	}
	return mp.peers[topic][peerID]
}

// List 列出主题的所有 Mesh 节点
func (mp *meshPeers) List(topic string) []string {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if mp.peers[topic] == nil {
		return nil
	}

	peers := make([]string, 0, len(mp.peers[topic]))
	for peerID := range mp.peers[topic] {
		peers = append(peers, peerID)
	}
	return peers
}

// Count 获取主题的 Mesh 节点数
func (mp *meshPeers) Count(topic string) int {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if mp.peers[topic] == nil {
		return 0
	}
	return len(mp.peers[topic])
}

// NeedMorePeers 是否需要更多节点
func (mp *meshPeers) NeedMorePeers(topic string) bool {
	return mp.Count(topic) < mp.d
}

// TooManyPeers 是否节点过多
func (mp *meshPeers) TooManyPeers(topic string) bool {
	return mp.Count(topic) > mp.dhi
}

// TooFewPeers 是否节点过少
func (mp *meshPeers) TooFewPeers(topic string) bool {
	return mp.Count(topic) < mp.dlo
}

// SelectPeersToGraft 选择需要 GRAFT 的节点
func (mp *meshPeers) SelectPeersToGraft(topic string, candidates []string, count int) []string {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	// 过滤已在 Mesh 中的节点
	available := make([]string, 0)
	for _, peerID := range candidates {
		if mp.peers[topic] == nil || !mp.peers[topic][peerID] {
			available = append(available, peerID)
		}
	}

	// 随机选择
	if len(available) <= count {
		return available
	}

	// 随机打乱
	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})

	return available[:count]
}

// SelectPeersToPrune 选择需要 PRUNE 的节点
func (mp *meshPeers) SelectPeersToPrune(topic string, count int) []string {
	mp.mu.RLock()
	defer mp.mu.RUnlock()

	if mp.peers[topic] == nil {
		return nil
	}

	peers := make([]string, 0, len(mp.peers[topic]))
	for peerID := range mp.peers[topic] {
		peers = append(peers, peerID)
	}

	// 随机选择需要 PRUNE 的节点
	if len(peers) <= count {
		return peers
	}

	rand.Shuffle(len(peers), func(i, j int) {
		peers[i], peers[j] = peers[j], peers[i]
	})

	return peers[:count]
}

// Clear 清空主题的 Mesh
func (mp *meshPeers) Clear(topic string) {
	mp.mu.Lock()
	defer mp.mu.Unlock()

	delete(mp.peers, topic)
}
