// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	crand "crypto/rand"
	"encoding/binary"
	"math/rand" //nolint:gosec // G404: 使用 crypto/rand 初始化种子，用于非安全的 mesh 随机选择
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mesh 管理器
// ============================================================================

// MeshManager Mesh 网络管理器
//
// 负责维护每个主题的 mesh 网络：
// - mesh: 已订阅主题的全连接网络
// - fanout: 未订阅但需要发布的主题的目标 peers
type MeshManager struct {
	mu sync.RWMutex

	// config 配置
	config *Config

	// scorer 评分器
	scorer *PeerScorer

	// mesh 每个主题的 mesh peers
	mesh map[string]map[types.NodeID]struct{}

	// fanout 每个主题的 fanout peers
	fanout map[string]map[types.NodeID]struct{}

	// fanoutLastPub fanout 最后发布时间
	fanoutLastPub map[string]time.Time

	// topics 主题状态
	topics map[string]*TopicState

	// peers 所有已知 peers
	peers map[types.NodeID]*PeerState

	// directPeers 直连 peers（始终保持在 mesh 中）
	directPeers map[types.NodeID]struct{}

	// backoffs 退避追踪
	backoffs *BackoffTracker

	// rand 随机数生成器
	rand *rand.Rand
}

// NewMeshManager 创建新的 Mesh 管理器
func NewMeshManager(config *Config, scorer *PeerScorer) *MeshManager {
	if config == nil {
		config = DefaultConfig()
	}

	return &MeshManager{
		config:        config,
		scorer:        scorer,
		mesh:          make(map[string]map[types.NodeID]struct{}),
		fanout:        make(map[string]map[types.NodeID]struct{}),
		fanoutLastPub: make(map[string]time.Time),
		topics:        make(map[string]*TopicState),
		peers:         make(map[types.NodeID]*PeerState),
		directPeers:   make(map[types.NodeID]struct{}),
		backoffs:      NewBackoffTracker(),
		rand:          rand.New(rand.NewSource(cryptoSeed())),
	}
}

// cryptoSeed 生成加密安全的随机种子
func cryptoSeed() int64 {
	var seed int64
	b := make([]byte, 8)
	if _, err := crand.Read(b); err != nil {
		// 回退到时间戳（不应该发生）
		return time.Now().UnixNano()
	}
	seed = int64(binary.BigEndian.Uint64(b))
	return seed
}

// ============================================================================
//                              主题管理
// ============================================================================

// Join 加入主题（订阅）
func (mm *MeshManager) Join(topic string) []types.NodeID {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 获取或创建主题状态
	ts := mm.getOrCreateTopic(topic)
	if ts.Subscribed {
		return nil // 已订阅
	}
	ts.Subscribed = true

	// 初始化 mesh
	if mm.mesh[topic] == nil {
		mm.mesh[topic] = make(map[types.NodeID]struct{})
	}

	// 从 fanout 迁移到 mesh
	if fanoutPeers, exists := mm.fanout[topic]; exists {
		for peer := range fanoutPeers {
			mm.mesh[topic][peer] = struct{}{}
			ts.Mesh[peer] = struct{}{}
		}
		delete(mm.fanout, topic)
		delete(mm.fanoutLastPub, topic)
	}

	// 选择更多 peers 填充 mesh
	toGraft := mm.selectPeersToGraft(topic, mm.config.D-len(mm.mesh[topic]))

	// 将选中的 peers 加入 mesh
	for _, peer := range toGraft {
		mm.mesh[topic][peer] = struct{}{}
		ts.Mesh[peer] = struct{}{}
		if mm.scorer != nil {
			mm.scorer.Graft(peer, topic)
		}
	}

	return toGraft
}

// Leave 离开主题（取消订阅）
func (mm *MeshManager) Leave(topic string) []types.NodeID {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ts, exists := mm.topics[topic]
	if !exists || !ts.Subscribed {
		return nil
	}
	ts.Subscribed = false

	// 收集需要 PRUNE 的 peers
	toPrune := make([]types.NodeID, 0, len(mm.mesh[topic]))
	for peer := range mm.mesh[topic] {
		toPrune = append(toPrune, peer)
		if mm.scorer != nil {
			mm.scorer.Prune(peer, topic)
		}
	}

	// 清空 mesh
	delete(mm.mesh, topic)
	ts.Mesh = make(map[types.NodeID]struct{})

	return toPrune
}

// ============================================================================
//                              Peer 管理
// ============================================================================

