// Package member 实现成员管理
//
// 本文件实现防误判机制与 Manager 的集成。
package member

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/stability"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              防误判组件
// ============================================================================

// AntiFalsePositive 防误判机制集成
//
// 集成以下三个组件：
//   - MemberConnectionState: 成员连接状态机（重连宽限期）
//   - ConnectionStabilityTracker: 震荡检测器
//   - DisconnectProtectionTracker: 断开保护跟踪器
type AntiFalsePositive struct {
	mu sync.RWMutex

	// 成员状态机
	// key: peer_id, value: 连接状态机
	memberStates map[string]*MemberConnectionState

	// 震荡检测器
	stabilityTracker *stability.ConnectionStabilityTracker

	// 断开保护跟踪器
	protectionTracker *DisconnectProtectionTracker

	// 配置
	gracePeriod        time.Duration
	flapWindow         time.Duration
	flapThreshold      int
	protectionDuration time.Duration

	// 回调
	onMemberRemove func(peerID string)
}

// AntiFalsePositiveConfig 防误判配置
type AntiFalsePositiveConfig struct {
	// 重连宽限期
	GracePeriod time.Duration
	// 震荡检测窗口
	FlapWindow time.Duration
	// 震荡阈值
	FlapThreshold int
	// 断开保护期
	ProtectionDuration time.Duration
}

// DefaultAntiFalsePositiveConfig 返回默认配置
func DefaultAntiFalsePositiveConfig() *AntiFalsePositiveConfig {
	return &AntiFalsePositiveConfig{
		GracePeriod:        ReconnectGracePeriod,
		FlapWindow:         stability.FlapWindowDuration,
		FlapThreshold:      stability.FlapThreshold,
		ProtectionDuration: DisconnectProtection,
	}
}

// NewAntiFalsePositive 创建防误判机制
func NewAntiFalsePositive(config *AntiFalsePositiveConfig) *AntiFalsePositive {
	if config == nil {
		config = DefaultAntiFalsePositiveConfig()
	}

	afp := &AntiFalsePositive{
		memberStates:       make(map[string]*MemberConnectionState),
		stabilityTracker:   stability.NewConnectionStabilityTracker(),
		protectionTracker:  NewDisconnectProtectionTracker(),
		gracePeriod:        config.GracePeriod,
		flapWindow:         config.FlapWindow,
		flapThreshold:      config.FlapThreshold,
		protectionDuration: config.ProtectionDuration,
	}

	// 配置震荡检测器
	afp.stabilityTracker.SetConfig(config.FlapWindow, config.FlapThreshold, stability.FlappingRecovery)

	// 配置断开保护跟踪器
	afp.protectionTracker.SetProtectionDuration(config.ProtectionDuration)

	return afp
}

// SetOnMemberRemove 设置成员移除回调
func (afp *AntiFalsePositive) SetOnMemberRemove(callback func(peerID string)) {
	afp.mu.Lock()
	defer afp.mu.Unlock()
	afp.onMemberRemove = callback
}

// ============================================================================
//                              断开检测流程
// ============================================================================

// OnPeerDisconnected 处理节点断开事件
//
// 返回值：
//   - shouldRemove: 是否应立即移除成员（震荡状态下返回 false）
//   - inGracePeriod: 是否进入宽限期
func (afp *AntiFalsePositive) OnPeerDisconnected(peerID, realmID string) (shouldRemove, inGracePeriod bool) {
	afp.mu.Lock()

	// 1. 记录状态转换（震荡检测）
	isNewFlapping := afp.stabilityTracker.RecordTransition(peerID)
	if isNewFlapping {
		logger.Warn("检测到连接震荡，抑制断开处理",
			"peerID", truncateID(peerID))
	}

	// 2. 如果正在震荡，抑制状态变更
	if afp.stabilityTracker.ShouldSuppressStateChange(peerID) {
		afp.mu.Unlock()
		logger.Debug("震荡状态，抑制断开事件",
			"peerID", truncateID(peerID))
		return false, false
	}

	// 3. 获取或创建状态机
	state, exists := afp.memberStates[peerID]
	if !exists {
		state = NewMemberConnectionState(peerID, realmID)
		state.SetOnGraceTimeout(afp.onGraceTimeout)
		afp.memberStates[peerID] = state
	}

	afp.mu.Unlock()

	// 4. 触发状态机断开处理（启动宽限期）
	state.OnDisconnect()

	return false, true // 进入宽限期，不立即移除
}

// onGraceTimeout 宽限期超时回调
func (afp *AntiFalsePositive) onGraceTimeout(peerID string) {
	afp.mu.Lock()
	callback := afp.onMemberRemove
	afp.mu.Unlock()

	logger.Info("宽限期超时，触发成员移除",
		"peerID", truncateID(peerID))

	// 记录到断开保护跟踪器
	afp.protectionTracker.OnMemberRemoved(peerID)

	// 清理状态机
	afp.mu.Lock()
	delete(afp.memberStates, peerID)
	afp.mu.Unlock()

	// 调用回调移除成员
	if callback != nil {
		callback(peerID)
	}
}

// ============================================================================
//                              重连检测流程
// ============================================================================

