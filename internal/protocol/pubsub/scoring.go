// Package pubsub 实现发布订阅协议
package pubsub

import (
	"math"
	"sync"
	"time"
)

// ============================================================================
//                              评分参数
// ============================================================================

// ScoreParams 全局评分参数
type ScoreParams struct {
	// DecayInterval 衰减间隔
	DecayInterval time.Duration

	// DecayToZero 衰减到零的阈值
	DecayToZero float64

	// RetainScore 断开后保留评分的时间
	RetainScore time.Duration

	// AppSpecificScore 应用层评分函数
	AppSpecificScore func(peerID string) float64

	// AppSpecificWeight 应用层评分权重 (P5)
	AppSpecificWeight float64

	// IPColocationFactorWeight IP 协同惩罚权重 (P6)
	IPColocationFactorWeight float64

	// IPColocationFactorThreshold IP 协同阈值
	IPColocationFactorThreshold int

	// IPColocationFactorWhitelist IP 白名单
	IPColocationFactorWhitelist map[string]struct{}

	// BehaviourPenaltyWeight 行为惩罚权重 (P7)
	BehaviourPenaltyWeight float64

	// BehaviourPenaltyThreshold 行为惩罚阈值
	BehaviourPenaltyThreshold float64

	// BehaviourPenaltyDecay 行为惩罚衰减因子
	BehaviourPenaltyDecay float64
}

// DefaultScoreParams 返回默认评分参数
func DefaultScoreParams() *ScoreParams {
	return &ScoreParams{
		DecayInterval:               time.Second,
		DecayToZero:                 0.01,
		RetainScore:                 10 * time.Minute,
		AppSpecificWeight:           1.0,
		IPColocationFactorWeight:    -10.0,
		IPColocationFactorThreshold: 5,
		IPColocationFactorWhitelist: make(map[string]struct{}),
		BehaviourPenaltyWeight:      -1.0,
		BehaviourPenaltyThreshold:   1.0,
		BehaviourPenaltyDecay:       0.999,
	}
}

// TopicScoreParams 主题评分参数
type TopicScoreParams struct {
	// TopicWeight 主题权重
	TopicWeight float64

	// TimeInMeshWeight Mesh 时间权重 (P1)
	TimeInMeshWeight float64

	// TimeInMeshQuantum Mesh 时间量子
	TimeInMeshQuantum time.Duration

	// TimeInMeshCap Mesh 时间上限
	TimeInMeshCap float64

	// FirstMessageDeliveriesWeight 首次消息投递权重 (P2)
	FirstMessageDeliveriesWeight float64

	// FirstMessageDeliveriesDecay 首次投递衰减因子
	FirstMessageDeliveriesDecay float64

	// FirstMessageDeliveriesCap 首次投递上限
	FirstMessageDeliveriesCap float64

	// MeshMessageDeliveriesWeight Mesh 消息投递权重 (P3)
	MeshMessageDeliveriesWeight float64

	// MeshMessageDeliveriesDecay Mesh 投递衰减因子
	MeshMessageDeliveriesDecay float64

	// MeshMessageDeliveriesCap Mesh 投递上限
	MeshMessageDeliveriesCap float64

	// MeshMessageDeliveriesThreshold Mesh 投递阈值
	MeshMessageDeliveriesThreshold float64

	// MeshMessageDeliveriesWindow Mesh 投递窗口
	MeshMessageDeliveriesWindow time.Duration

	// MeshMessageDeliveriesActivation Mesh 投递激活时间
	MeshMessageDeliveriesActivation time.Duration

	// MeshFailurePenaltyWeight Mesh 失败惩罚权重 (P3b)
	MeshFailurePenaltyWeight float64

	// MeshFailurePenaltyDecay Mesh 失败惩罚衰减因子
	MeshFailurePenaltyDecay float64

	// InvalidMessageDeliveriesWeight 无效消息惩罚权重 (P4)
	InvalidMessageDeliveriesWeight float64

	// InvalidMessageDeliveriesDecay 无效消息衰减因子
	InvalidMessageDeliveriesDecay float64
}