// AddPeer 添加 peer
func (mm *MeshManager) AddPeer(peer types.NodeID, outbound bool) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if _, exists := mm.peers[peer]; exists {
		mm.peers[peer].Connected = true
		mm.peers[peer].LastSeen = time.Now()
		return
	}

	now := time.Now()
	mm.peers[peer] = &PeerState{
		ID:         peer,
		Topics:     make(map[string]struct{}),
		Connected:  true,
		Outbound:   outbound,
		FirstSeen:  now,
		LastSeen:   now,
		Behaviours: NewPeerBehaviours(),
	}
}

// RemovePeer 移除 peer
func (mm *MeshManager) RemovePeer(peer types.NodeID) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	ps, exists := mm.peers[peer]
	if !exists {
		return
	}
	ps.Connected = false

	// 从所有 mesh 中移除
	for topic := range ps.Topics {
		if meshPeers, exists := mm.mesh[topic]; exists {
			delete(meshPeers, peer)
		}
		if ts, exists := mm.topics[topic]; exists {
			delete(ts.Mesh, peer)
			delete(ts.Peers, peer)
		}
	}

	// 从 fanout 中移除
	for topic := range mm.fanout {
		delete(mm.fanout[topic], peer)
	}
}

// AddPeerToTopic 将 peer 添加到主题
func (mm *MeshManager) AddPeerToTopic(peer types.NodeID, topic string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 确保 peer 存在
	ps, exists := mm.peers[peer]
	if !exists {
		mm.AddPeer(peer, false)
		ps = mm.peers[peer]
	}
	ps.Topics[topic] = struct{}{}

	// 更新主题状态
	ts := mm.getOrCreateTopic(topic)
	ts.Peers[peer] = struct{}{}
}

// RemovePeerFromTopic 将 peer 从主题移除
func (mm *MeshManager) RemovePeerFromTopic(peer types.NodeID, topic string) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if ps, exists := mm.peers[peer]; exists {
		delete(ps.Topics, topic)
	}

	// 从 mesh 移除
	if meshPeers, exists := mm.mesh[topic]; exists {
		delete(meshPeers, peer)
	}

	// 更新主题状态
	if ts, exists := mm.topics[topic]; exists {
		delete(ts.Mesh, peer)
		delete(ts.Peers, peer)
	}
}

// ============================================================================
//                              Mesh 操作
// ============================================================================

// Graft 将 peer 加入 mesh
func (mm *MeshManager) Graft(peer types.NodeID, topic string) bool {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 检查是否订阅该主题
	ts, exists := mm.topics[topic]
	if !exists || !ts.Subscribed {
		return false
	}

	// 检查 peer 是否在该主题
	ps, exists := mm.peers[peer]
	if !exists || !ps.Connected {
		return false
	}
	if _, inTopic := ps.Topics[topic]; !inTopic {
		return false
	}

	// 检查退避
	if mm.backoffs.IsBackedOff(peer.String(), topic) {
		return false
	}

	// 检查评分
	if mm.scorer != nil && mm.scorer.IsBelowGraylistThreshold(peer) {
		return false
	}

	// 添加到 mesh
	if mm.mesh[topic] == nil {
		mm.mesh[topic] = make(map[types.NodeID]struct{})
	}
	mm.mesh[topic][peer] = struct{}{}
	ts.Mesh[peer] = struct{}{}

	if mm.scorer != nil {
		mm.scorer.Graft(peer, topic)
	}

	return true
}

// Prune 将 peer 从 mesh 移除
func (mm *MeshManager) Prune(peer types.NodeID, topic string, backoff time.Duration) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	if meshPeers, exists := mm.mesh[topic]; exists {
		delete(meshPeers, peer)
	}
	if ts, exists := mm.topics[topic]; exists {
		delete(ts.Mesh, peer)
	}

	// 添加退避
	if backoff > 0 {
		mm.backoffs.AddBackoff(peer.String(), topic, backoff)
	}

	if mm.scorer != nil {
		mm.scorer.Prune(peer, topic)
	}
}

// MeshPeers 获取主题的 mesh peers
func (mm *MeshManager) MeshPeers(topic string) []types.NodeID {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	meshPeers, exists := mm.mesh[topic]
	if !exists {
		return nil
	}

	peers := make([]types.NodeID, 0, len(meshPeers))
	for peer := range meshPeers {
		peers = append(peers, peer)
	}
	return peers
}

// MeshPeerCount 获取主题的 mesh peer 数量
func (mm *MeshManager) MeshPeerCount(topic string) int {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return len(mm.mesh[topic])
}

