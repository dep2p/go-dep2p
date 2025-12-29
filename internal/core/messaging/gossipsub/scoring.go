// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	"math"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              节点评分器
// ============================================================================

// PeerScorer 节点评分器
//
// 实现 GossipSub v1.1 的评分机制，评估节点行为并影响：
// - Mesh 选择（GRAFT/PRUNE）
// - Gossip 目标选择
// - 消息转发决策
type PeerScorer struct {
	mu sync.RWMutex

	// params 评分参数
	params *ScoreParams

	// 阈值（从 Config 获取）
	gossipThreshold   float64
	publishThreshold  float64
	graylistThreshold float64
	acceptPXThreshold float64

	// topicParams 主题评分参数
	topicParams map[string]*TopicScoreParams

	// peerStats peer 统计
	peerStats map[types.NodeID]*peerScoreStats

	// peerIPs peer IP 映射（用于 IP 协同检测）
	peerIPs map[types.NodeID]string

	// ipPeers IP 到 peer 列表的映射
	ipPeers map[string]map[types.NodeID]struct{}

	// inspectQueue 检查队列
	inspectQueue []types.NodeID

	// lastDecay 最后衰减时间
	lastDecay time.Time
}

// peerScoreStats peer 评分统计
type peerScoreStats struct {
	// connected 是否已连接
	connected bool

	// firstSeen 首次发现时间
	firstSeen time.Time

	// lastSeen 最后活跃时间
	lastSeen time.Time

	// topicStats 主题统计
	topicStats map[string]*topicScoreStats

	// behaviourPenalty 行为惩罚
	behaviourPenalty float64

	// appScore 应用层评分
	appScore float64

	// ips 关联的 IP 地址
	ips []string
}

// topicScoreStats 主题评分统计
type topicScoreStats struct {
	// inMesh 是否在 mesh 中
	inMesh bool

	// meshTime mesh 时间
	meshTime time.Duration

	// graftTime 最后 GRAFT 时间
	graftTime time.Time

	// firstMessageDeliveries 首次消息投递计数
	firstMessageDeliveries float64

	// meshMessageDeliveries mesh 消息投递计数
	meshMessageDeliveries float64

	// meshMessageDeliveriesActive mesh 投递是否激活
	meshMessageDeliveriesActive bool

	// meshFailurePenalty mesh 失败惩罚
	meshFailurePenalty float64

	// invalidMessages 无效消息计数
	invalidMessages float64
}

// NewPeerScorer 创建新的节点评分器
func NewPeerScorer(params *ScoreParams) *PeerScorer {
	if params == nil {
		params = DefaultScoreParams()
	}

	return &PeerScorer{
		params:            params,
		gossipThreshold:   -500,
		publishThreshold:  -1000,
		graylistThreshold: -2500,
		acceptPXThreshold: 10,
		topicParams:       make(map[string]*TopicScoreParams),
		peerStats:         make(map[types.NodeID]*peerScoreStats),
		peerIPs:           make(map[types.NodeID]string),
		ipPeers:           make(map[string]map[types.NodeID]struct{}),
		lastDecay:         time.Now(),
	}
}

// NewPeerScorerWithThresholds 创建带阈值的节点评分器
func NewPeerScorerWithThresholds(params *ScoreParams, gossip, publish, graylist, acceptPX float64) *PeerScorer {
	ps := NewPeerScorer(params)
	ps.gossipThreshold = gossip
	ps.publishThreshold = publish
	ps.graylistThreshold = graylist
	ps.acceptPXThreshold = acceptPX
	return ps
}

// ============================================================================
//                              评分计算
// ============================================================================

// Score 计算 peer 评分
func (ps *PeerScorer) Score(peer types.NodeID) float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats, exists := ps.peerStats[peer]
	if !exists {
		return 0
	}

	return ps.computeScore(peer, stats)
}

