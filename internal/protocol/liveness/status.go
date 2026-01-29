// Package liveness 实现存活检测服务
package liveness

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// peerStatus 节点状态
type peerStatus struct {
	peerID     string
	alive      bool
	lastSeen   time.Time
	lastRTT    time.Duration
	rttSamples []time.Duration
	failCount  int
	
	// 增强统计字段
	minRTT       time.Duration // 历史最小 RTT
	maxRTT       time.Duration // 历史最大 RTT
	totalPings   int           // 总 Ping 次数
	successCount int           // 成功次数
	
	mu sync.RWMutex
}

// newPeerStatus 创建节点状态
func newPeerStatus(peerID string) *peerStatus {
	return &peerStatus{
		peerID:     peerID,
		alive:      false,
		rttSamples: make([]time.Duration, 0, 10),
		minRTT:     0, // 0 表示尚未记录
		maxRTT:     0,
	}
}

// recordSuccess 记录成功
func (ps *peerStatus) recordSuccess(rtt time.Duration) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.alive = true
	ps.lastSeen = time.Now()
	ps.lastRTT = rtt
	ps.failCount = 0

	// 更新 RTT 样本（用于计算滑动窗口平均值）
	ps.rttSamples = append(ps.rttSamples, rtt)
	
	// 限制样本数量（滑动窗口）
	maxSamples := 10
	if len(ps.rttSamples) > maxSamples {
		ps.rttSamples = ps.rttSamples[len(ps.rttSamples)-maxSamples:]
	}
	
	// 更新 Min/Max RTT（历史极值）
	if ps.minRTT == 0 || rtt < ps.minRTT {
		ps.minRTT = rtt
	}
	if rtt > ps.maxRTT {
		ps.maxRTT = rtt
	}
	
	// 更新计数
	ps.totalPings++
	ps.successCount++
}

// recordFailure 记录失败
func (ps *peerStatus) recordFailure() {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.failCount++
	ps.totalPings++ // 失败也计入总次数
	
	// 达到阈值时标记为下线
	if ps.failCount >= 3 { // 默认阈值
		ps.alive = false
	}
}

// getStatus 获取状态
func (ps *peerStatus) getStatus() interfaces.LivenessStatus {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	avgRTT := ps.calculateAvgRTT()
	
	// 计算成功率
	var successRate float64
	if ps.totalPings > 0 {
		successRate = float64(ps.successCount) / float64(ps.totalPings)
	}

	return interfaces.LivenessStatus{
		Alive:        ps.alive,
		LastSeen:     ps.lastSeen,
		LastRTT:      ps.lastRTT,
		AvgRTT:       avgRTT,
		MinRTT:       ps.minRTT,
		MaxRTT:       ps.maxRTT,
		FailCount:    ps.failCount,
		TotalPings:   ps.totalPings,
		SuccessCount: ps.successCount,
		SuccessRate:  successRate,
	}
}

// calculateAvgRTT 计算平均RTT (需要持有锁)
func (ps *peerStatus) calculateAvgRTT() time.Duration {
	if len(ps.rttSamples) == 0 {
		return 0
	}

	var sum time.Duration
	for _, rtt := range ps.rttSamples {
		sum += rtt
	}

	return sum / time.Duration(len(ps.rttSamples))
}

// isAlive 检查是否存活
func (ps *peerStatus) isAlive() bool {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.alive
}