// OnPeerReconnected 处理节点重连事件
//
// 返回值：
//   - recovered: 是否成功恢复（宽限期内重连）
//   - suppressedByFlapping: 是否被震荡检测抑制
func (afp *AntiFalsePositive) OnPeerReconnected(peerID string) (recovered, suppressedByFlapping bool) {
	afp.mu.Lock()

	// 1. 记录状态转换（震荡检测）
	afp.stabilityTracker.RecordTransition(peerID)

	// 2. 如果正在震荡，记录但不做状态变更
	if afp.stabilityTracker.ShouldSuppressStateChange(peerID) {
		afp.mu.Unlock()
		logger.Debug("震荡状态，抑制重连事件",
			"peerID", truncateID(peerID))
		return false, true
	}

	// 3. 获取状态机
	state, exists := afp.memberStates[peerID]
	if !exists {
		afp.mu.Unlock()
		return false, false
	}

	afp.mu.Unlock()

	// 4. 尝试在宽限期内恢复
	if state.OnReconnect() {
		logger.Info("宽限期内重连成功，恢复成员状态",
			"peerID", truncateID(peerID))
		return true, false
	}

	return false, false
}

// OnCommunication 处理通信事件
//
// 在宽限期内收到通信时，延长宽限期。
func (afp *AntiFalsePositive) OnCommunication(peerID string) {
	afp.mu.RLock()
	state, exists := afp.memberStates[peerID]
	afp.mu.RUnlock()

	if exists {
		state.OnCommunication()
	}
}

// ============================================================================
//                              添加成员检查
// ============================================================================

// ShouldRejectAdd 检查是否应该拒绝添加成员
//
// 返回值：
//   - reject: 是否应该拒绝
//   - reason: 拒绝原因
func (afp *AntiFalsePositive) ShouldRejectAdd(peerID string) (reject bool, reason string) {
	// 1. 检查断开保护期
	if afp.protectionTracker.IsProtected(peerID) {
		remaining := afp.protectionTracker.GetRemainingProtection(peerID)
		return true, "in disconnect protection period, remaining: " + remaining.String()
	}

	// 2. 检查震荡状态
	if afp.stabilityTracker.ShouldSuppressStateChange(peerID) {
		return true, "peer is in flapping state"
	}

	return false, ""
}

// ClearProtection 清除保护状态
//
// 用于管理员强制允许重新添加。
func (afp *AntiFalsePositive) ClearProtection(peerID string) {
	afp.protectionTracker.ClearProtection(peerID)
	afp.stabilityTracker.ResetPeer(peerID)

	afp.mu.Lock()
	delete(afp.memberStates, peerID)
	afp.mu.Unlock()
}

// ============================================================================
//                              状态查询
// ============================================================================

// IsInGracePeriod 检查成员是否在宽限期内
func (afp *AntiFalsePositive) IsInGracePeriod(peerID string) bool {
	afp.mu.RLock()
	state, exists := afp.memberStates[peerID]
	afp.mu.RUnlock()

	if !exists {
		return false
	}

	return state.IsInGracePeriod()
}

// IsFlapping 检查成员是否处于震荡状态
func (afp *AntiFalsePositive) IsFlapping(peerID string) bool {
	return afp.stabilityTracker.IsFlapping(peerID)
}

// IsProtected 检查成员是否在断开保护期内
func (afp *AntiFalsePositive) IsProtected(peerID string) bool {
	return afp.protectionTracker.IsProtected(peerID)
}

// GetState 获取成员连接状态
//
// 返回值：
//   - 如果成员存在状态机，返回当前状态
//   - 如果成员不存在状态机，返回 ConnStateConnected（默认假设已连接）
func (afp *AntiFalsePositive) GetState(peerID string) types.ConnState {
	afp.mu.RLock()
	state, exists := afp.memberStates[peerID]
	afp.mu.RUnlock()

	if !exists {
		// 如果没有状态机记录，默认假设已连接
		return types.ConnStateConnected
	}

	return state.GetState()
}

// GetStats 获取防误判统计信息
func (afp *AntiFalsePositive) GetStats() *AntiFalsePositiveStats {
	afp.mu.RLock()
	inGracePeriod := 0
	for _, state := range afp.memberStates {
		if state.IsInGracePeriod() {
			inGracePeriod++
		}
	}
	afp.mu.RUnlock()

	return &AntiFalsePositiveStats{
		InGracePeriod:  inGracePeriod,
		FlappingPeers:  afp.stabilityTracker.GetFlappingPeers(),
		ProtectedPeers: afp.protectionTracker.GetProtectedPeers(),
	}
}

// AntiFalsePositiveStats 防误判统计信息
type AntiFalsePositiveStats struct {
	// 在宽限期内的成员数
	InGracePeriod int
	// 震荡中的成员列表
	FlappingPeers []string
	// 保护期内的成员列表
	ProtectedPeers []string
}

// ============================================================================
//                              清理
// ============================================================================

// Cleanup 清理过期数据
func (afp *AntiFalsePositive) Cleanup() {
	afp.stabilityTracker.Cleanup()
	afp.protectionTracker.Cleanup()
}

// Close 关闭防误判机制
func (afp *AntiFalsePositive) Close() {
	afp.mu.Lock()
	defer afp.mu.Unlock()

	// 关闭所有状态机
	for _, state := range afp.memberStates {
		state.Close()
	}
	afp.memberStates = nil
}