// computeScore 内部评分计算
func (ps *PeerScorer) computeScore(peer types.NodeID, stats *peerScoreStats) float64 {
	var score float64

	// 主题评分
	for topic, tStats := range stats.topicStats {
		tParams := ps.getTopicParams(topic)
		topicScore := ps.computeTopicScore(tStats, tParams)
		score += topicScore * tParams.TopicWeight
	}

	// 应用层评分 (P5)
	if ps.params.AppSpecificScore != nil {
		appScore := ps.params.AppSpecificScore(peer.String())
		score += appScore * ps.params.AppSpecificWeight
	} else {
		score += stats.appScore * ps.params.AppSpecificWeight
	}

	// IP 协同惩罚 (P6)
	ipScore := ps.computeIPColocationScore(peer)
	score += ipScore * ps.params.IPColocationFactorWeight

	// 行为惩罚 (P7)
	if stats.behaviourPenalty > ps.params.BehaviourPenaltyThreshold {
		score += stats.behaviourPenalty * ps.params.BehaviourPenaltyWeight
	}

	return score
}

// computeTopicScore 计算主题评分
func (ps *PeerScorer) computeTopicScore(stats *topicScoreStats, params *TopicScoreParams) float64 {
	var score float64

	// P1: Mesh 时间
	if stats.inMesh {
		meshTime := stats.meshTime
		if meshTime > 0 {
			p1 := meshTime.Seconds() / params.TimeInMeshQuantum.Seconds()
			if p1 > params.TimeInMeshCap {
				p1 = params.TimeInMeshCap
			}
			score += p1 * params.TimeInMeshWeight
		}
	}

	// P2: 首次消息投递
	p2 := stats.firstMessageDeliveries
	if p2 > params.FirstMessageDeliveriesCap {
		p2 = params.FirstMessageDeliveriesCap
	}
	score += p2 * params.FirstMessageDeliveriesWeight

	// P3: Mesh 消息投递
	if stats.meshMessageDeliveriesActive {
		deficit := params.MeshMessageDeliveriesThreshold - stats.meshMessageDeliveries
		if deficit > 0 {
			p3 := deficit * deficit
			score += p3 * params.MeshMessageDeliveriesWeight
		}
	}

	// P3b: Mesh 失败惩罚
	if stats.meshFailurePenalty > 0 {
		score += stats.meshFailurePenalty * params.MeshFailurePenaltyWeight
	}

	// P4: 无效消息
	if stats.invalidMessages > 0 {
		p4 := stats.invalidMessages * stats.invalidMessages
		score += p4 * params.InvalidMessageDeliveriesWeight
	}

	return score
}

// computeIPColocationScore 计算 IP 协同评分
func (ps *PeerScorer) computeIPColocationScore(peer types.NodeID) float64 {
	ip, exists := ps.peerIPs[peer]
	if !exists {
		return 0
	}

	// 检查白名单
	if _, whitelisted := ps.params.IPColocationFactorWhitelist[ip]; whitelisted {
		return 0
	}

	// 计算同 IP 的 peer 数量
	peers, exists := ps.ipPeers[ip]
	if !exists {
		return 0
	}

	count := len(peers)
	if count <= ps.params.IPColocationFactorThreshold {
		return 0
	}

	// 超过阈值，计算惩罚
	excess := count - ps.params.IPColocationFactorThreshold
	return float64(excess * excess)
}

// getTopicParams 获取主题参数
func (ps *PeerScorer) getTopicParams(topic string) *TopicScoreParams {
	if params, exists := ps.topicParams[topic]; exists {
		return params
	}
	return DefaultTopicScoreParams()
}

// ============================================================================
//                              事件处理
// ============================================================================

// AddPeer 添加 peer
func (ps *PeerScorer) AddPeer(peer types.NodeID, ip string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, exists := ps.peerStats[peer]; exists {
		return
	}

	now := time.Now()
	ps.peerStats[peer] = &peerScoreStats{
		connected:  true,
		firstSeen:  now,
		lastSeen:   now,
		topicStats: make(map[string]*topicScoreStats),
	}

	// 记录 IP
	if ip != "" {
		ps.peerIPs[peer] = ip
		if ps.ipPeers[ip] == nil {
			ps.ipPeers[ip] = make(map[types.NodeID]struct{})
		}
		ps.ipPeers[ip][peer] = struct{}{}
	}
}

// RemovePeer 移除 peer
func (ps *PeerScorer) RemovePeer(peer types.NodeID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats, exists := ps.peerStats[peer]
	if !exists {
		return
	}

	stats.connected = false

	// 从 IP 映射中移除
	if ip, exists := ps.peerIPs[peer]; exists {
		if peers, exists := ps.ipPeers[ip]; exists {
			delete(peers, peer)
			if len(peers) == 0 {
				delete(ps.ipPeers, ip)
			}
		}
		delete(ps.peerIPs, peer)
	}

	// 不立即删除统计，保留一段时间用于评分
}

