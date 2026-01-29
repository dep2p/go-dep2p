// Package stability 实现连接稳定性跟踪
//
// 本包提供震荡检测器，用于识别和处理不稳定的连接。
package stability

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("realm/stability")

// ============================================================================
//                              配置常量
// ============================================================================

const (
	// FlapWindowDuration 震荡检测时间窗口
	// 在此时间窗口内发生的状态转换会被计入震荡检测
	FlapWindowDuration = 60 * time.Second

	// FlapThreshold 震荡阈值
	// 在时间窗口内超过此次数的状态转换被视为震荡
	FlapThreshold = 3

	// FlappingRecovery 震荡恢复时间
	// 被标记为震荡后，需要稳定此时间才能解除震荡状态
	FlappingRecovery = 5 * time.Minute
)

// ============================================================================
//                              震荡检测器
// ============================================================================

// ConnectionStabilityTracker 连接稳定性跟踪器
//
// 用于检测和处理不稳定的连接（震荡）。
// 当节点在短时间内频繁断开重连时，暂时抑制状态变更通知，
// 避免对上层应用造成干扰。
//
// 震荡检测规则：
//   - 在 60 秒内发生 >= 3 次状态转换视为震荡
//   - 震荡状态下，抑制状态变更通知
//   - 稳定 5 分钟后解除震荡状态
type ConnectionStabilityTracker struct {
	mu sync.RWMutex

	// 状态转换记录
	// key: peer_id, value: 状态转换时间列表
	transitions map[string][]time.Time

	// 震荡状态
	// key: peer_id, value: 被标记为震荡的时间
	flapping map[string]time.Time

	// 配置
	windowDuration time.Duration
	threshold      int
	recoveryTime   time.Duration
}

// NewConnectionStabilityTracker 创建震荡检测器
func NewConnectionStabilityTracker() *ConnectionStabilityTracker {
	return &ConnectionStabilityTracker{
		transitions:    make(map[string][]time.Time),
		flapping:       make(map[string]time.Time),
		windowDuration: FlapWindowDuration,
		threshold:      FlapThreshold,
		recoveryTime:   FlappingRecovery,
	}
}

// SetConfig 设置配置参数
func (cst *ConnectionStabilityTracker) SetConfig(windowDuration time.Duration, threshold int, recoveryTime time.Duration) {
	cst.mu.Lock()
	defer cst.mu.Unlock()

	if windowDuration > 0 {
		cst.windowDuration = windowDuration
	}
	if threshold > 0 {
		cst.threshold = threshold
	}
	if recoveryTime > 0 {
		cst.recoveryTime = recoveryTime
	}
}

// RecordTransition 记录状态转换
//
// 每次连接状态发生变化时调用。
// 返回是否检测到新的震荡状态。
func (cst *ConnectionStabilityTracker) RecordTransition(peerID string) bool {
	cst.mu.Lock()
	defer cst.mu.Unlock()

	now := time.Now()

	// 记录转换
	cst.transitions[peerID] = append(cst.transitions[peerID], now)

	// 清理过期记录
	cst.cleanupTransitions(peerID, now)

	// 检查是否震荡
	transitionCount := len(cst.transitions[peerID])
	if transitionCount >= cst.threshold {
		// 如果尚未标记为震荡，现在标记
		if _, isFlapping := cst.flapping[peerID]; !isFlapping {
			cst.flapping[peerID] = now
			logger.Warn("检测到连接震荡",
				"peerID", truncateID(peerID),
				"transitions", transitionCount,
				"window", cst.windowDuration)
			return true
		}
	}

	return false
}

// cleanupTransitions 清理过期的转换记录
func (cst *ConnectionStabilityTracker) cleanupTransitions(peerID string, now time.Time) {
	transitions := cst.transitions[peerID]
	windowStart := now.Add(-cst.windowDuration)

	// 保留窗口内的记录
	validTransitions := make([]time.Time, 0, len(transitions))
	for _, t := range transitions {
		if t.After(windowStart) {
			validTransitions = append(validTransitions, t)
		}
	}

	if len(validTransitions) == 0 {
		delete(cst.transitions, peerID)
	} else {
		cst.transitions[peerID] = validTransitions
	}
}

// IsFlapping 检查节点是否处于震荡状态
func (cst *ConnectionStabilityTracker) IsFlapping(peerID string) bool {
	cst.mu.RLock()
	flappingSince, isFlapping := cst.flapping[peerID]
	cst.mu.RUnlock()

	if !isFlapping {
		return false
	}

	// 检查是否已恢复
	if time.Since(flappingSince) > cst.recoveryTime {
		// 已稳定足够长时间，解除震荡状态
		cst.mu.Lock()
		delete(cst.flapping, peerID)
		cst.mu.Unlock()

		logger.Info("连接震荡已恢复",
			"peerID", truncateID(peerID),
			"stableDuration", time.Since(flappingSince))
		return false
	}

	return true
}

// ShouldSuppressStateChange 检查是否应该抑制状态变更
//
// 当节点处于震荡状态时，返回 true，
// 表示应该抑制状态变更通知，避免干扰上层应用。
func (cst *ConnectionStabilityTracker) ShouldSuppressStateChange(peerID string) bool {
	return cst.IsFlapping(peerID)
}

// GetTransitionCount 获取节点在时间窗口内的状态转换次数
func (cst *ConnectionStabilityTracker) GetTransitionCount(peerID string) int {
	cst.mu.RLock()
	defer cst.mu.RUnlock()

	now := time.Now()
	windowStart := now.Add(-cst.windowDuration)

	count := 0
	for _, t := range cst.transitions[peerID] {
		if t.After(windowStart) {
			count++
		}
	}

	return count
}

// GetFlappingPeers 获取所有震荡中的节点
func (cst *ConnectionStabilityTracker) GetFlappingPeers() []string {
	cst.mu.RLock()
	defer cst.mu.RUnlock()

	var peers []string
	now := time.Now()

	for peerID, flappingSince := range cst.flapping {
		// 只返回尚未恢复的
		if now.Sub(flappingSince) <= cst.recoveryTime {
			peers = append(peers, peerID)
		}
	}

	return peers
}

// ResetPeer 重置节点的震荡状态
//
// 当节点正常断开或被移除时调用。
func (cst *ConnectionStabilityTracker) ResetPeer(peerID string) {
	cst.mu.Lock()
	defer cst.mu.Unlock()

	delete(cst.transitions, peerID)
	delete(cst.flapping, peerID)
}

// Cleanup 清理过期数据
func (cst *ConnectionStabilityTracker) Cleanup() {
	cst.mu.Lock()
	defer cst.mu.Unlock()

	now := time.Now()

	// 清理已恢复的震荡状态
	for peerID, flappingSince := range cst.flapping {
		if now.Sub(flappingSince) > cst.recoveryTime {
			delete(cst.flapping, peerID)
		}
	}

	// 清理过期的转换记录
	windowStart := now.Add(-cst.windowDuration)
	for peerID, transitions := range cst.transitions {
		validTransitions := make([]time.Time, 0)
		for _, t := range transitions {
			if t.After(windowStart) {
				validTransitions = append(validTransitions, t)
			}
		}
		if len(validTransitions) == 0 {
			delete(cst.transitions, peerID)
		} else {
			cst.transitions[peerID] = validTransitions
		}
	}
}

// truncateID 安全截断 ID 用于日志
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