// DefaultTopicScoreParams 返回默认主题评分参数
func DefaultTopicScoreParams() *TopicScoreParams {
	return &TopicScoreParams{
		TopicWeight:                     1.0,
		TimeInMeshWeight:                0.01,
		TimeInMeshQuantum:               time.Second,
		TimeInMeshCap:                   3600,
		FirstMessageDeliveriesWeight:    1.0,
		FirstMessageDeliveriesDecay:     0.9999,
		FirstMessageDeliveriesCap:       100,
		MeshMessageDeliveriesWeight:     -1.0,
		MeshMessageDeliveriesDecay:      0.9999,
		MeshMessageDeliveriesCap:        1000,
		MeshMessageDeliveriesThreshold:  1,
		MeshMessageDeliveriesWindow:     10 * time.Millisecond,
		MeshMessageDeliveriesActivation: 5 * time.Second,
		MeshFailurePenaltyWeight:        -1.0,
		MeshFailurePenaltyDecay:         0.999,
		InvalidMessageDeliveriesWeight:  -1000.0,
		InvalidMessageDeliveriesDecay:   0.9999,
	}
}

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

	// 阈值
	gossipThreshold   float64
	publishThreshold  float64
	graylistThreshold float64
	acceptPXThreshold float64

	// topicParams 主题评分参数
	topicParams map[string]*TopicScoreParams

	// peerStats peer 统计
	peerStats map[string]*peerScoreStats

	// peerIPs peer IP 映射（用于 IP 协同检测）
	peerIPs map[string]string

	// ipPeers IP 到 peer 列表的映射
	ipPeers map[string]map[string]struct{}

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
		peerStats:         make(map[string]*peerScoreStats),
		peerIPs:           make(map[string]string),
		ipPeers:           make(map[string]map[string]struct{}),
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
func (ps *PeerScorer) Score(peerID string) float64 {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats, exists := ps.peerStats[peerID]
	if !exists {
		return 0
	}

	return ps.computeScore(peerID, stats)
}