// Graft peer 加入 mesh
func (ps *PeerScorer) Graft(peer types.NodeID, topic string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peer)
	tStats := ps.getOrCreateTopicStats(stats, topic)

	tStats.inMesh = true
	tStats.graftTime = time.Now()
	tStats.meshMessageDeliveriesActive = false
}

// Prune peer 离开 mesh
func (ps *PeerScorer) Prune(peer types.NodeID, topic string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peer)
	tStats := ps.getOrCreateTopicStats(stats, topic)

	if tStats.inMesh {
		tStats.inMesh = false
		// 计算 mesh 失败惩罚
		if tStats.meshMessageDeliveriesActive {
			params := ps.getTopicParams(topic)
			deficit := params.MeshMessageDeliveriesThreshold - tStats.meshMessageDeliveries
			if deficit > 0 {
				tStats.meshFailurePenalty += deficit * deficit
			}
		}
	}
}

// ValidateMessage 验证消息
func (ps *PeerScorer) ValidateMessage(peer types.NodeID, topic string, isFirst, isValid bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peer)
	tStats := ps.getOrCreateTopicStats(stats, topic)

	if isFirst && isValid {
		// 首次有效消息投递
		tStats.firstMessageDeliveries++
		if tStats.inMesh {
			tStats.meshMessageDeliveries++
		}
	} else if !isValid {
		// 无效消息
		tStats.invalidMessages++
	}
}

// DuplicateMessage 记录重复消息
func (ps *PeerScorer) DuplicateMessage(peer types.NodeID, topic string, wasFirst bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peer)
	tStats := ps.getOrCreateTopicStats(stats, topic)

	// 重复消息但在 mesh 投递窗口内也计入
	if tStats.inMesh && wasFirst {
		tStats.meshMessageDeliveries++
	}
}

// BrokenPromise 记录未履行的 IWANT
func (ps *PeerScorer) BrokenPromise(peer types.NodeID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peer)
	stats.behaviourPenalty++
}

// SetAppScore 设置应用层评分
func (ps *PeerScorer) SetAppScore(peer types.NodeID, score float64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peer)
	stats.appScore = score
}

// ============================================================================
//                              衰减
// ============================================================================

// Decay 执行评分衰减
func (ps *PeerScorer) Decay() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(ps.lastDecay)
	if elapsed < ps.params.DecayInterval {
		return
	}
	ps.lastDecay = now

	// 计算衰减因子
	decayIntervals := elapsed / ps.params.DecayInterval

	for peer, stats := range ps.peerStats {
		ps.decayPeerStats(stats, int(decayIntervals))

		// 清理断开连接且评分归零的 peer
		if !stats.connected && now.Sub(stats.lastSeen) > ps.params.RetainScore {
			delete(ps.peerStats, peer)
		}
	}
}

// decayPeerStats 衰减 peer 统计
func (ps *PeerScorer) decayPeerStats(stats *peerScoreStats, intervals int) {
	for topic, tStats := range stats.topicStats {
		params := ps.getTopicParams(topic)

		// 更新 mesh 时间
		if tStats.inMesh {
			tStats.meshTime += ps.params.DecayInterval * time.Duration(intervals)

			// 激活 mesh 消息投递统计
			if !tStats.meshMessageDeliveriesActive {
				elapsed := time.Since(tStats.graftTime)
				if elapsed >= params.MeshMessageDeliveriesActivation {
					tStats.meshMessageDeliveriesActive = true
				}
			}
		}

		// 衰减首次消息投递
		decayFactor := math.Pow(params.FirstMessageDeliveriesDecay, float64(intervals))
		tStats.firstMessageDeliveries *= decayFactor

		// 衰减 mesh 消息投递
		meshDecay := math.Pow(params.MeshMessageDeliveriesDecay, float64(intervals))
		tStats.meshMessageDeliveries *= meshDecay

		// 衰减 mesh 失败惩罚
		failDecay := math.Pow(params.MeshFailurePenaltyDecay, float64(intervals))
		tStats.meshFailurePenalty *= failDecay

		// 衰减无效消息
		invalidDecay := math.Pow(params.InvalidMessageDeliveriesDecay, float64(intervals))
		tStats.invalidMessages *= invalidDecay

		// 清理接近零的值
		if tStats.firstMessageDeliveries < ps.params.DecayToZero {
			tStats.firstMessageDeliveries = 0
		}
		if tStats.meshMessageDeliveries < ps.params.DecayToZero {
			tStats.meshMessageDeliveries = 0
		}
		if tStats.meshFailurePenalty < ps.params.DecayToZero {
			tStats.meshFailurePenalty = 0
		}
		if tStats.invalidMessages < ps.params.DecayToZero {
			tStats.invalidMessages = 0
		}
	}

	// 衰减行为惩罚
	behaviourDecay := math.Pow(ps.params.BehaviourPenaltyDecay, float64(intervals))
	stats.behaviourPenalty *= behaviourDecay
	if stats.behaviourPenalty < ps.params.DecayToZero {
		stats.behaviourPenalty = 0
	}
}

