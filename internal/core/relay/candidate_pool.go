package relay

import (
	"context"
	"time"
	
	"github.com/dep2p/go-dep2p/internal/core/nat"
)

// NewRelayCandidatePool 创建中继候选池
func NewRelayCandidatePool(realmID string) *RelayCandidatePool {
	return &RelayCandidatePool{
		realmID:    realmID,
		candidates: make(map[string]*RelayCandidate),
		maxSize:    50, // 最多缓存 50 个候选
		selector:   NewSelector(),
	}
}

// Add 添加候选
//
// 只有公网可达的节点才能成为候选。
func (p *RelayCandidatePool) Add(candidate *RelayCandidate) {
	if candidate == nil {
		return
	}
	
	p.mu.Lock()
	defer p.mu.Unlock()
	
	// 只有公网可达的才能成为候选
	if candidate.Reachability != nat.ReachabilityPublic {
		return
	}
	
	// 检查是否达到最大容量
	if len(p.candidates) >= p.maxSize {
		// 移除最旧的候选
		p.removeOldest()
	}
	
	// 如果未设置 LastSeen，使用当前时间
	if candidate.LastSeen.IsZero() {
		candidate.LastSeen = time.Now()
	}
	
	// 添加到池中
	p.candidates[candidate.PeerID] = candidate
}

// Remove 移除候选
func (p *RelayCandidatePool) Remove(peerID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	delete(p.candidates, peerID)
}

// UpdateReachability 更新候选的可达性状态
//
// 如果状态变为非公网可达，则自动移除该候选。
func (p *RelayCandidatePool) UpdateReachability(peerID string, reachability nat.Reachability) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	candidate, exists := p.candidates[peerID]
	if !exists {
		return
	}
	
	// 更新可达性状态
	candidate.Reachability = reachability
	
	// 如果不再公网可达，移除候选
	if reachability != nat.ReachabilityPublic {
		delete(p.candidates, peerID)
	}
}

// SetMetrics 设置指标收集器
func (p *RelayCandidatePool) SetMetrics(metrics *CandidateMetrics) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.metrics = metrics
}

// SelectBest 选择最优候选
//
// 使用 Selector 根据延迟、容量、可靠性等因素评分，
// 返回评分最高的候选。
func (p *RelayCandidatePool) SelectBest() *RelayCandidate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if len(p.candidates) == 0 {
		return nil
	}
	
	// 转换为 RelayInfo 列表供 Selector 评分
	relays := make([]RelayInfo, 0, len(p.candidates))
	candidateMap := make(map[string]*RelayCandidate)
	
	for _, candidate := range p.candidates {
		var relayInfo RelayInfo
		
		// 尝试从指标收集器获取真实数据
		if p.metrics != nil {
			if metrics := p.metrics.GetMetrics(candidate.PeerID); metrics != nil {
				// 使用真实指标
				relayInfo = RelayInfo{
					ID:          candidate.PeerID,
					Latency:     int64(metrics.Latency.Milliseconds()),
					Capacity:    metrics.Capacity,
					Reliability: metrics.Reliability,
				}
			} else {
				// 无指标数据，使用默认值
				relayInfo = RelayInfo{
					ID:          candidate.PeerID,
					Latency:     100,  // 默认 100ms
					Capacity:    0.8,  // 默认容量
					Reliability: 0.9,  // 默认可靠性
				}
			}
		} else {
			// 无指标收集器，基于最后活跃时间简化评分
			ageSeconds := int64(time.Since(candidate.LastSeen).Seconds())
			
			relayInfo = RelayInfo{
				ID:          candidate.PeerID,
				Latency:     ageSeconds * 10, // 年龄越大延迟越高
				Capacity:    0.8,
				Reliability: 0.9,
			}
		}
		
		relays = append(relays, relayInfo)
		candidateMap[candidate.PeerID] = candidate
	}
	
	// 使用 Selector 选择最优
	best := p.selector.SelectBest(relays, "")
	if best.ID == "" {
		return nil
	}
	
	return candidateMap[best.ID]
}

// Count 返回候选数量
func (p *RelayCandidatePool) Count() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	return len(p.candidates)
}

// GetAll 返回所有候选（拷贝）
func (p *RelayCandidatePool) GetAll() []*RelayCandidate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	result := make([]*RelayCandidate, 0, len(p.candidates))
	for _, candidate := range p.candidates {
		// 创建拷贝
		c := &RelayCandidate{
			PeerID:       candidate.PeerID,
			Addrs:        make([]string, len(candidate.Addrs)),
			Reachability: candidate.Reachability,
			LastSeen:     candidate.LastSeen,
			Score:        candidate.Score,
		}
		copy(c.Addrs, candidate.Addrs)
		result = append(result, c)
	}
	
	return result
}

// removeOldest 移除最旧的候选（内部方法，调用者需要持有锁）
func (p *RelayCandidatePool) removeOldest() {
	if len(p.candidates) == 0 {
		return
	}
	
	var oldestPeerID string
	var oldestTime time.Time
	
	for peerID, candidate := range p.candidates {
		if oldestPeerID == "" || candidate.LastSeen.Before(oldestTime) {
			oldestPeerID = peerID
			oldestTime = candidate.LastSeen
		}
	}
	
	if oldestPeerID != "" {
		delete(p.candidates, oldestPeerID)
	}
}

// Clear 清空所有候选
func (p *RelayCandidatePool) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	p.candidates = make(map[string]*RelayCandidate)
}

// RefreshOnNetworkChange 网络变化时刷新候选池
//
// 当网络变化时（如 4G→WiFi），需要：
// 1. 清理失效的候选（长时间未见）
// 2. 重新评估现有候选的延迟指标
func (p *RelayCandidatePool) RefreshOnNetworkChange(_ context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	logger.Info("网络变化，刷新中继候选池")
	
	now := time.Now()
	staleThreshold := 5 * time.Minute // 5 分钟未见视为失效
	
	staleCount := 0
	for peerID, candidate := range p.candidates {
		// 清理失效候选
		if now.Sub(candidate.LastSeen) > staleThreshold {
			delete(p.candidates, peerID)
			staleCount++
			logger.Debug("移除失效中继候选",
				"peerID", peerID,
				"lastSeen", candidate.LastSeen)
			continue
		}
		
		// 重置评分（网络变化后需要重新评估）
		candidate.Score = 0
		
		logger.Debug("重置中继候选评分",
			"peerID", peerID)
	}
	
	logger.Info("中继候选池刷新完成",
		"removed", staleCount,
		"remaining", len(p.candidates))
}