// ============================================================================
//                              Fanout 操作
// ============================================================================

// FanoutPeers 获取主题的 fanout peers
func (mm *MeshManager) FanoutPeers(topic string) []types.NodeID {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 检查现有 fanout
	if fanoutPeers, exists := mm.fanout[topic]; exists {
		// 更新最后发布时间
		mm.fanoutLastPub[topic] = time.Now()

		peers := make([]types.NodeID, 0, len(fanoutPeers))
		for peer := range fanoutPeers {
			peers = append(peers, peer)
		}
		return peers
	}

	// 创建新的 fanout
	ts := mm.getOrCreateTopic(topic)
	candidates := make([]types.NodeID, 0, len(ts.Peers))
	for peer := range ts.Peers {
		if ps, exists := mm.peers[peer]; exists && ps.Connected {
			candidates = append(candidates, peer)
		}
	}

	// 选择 D 个 peers
	selected := mm.selectRandomPeers(candidates, mm.config.D)

	mm.fanout[topic] = make(map[types.NodeID]struct{})
	for _, peer := range selected {
		mm.fanout[topic][peer] = struct{}{}
	}
	mm.fanoutLastPub[topic] = time.Now()

	return selected
}

// CleanupFanout 清理过期 fanout
func (mm *MeshManager) CleanupFanout() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	now := time.Now()
	for topic, lastPub := range mm.fanoutLastPub {
		if now.Sub(lastPub) > mm.config.FanoutTTL {
			delete(mm.fanout, topic)
			delete(mm.fanoutLastPub, topic)
		}
	}
}

// ============================================================================
//                              心跳维护
// ============================================================================

// HeartbeatMaintenance 心跳维护
//
// 返回需要 GRAFT 和 PRUNE 的 peers
func (mm *MeshManager) HeartbeatMaintenance() (grafts map[string][]types.NodeID, prunes map[string][]types.NodeID) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	grafts = make(map[string][]types.NodeID)
	prunes = make(map[string][]types.NodeID)

	for topic, ts := range mm.topics {
		if !ts.Subscribed {
			continue
		}

		meshPeers := mm.mesh[topic]
		meshSize := len(meshPeers)

		// 检查是否需要扩展
		if meshSize < mm.config.Dlo {
			needed := mm.config.D - meshSize
			toGraft := mm.selectPeersToGraft(topic, needed)
			for _, peer := range toGraft {
				mm.mesh[topic][peer] = struct{}{}
				ts.Mesh[peer] = struct{}{}
				if mm.scorer != nil {
					mm.scorer.Graft(peer, topic)
				}
			}
			if len(toGraft) > 0 {
				grafts[topic] = toGraft
			}
		}

		// 检查是否需要收缩
		if meshSize > mm.config.Dhi {
			excess := meshSize - mm.config.D
			toPrune := mm.selectPeersToPrune(topic, excess)
			for _, peer := range toPrune {
				delete(mm.mesh[topic], peer)
				delete(ts.Mesh, peer)
				mm.backoffs.AddBackoff(peer.String(), topic, mm.config.PruneBackoff)
				if mm.scorer != nil {
					mm.scorer.Prune(peer, topic)
				}
			}
			if len(toPrune) > 0 {
				prunes[topic] = toPrune
			}
		}

		// 确保出站连接数
		mm.ensureOutboundConnections(topic)
	}

	// 清理退避
	mm.backoffs.Cleanup()

	return grafts, prunes
}

// selectPeersToGraft 选择要 GRAFT 的 peers
func (mm *MeshManager) selectPeersToGraft(topic string, count int) []types.NodeID {
	if count <= 0 {
		return nil
	}

	ts := mm.getOrCreateTopic(topic)
	meshPeers := mm.mesh[topic]
	if meshPeers == nil {
		meshPeers = make(map[types.NodeID]struct{})
	}

	// 候选 peers：在主题中但不在 mesh 中的已连接 peers
	candidates := make([]types.NodeID, 0)
	for peer := range ts.Peers {
		// 跳过已在 mesh 中的
		if _, inMesh := meshPeers[peer]; inMesh {
			continue
		}
		// 检查连接状态
		ps, exists := mm.peers[peer]
		if !exists || !ps.Connected {
			continue
		}
		// 检查退避
		if mm.backoffs.IsBackedOff(peer.String(), topic) {
			continue
		}
		// 检查评分
		if mm.scorer != nil && mm.scorer.IsBelowGraylistThreshold(peer) {
			continue
		}
		candidates = append(candidates, peer)
	}

	// 优先选择高分 peers
	if mm.scorer != nil && len(candidates) > count {
		sort.Slice(candidates, func(i, j int) bool {
			return mm.scorer.Score(candidates[i]) > mm.scorer.Score(candidates[j])
		})
		// 从高分中随机选择
		topN := minInt(count*2, len(candidates))
		candidates = candidates[:topN]
	}

	return mm.selectRandomPeers(candidates, count)
}