// ============================================================================
//                              阈值检查
// ============================================================================

// IsBelowGossipThreshold 检查是否低于 gossip 阈值
func (ps *PeerScorer) IsBelowGossipThreshold(peer types.NodeID) bool {
	return ps.Score(peer) < ps.gossipThreshold
}

// IsBelowPublishThreshold 检查是否低于发布阈值
func (ps *PeerScorer) IsBelowPublishThreshold(peer types.NodeID) bool {
	return ps.Score(peer) < ps.publishThreshold
}

// IsBelowGraylistThreshold 检查是否低于灰名单阈值
func (ps *PeerScorer) IsBelowGraylistThreshold(peer types.NodeID) bool {
	return ps.Score(peer) < ps.graylistThreshold
}

// IsAboveAcceptPXThreshold 检查是否高于 PX 接受阈值
func (ps *PeerScorer) IsAboveAcceptPXThreshold(peer types.NodeID) bool {
	return ps.Score(peer) > ps.acceptPXThreshold
}

// ============================================================================
//                              主题参数管理
// ============================================================================

// SetTopicParams 设置主题参数
func (ps *PeerScorer) SetTopicParams(topic string, params *TopicScoreParams) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.topicParams[topic] = params
}

// RemoveTopicParams 移除主题参数
func (ps *PeerScorer) RemoveTopicParams(topic string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	delete(ps.topicParams, topic)
}

// ============================================================================
//                              辅助方法
// ============================================================================

// getOrCreateStats 获取或创建 peer 统计
func (ps *PeerScorer) getOrCreateStats(peer types.NodeID) *peerScoreStats {
	stats, exists := ps.peerStats[peer]
	if !exists {
		now := time.Now()
		stats = &peerScoreStats{
			connected:  true,
			firstSeen:  now,
			lastSeen:   now,
			topicStats: make(map[string]*topicScoreStats),
		}
		ps.peerStats[peer] = stats
	}
	stats.lastSeen = time.Now()
	return stats
}

// getOrCreateTopicStats 获取或创建主题统计
func (ps *PeerScorer) getOrCreateTopicStats(stats *peerScoreStats, topic string) *topicScoreStats {
	tStats, exists := stats.topicStats[topic]
	if !exists {
		tStats = &topicScoreStats{}
		stats.topicStats[topic] = tStats
	}
	return tStats
}

// GetPeerScore 获取 peer 评分详情
func (ps *PeerScorer) GetPeerScore(peer types.NodeID) (float64, map[string]float64) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats, exists := ps.peerStats[peer]
	if !exists {
		return 0, nil
	}

	totalScore := ps.computeScore(peer, stats)
	topicScores := make(map[string]float64)

	for topic, tStats := range stats.topicStats {
		params := ps.getTopicParams(topic)
		topicScores[topic] = ps.computeTopicScore(tStats, params)
	}

	return totalScore, topicScores
}

// Reset 重置评分器
func (ps *PeerScorer) Reset() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.peerStats = make(map[types.NodeID]*peerScoreStats)
	ps.peerIPs = make(map[types.NodeID]string)
	ps.ipPeers = make(map[string]map[types.NodeID]struct{})
	ps.lastDecay = time.Now()
}

