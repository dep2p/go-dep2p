package member

import (
	"sync"
	"time"
)

// ============================================================================
//                              配置常量
// ============================================================================

const (
	// DisconnectProtection 断开保护期
	// 成员被移除后，在此时间内拒绝重新添加
	// 防止快速断开重连导致的状态不一致
	DisconnectProtection = 30 * time.Second

	// ProtectionCacheExpiry 保护缓存过期时间
	// 超过此时间的保护记录会被清理
	ProtectionCacheExpiry = 5 * time.Minute
)

// ============================================================================
//                              断开保护跟踪器
// ============================================================================

// DisconnectProtectionTracker 断开保护跟踪器
//
// 跟踪最近被移除的成员，在保护期内拒绝重新添加。
// 这可以防止以下场景：
//   - 成员被移除后立即尝试重新加入（可能是攻击）
//   - 网络问题导致的快速移除-重新添加循环
//   - 分布式状态不一致（部分节点认为已移除，部分认为在线）
type DisconnectProtectionTracker struct {
	mu sync.RWMutex

	// 最近移除的成员
	// key: peer_id, value: 移除时间
	removedMembers map[string]time.Time

	// 配置
	protectionDuration time.Duration
}

// NewDisconnectProtectionTracker 创建断开保护跟踪器
func NewDisconnectProtectionTracker() *DisconnectProtectionTracker {
	return &DisconnectProtectionTracker{
		removedMembers:     make(map[string]time.Time),
		protectionDuration: DisconnectProtection,
	}
}

// SetProtectionDuration 设置保护期时长
func (dpt *DisconnectProtectionTracker) SetProtectionDuration(duration time.Duration) {
	dpt.mu.Lock()
	defer dpt.mu.Unlock()
	dpt.protectionDuration = duration
}

// OnMemberRemoved 记录成员被移除
//
// 当成员被移除时调用，开始保护期。
func (dpt *DisconnectProtectionTracker) OnMemberRemoved(peerID string) {
	dpt.mu.Lock()
	defer dpt.mu.Unlock()

	dpt.removedMembers[peerID] = time.Now()

	logger.Debug("开始断开保护期",
		"peerID", truncateID(peerID),
		"duration", dpt.protectionDuration)
}

// IsProtected 检查成员是否在保护期内
//
// 返回 true 表示在保护期内，应该拒绝重新添加。
// 返回 false 表示可以重新添加。
func (dpt *DisconnectProtectionTracker) IsProtected(peerID string) bool {
	dpt.mu.RLock()
	removedAt, exists := dpt.removedMembers[peerID]
	protectionDuration := dpt.protectionDuration
	dpt.mu.RUnlock()

	if !exists {
		return false
	}

	// 检查是否仍在保护期内
	if time.Since(removedAt) <= protectionDuration {
		logger.Debug("成员仍在保护期内",
			"peerID", truncateID(peerID),
			"remaining", protectionDuration-time.Since(removedAt))
		return true
	}

	// 保护期已过，清理记录
	dpt.mu.Lock()
	delete(dpt.removedMembers, peerID)
	dpt.mu.Unlock()

	return false
}

// GetRemainingProtection 获取剩余保护时间
//
// 返回 0 表示不在保护期内。
func (dpt *DisconnectProtectionTracker) GetRemainingProtection(peerID string) time.Duration {
	dpt.mu.RLock()
	removedAt, exists := dpt.removedMembers[peerID]
	protectionDuration := dpt.protectionDuration
	dpt.mu.RUnlock()

	if !exists {
		return 0
	}

	remaining := protectionDuration - time.Since(removedAt)
	if remaining < 0 {
		return 0
	}

	return remaining
}

// ClearProtection 清除保护状态
//
// 在特殊情况下（如管理员操作）允许立即重新添加。
func (dpt *DisconnectProtectionTracker) ClearProtection(peerID string) {
	dpt.mu.Lock()
	defer dpt.mu.Unlock()

	delete(dpt.removedMembers, peerID)
	logger.Debug("清除断开保护",
		"peerID", truncateID(peerID))
}

// GetProtectedPeers 获取所有在保护期内的节点
func (dpt *DisconnectProtectionTracker) GetProtectedPeers() []string {
	dpt.mu.RLock()
	defer dpt.mu.RUnlock()

	now := time.Now()
	var peers []string

	for peerID, removedAt := range dpt.removedMembers {
		if now.Sub(removedAt) <= dpt.protectionDuration {
			peers = append(peers, peerID)
		}
	}

	return peers
}

// Cleanup 清理过期记录
func (dpt *DisconnectProtectionTracker) Cleanup() {
	dpt.mu.Lock()
	defer dpt.mu.Unlock()

	now := time.Now()
	// 使用保护期作为清理阈值
	// 超过保护期的记录可以清理（保护期已过，不再需要保护）
	cleanupThreshold := dpt.protectionDuration

	for peerID, removedAt := range dpt.removedMembers {
		// 超过保护期的记录可以清理
		if now.Sub(removedAt) > cleanupThreshold {
			delete(dpt.removedMembers, peerID)
		}
	}
}

// Reset 重置所有保护状态
func (dpt *DisconnectProtectionTracker) Reset() {
	dpt.mu.Lock()
	defer dpt.mu.Unlock()
	dpt.removedMembers = make(map[string]time.Time)
}