// selectPeersToPrune 选择要 PRUNE 的 peers
func (mm *MeshManager) selectPeersToPrune(topic string, count int) []types.NodeID {
	if count <= 0 {
		return nil
	}

	meshPeers := mm.mesh[topic]
	if len(meshPeers) <= count {
		return nil
	}

	// 收集非直连、非出站的 peers
	candidates := make([]types.NodeID, 0)
	for peer := range meshPeers {
		// 跳过直连 peers
		if _, isDirect := mm.directPeers[peer]; isDirect {
			continue
		}
		// 优先保留出站连接
		ps, exists := mm.peers[peer]
		if exists && ps.Outbound && len(candidates) >= count {
			continue
		}
		candidates = append(candidates, peer)
	}

	// 优先移除低分 peers
	if mm.scorer != nil {
		sort.Slice(candidates, func(i, j int) bool {
			return mm.scorer.Score(candidates[i]) < mm.scorer.Score(candidates[j])
		})
	}

	if len(candidates) > count {
		candidates = candidates[:count]
	}
	return candidates
}

// ensureOutboundConnections 确保足够的出站连接
func (mm *MeshManager) ensureOutboundConnections(topic string) {
	meshPeers := mm.mesh[topic]
	if meshPeers == nil {
		return
	}

	// 统计出站连接
	outbound := 0
	for peer := range meshPeers {
		if ps, exists := mm.peers[peer]; exists && ps.Outbound {
			outbound++
		}
	}

	// 如果出站连接不足，尝试添加
	if outbound < mm.config.Dout && len(meshPeers) < mm.config.Dhi {
		needed := mm.config.Dout - outbound
		ts := mm.getOrCreateTopic(topic)

		candidates := make([]types.NodeID, 0)
		for peer := range ts.Peers {
			ps, exists := mm.peers[peer]
			if !exists || !ps.Connected || !ps.Outbound {
				continue
			}
			if _, inMesh := meshPeers[peer]; inMesh {
				continue
			}
			if mm.backoffs.IsBackedOff(peer.String(), topic) {
				continue
			}
			candidates = append(candidates, peer)
		}

		selected := mm.selectRandomPeers(candidates, needed)
		for _, peer := range selected {
			mm.mesh[topic][peer] = struct{}{}
			ts.Mesh[peer] = struct{}{}
			if mm.scorer != nil {
				mm.scorer.Graft(peer, topic)
			}
		}
	}
}

// ============================================================================
//                              Gossip 选择
// ============================================================================

// SelectGossipPeers 选择 gossip 目标 peers
func (mm *MeshManager) SelectGossipPeers(topic string) []types.NodeID {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	ts, exists := mm.topics[topic]
	if !exists {
		return nil
	}

	meshPeers := mm.mesh[topic]
	if meshPeers == nil {
		meshPeers = make(map[types.NodeID]struct{})
	}

	// 候选 peers：在主题中但不在 mesh 中的
	candidates := make([]types.NodeID, 0)
	for peer := range ts.Peers {
		// 跳过 mesh 中的
		if _, inMesh := meshPeers[peer]; inMesh {
			continue
		}
		// 检查连接和评分
		ps, exists := mm.peers[peer]
		if !exists || !ps.Connected {
			continue
		}
		if mm.scorer != nil && mm.scorer.IsBelowGossipThreshold(peer) {
			continue
		}
		candidates = append(candidates, peer)
	}

	return mm.selectRandomPeers(candidates, mm.config.Dlazy)
}

// ============================================================================
//                              PX (Peer Exchange)
// ============================================================================

// GetPXPeers 获取 PX peers（用于 PRUNE 消息）
func (mm *MeshManager) GetPXPeers(topic string, exclude types.NodeID, count int) []PeerInfo {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	ts, exists := mm.topics[topic]
	if !exists {
		return nil
	}

	candidates := make([]types.NodeID, 0)
	for peer := range ts.Peers {
		if peer == exclude {
			continue
		}
		ps, exists := mm.peers[peer]
		if !exists || !ps.Connected {
			continue
		}
		// 只推荐高分 peers
		if mm.scorer != nil && !mm.scorer.IsAboveAcceptPXThreshold(peer) {
			continue
		}
		candidates = append(candidates, peer)
	}

	selected := mm.selectRandomPeers(candidates, count)
	pxPeers := make([]PeerInfo, len(selected))
	for i, peer := range selected {
		pxPeers[i] = PeerInfo{ID: peer}
	}
	return pxPeers
}