// computeScore 内部评分计算
func (ps *PeerScorer) computeScore(peerID string, stats *peerScoreStats) float64 {
	var score float64

	// 主题评分
	for topic, tStats := range stats.topicStats {
		tParams := ps.getTopicParams(topic)
		topicScore := ps.computeTopicScore(tStats, tParams)
		score += topicScore * tParams.TopicWeight
	}

	// 应用层评分 (P5)
	if ps.params.AppSpecificScore != nil {
		appScore := ps.params.AppSpecificScore(peerID)
		score += appScore * ps.params.AppSpecificWeight
	} else {
		score += stats.appScore * ps.params.AppSpecificWeight
	}

	// IP 协同惩罚 (P6)
	ipScore := ps.computeIPColocationScore(peerID)
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
func (ps *PeerScorer) computeIPColocationScore(peerID string) float64 {
	ip, exists := ps.peerIPs[peerID]
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
func (ps *PeerScorer) AddPeer(peerID string, ip string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if _, exists := ps.peerStats[peerID]; exists {
		return
	}

	now := time.Now()
	ps.peerStats[peerID] = &peerScoreStats{
		connected:  true,
		firstSeen:  now,
		lastSeen:   now,
		topicStats: make(map[string]*topicScoreStats),
	}

	// 记录 IP
	if ip != "" {
		ps.peerIPs[peerID] = ip
		if ps.ipPeers[ip] == nil {
			ps.ipPeers[ip] = make(map[string]struct{})
		}
		ps.ipPeers[ip][peerID] = struct{}{}
	}
}

// RemovePeer 移除 peer
func (ps *PeerScorer) RemovePeer(peerID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats, exists := ps.peerStats[peerID]
	if !exists {
		return
	}

	stats.connected = false

	// 从 IP 映射中移除
	if ip, exists := ps.peerIPs[peerID]; exists {
		if peers, exists := ps.ipPeers[ip]; exists {
			delete(peers, peerID)
			if len(peers) == 0 {
				delete(ps.ipPeers, ip)
			}
		}
		delete(ps.peerIPs, peerID)
	}
}

// Graft peer 加入 mesh
func (ps *PeerScorer) Graft(peerID string, topic string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
	tStats := ps.getOrCreateTopicStats(stats, topic)

	tStats.inMesh = true
	tStats.graftTime = time.Now()
	tStats.meshMessageDeliveriesActive = false
}

// Prune peer 离开 mesh
func (ps *PeerScorer) Prune(peerID string, topic string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
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
func (ps *PeerScorer) ValidateMessage(peerID string, topic string, isFirst, isValid bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
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
func (ps *PeerScorer) DuplicateMessage(peerID string, topic string, wasFirst bool) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
	tStats := ps.getOrCreateTopicStats(stats, topic)

	// 重复消息但在 mesh 投递窗口内也计入
	if tStats.inMesh && wasFirst {
		tStats.meshMessageDeliveries++
	}
}

// BrokenPromise 记录未履行的 IWANT
func (ps *PeerScorer) BrokenPromise(peerID string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
	stats.behaviourPenalty++
}

// DeliveryFailed 记录消息发送失败
//
// P2-L4 修复：通过评分机制惩罚发送失败的节点，使其在下次 prune 时被优先移除。
func (ps *PeerScorer) DeliveryFailed(peerID string, _ string) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
	// 增加行为惩罚（P7）
	stats.behaviourPenalty += 0.5
}

// SetAppScore 设置应用层评分
func (ps *PeerScorer) SetAppScore(peerID string, score float64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	stats := ps.getOrCreateStats(peerID)
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

	for peerID, stats := range ps.peerStats {
		ps.decayPeerStats(stats, int(decayIntervals))

		// 清理断开连接且评分归零的 peer
		if !stats.connected && now.Sub(stats.lastSeen) > ps.params.RetainScore {
			delete(ps.peerStats, peerID)
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
func (ps *PeerScorer) IsBelowGossipThreshold(peerID string) bool {
	return ps.Score(peerID) < ps.gossipThreshold
}

// IsBelowPublishThreshold 检查是否低于发布阈值
func (ps *PeerScorer) IsBelowPublishThreshold(peerID string) bool {
	return ps.Score(peerID) < ps.publishThreshold
}

// IsBelowGraylistThreshold 检查是否低于灰名单阈值
func (ps *PeerScorer) IsBelowGraylistThreshold(peerID string) bool {
	return ps.Score(peerID) < ps.graylistThreshold
}

// IsAboveAcceptPXThreshold 检查是否高于 PX 接受阈值
func (ps *PeerScorer) IsAboveAcceptPXThreshold(peerID string) bool {
	return ps.Score(peerID) > ps.acceptPXThreshold
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
func (ps *PeerScorer) getOrCreateStats(peerID string) *peerScoreStats {
	stats, exists := ps.peerStats[peerID]
	if !exists {
		now := time.Now()
		stats = &peerScoreStats{
			connected:  true,
			firstSeen:  now,
			lastSeen:   now,
			topicStats: make(map[string]*topicScoreStats),
		}
		ps.peerStats[peerID] = stats
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
func (ps *PeerScorer) GetPeerScore(peerID string) (float64, map[string]float64) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	stats, exists := ps.peerStats[peerID]
	if !exists {
		return 0, nil
	}

	totalScore := ps.computeScore(peerID, stats)
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

	ps.peerStats = make(map[string]*peerScoreStats)
	ps.peerIPs = make(map[string]string)
	ps.ipPeers = make(map[string]map[string]struct{})
	ps.lastDecay = time.Now()
}

// GetThresholds 获取所有阈值
func (ps *PeerScorer) GetThresholds() (gossip, publish, graylist, acceptPX float64) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.gossipThreshold, ps.publishThreshold, ps.graylistThreshold, ps.acceptPXThreshold
}

// SetThresholds 设置所有阈值
func (ps *PeerScorer) SetThresholds(gossip, publish, graylist, acceptPX float64) {
	ps.mu.Lock()
	defer ps.mu.Unlock()
	ps.gossipThreshold = gossip
	ps.publishThreshold = publish
	ps.graylistThreshold = graylist
	ps.acceptPXThreshold = acceptPX
}