// HandlePX 处理收到的 PX peers
func (mm *MeshManager) HandlePX(from types.NodeID, _ string, peers []PeerInfo) []types.NodeID {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	// 检查发送者评分
	if mm.scorer != nil && !mm.scorer.IsAboveAcceptPXThreshold(from) {
		return nil
	}

	// 收集需要连接的 peers
	toConnect := make([]types.NodeID, 0)
	for _, px := range peers {
		// 跳过已知 peers
		if _, exists := mm.peers[px.ID]; exists {
			continue
		}
		toConnect = append(toConnect, px.ID)
	}

	return toConnect
}

// ============================================================================
//                              查询方法
// ============================================================================

// IsSubscribed 检查是否订阅主题
func (mm *MeshManager) IsSubscribed(topic string) bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	ts, exists := mm.topics[topic]
	return exists && ts.Subscribed
}

// Topics 返回所有已订阅的主题
func (mm *MeshManager) Topics() []string {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	topics := make([]string, 0)
	for topic, ts := range mm.topics {
		if ts.Subscribed {
			topics = append(topics, topic)
		}
	}
	return topics
}

// PeersInTopic 返回主题中的所有 peers
func (mm *MeshManager) PeersInTopic(topic string) []types.NodeID {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	ts, exists := mm.topics[topic]
	if !exists {
		return nil
	}

	peers := make([]types.NodeID, 0, len(ts.Peers))
	for peer := range ts.Peers {
		peers = append(peers, peer)
	}
	return peers
}

// IsPeerInMesh 检查 peer 是否在主题的 mesh 中
func (mm *MeshManager) IsPeerInMesh(peer types.NodeID, topic string) bool {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	meshPeers, exists := mm.mesh[topic]
	if !exists {
		return false
	}
	_, inMesh := meshPeers[peer]
	return inMesh
}

// ============================================================================
//                              辅助方法
// ============================================================================

// getOrCreateTopic 获取或创建主题状态
func (mm *MeshManager) getOrCreateTopic(topic string) *TopicState {
	ts, exists := mm.topics[topic]
	if !exists {
		ts = NewTopicState(topic)
		mm.topics[topic] = ts
	}
	return ts
}

// selectRandomPeers 随机选择 peers
func (mm *MeshManager) selectRandomPeers(peers []types.NodeID, count int) []types.NodeID {
	if len(peers) <= count {
		return peers
	}

	// Fisher-Yates 洗牌
	shuffled := make([]types.NodeID, len(peers))
	copy(shuffled, peers)
	for i := len(shuffled) - 1; i > 0; i-- {
		j := mm.rand.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled[:count]
}

// AddDirectPeer 添加直连 peer
func (mm *MeshManager) AddDirectPeer(peer types.NodeID) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	mm.directPeers[peer] = struct{}{}
}

// RemoveDirectPeer 移除直连 peer
func (mm *MeshManager) RemoveDirectPeer(peer types.NodeID) {
	mm.mu.Lock()
	defer mm.mu.Unlock()
	delete(mm.directPeers, peer)
}

// GetStats 获取统计信息
func (mm *MeshManager) GetStats() *Stats {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	stats := &Stats{
		TopicStats: make(map[string]*TopicStats),
		TotalPeers: len(mm.peers),
		MeshPeers:  make(map[string]int),
	}

	for topic, ts := range mm.topics {
		stats.TopicStats[topic] = &TopicStats{
			Topic:         topic,
			MeshPeerCount: len(ts.Mesh),
			PeerCount:     len(ts.Peers),
		}
		stats.MeshPeers[topic] = len(ts.Mesh)
	}

	return stats
}

// Reset 重置管理器
func (mm *MeshManager) Reset() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.mesh = make(map[string]map[types.NodeID]struct{})
	mm.fanout = make(map[string]map[types.NodeID]struct{})
	mm.fanoutLastPub = make(map[string]time.Time)
	mm.topics = make(map[string]*TopicState)
	mm.peers = make(map[types.NodeID]*PeerState)
	mm.backoffs.Clear()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

