package member

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              配置常量
// ============================================================================

const (
	// ReconnectGracePeriod 重连宽限期
	// 断开检测后，在此时间内重连不会触发成员移除
	ReconnectGracePeriod = 15 * time.Second

	// MaxGracePeriodExtensions 最大宽限期延长次数
	// 如果在宽限期内收到通信，可以延长宽限期
	MaxGracePeriodExtensions = 2
)

// ============================================================================
//                              成员连接状态机
// ============================================================================

// MemberConnectionState 成员连接状态
//
// 管理单个成员的连接状态转换，实现重连宽限期机制。
// 防止网络切换（如 4G→WiFi）导致的误判断开。
//
// 状态转换：
//   - Connected → Disconnecting → Disconnected
//   - Disconnecting → Connected（宽限期内重连）
//   - Disconnected → Connected（重新加入）
type MemberConnectionState struct {
	mu sync.RWMutex

	// 基础信息
	peerID  string
	realmID string

	// 状态
	state            types.ConnState
	lastStateChange  time.Time
	disconnectedAt   time.Time
	extensions       int // 宽限期延长次数
	graceTimer       *time.Timer
	graceTimerActive bool

	// 回调
	onGraceTimeout func(peerID string)
}

// NewMemberConnectionState 创建成员连接状态
func NewMemberConnectionState(peerID, realmID string) *MemberConnectionState {
	return &MemberConnectionState{
		peerID:          peerID,
		realmID:         realmID,
		state:           types.ConnStateConnected,
		lastStateChange: time.Now(),
	}
}

// SetOnGraceTimeout 设置宽限期超时回调
func (mcs *MemberConnectionState) SetOnGraceTimeout(callback func(peerID string)) {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()
	mcs.onGraceTimeout = callback
}

// GetState 获取当前状态
func (mcs *MemberConnectionState) GetState() types.ConnState {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()
	return mcs.state
}

// OnDisconnect 处理断开事件
//
// 启动宽限期定时器，在宽限期内重连可以恢复连接状态。
func (mcs *MemberConnectionState) OnDisconnect() {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	// 如果已经在断开状态，忽略
	if mcs.state == types.ConnStateDisconnected {
		return
	}

	now := time.Now()
	mcs.state = types.ConnStateDisconnecting
	mcs.lastStateChange = now
	mcs.disconnectedAt = now
	mcs.extensions = 0

	// 启动宽限期定时器
	mcs.startGraceTimer()

	logger.Debug("成员进入断开宽限期",
		"peerID", truncateID(mcs.peerID),
		"gracePeriod", ReconnectGracePeriod)
}

// OnReconnect 处理重连事件
//
// 在宽限期内重连时调用，取消宽限期定时器并恢复连接状态。
// 返回 true 表示成功恢复，false 表示宽限期已过或状态不对。
func (mcs *MemberConnectionState) OnReconnect() bool {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	// 如果不在断开中状态，忽略
	if mcs.state != types.ConnStateDisconnecting {
		// 如果是已断开状态，这是重新加入
		if mcs.state == types.ConnStateDisconnected {
			mcs.state = types.ConnStateConnected
			mcs.lastStateChange = time.Now()
			mcs.extensions = 0
			return true
		}
		return false
	}

	// 取消宽限期定时器
	mcs.cancelGraceTimer()

	// 恢复连接状态
	mcs.state = types.ConnStateConnected
	mcs.lastStateChange = time.Now()
	mcs.extensions = 0

	logger.Info("成员在宽限期内重连成功",
		"peerID", truncateID(mcs.peerID),
		"disconnectedDuration", time.Since(mcs.disconnectedAt))

	return true
}

// OnCommunication 处理通信事件
//
// 在宽限期内收到通信时调用，延长宽限期（最多延长 2 次）。
func (mcs *MemberConnectionState) OnCommunication() {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()

	// 如果在断开中状态且未达到最大延长次数
	if mcs.state == types.ConnStateDisconnecting && mcs.extensions < MaxGracePeriodExtensions {
		mcs.extensions++
		mcs.restartGraceTimer()

		logger.Debug("延长成员宽限期",
			"peerID", truncateID(mcs.peerID),
			"extensions", mcs.extensions,
			"maxExtensions", MaxGracePeriodExtensions)
	}
}

// startGraceTimer 启动宽限期定时器
func (mcs *MemberConnectionState) startGraceTimer() {
	mcs.cancelGraceTimer() // 先取消已有定时器

	mcs.graceTimer = time.AfterFunc(ReconnectGracePeriod, func() {
		mcs.onGraceTimeoutInternal()
	})
	mcs.graceTimerActive = true
}

// restartGraceTimer 重启宽限期定时器
func (mcs *MemberConnectionState) restartGraceTimer() {
	mcs.cancelGraceTimer()
	mcs.startGraceTimer()
}

// cancelGraceTimer 取消宽限期定时器
func (mcs *MemberConnectionState) cancelGraceTimer() {
	if mcs.graceTimer != nil {
		mcs.graceTimer.Stop()
		mcs.graceTimer = nil
	}
	mcs.graceTimerActive = false
}

// onGraceTimeoutInternal 宽限期超时处理（内部）
func (mcs *MemberConnectionState) onGraceTimeoutInternal() {
	mcs.mu.Lock()

	// 双重检查状态
	if mcs.state != types.ConnStateDisconnecting {
		mcs.mu.Unlock()
		return
	}

	// 转换为断开状态
	mcs.state = types.ConnStateDisconnected
	mcs.lastStateChange = time.Now()
	mcs.graceTimerActive = false

	callback := mcs.onGraceTimeout
	peerID := mcs.peerID

	mcs.mu.Unlock()

	logger.Info("成员宽限期超时，标记为断开",
		"peerID", truncateID(peerID),
		"totalDisconnectedDuration", time.Since(mcs.disconnectedAt))

	// 调用回调
	if callback != nil {
		callback(peerID)
	}
}

// IsInGracePeriod 检查是否在宽限期内
func (mcs *MemberConnectionState) IsInGracePeriod() bool {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()
	return mcs.state == types.ConnStateDisconnecting && mcs.graceTimerActive
}

// GetDisconnectedDuration 获取断开持续时间
func (mcs *MemberConnectionState) GetDisconnectedDuration() time.Duration {
	mcs.mu.RLock()
	defer mcs.mu.RUnlock()

	if mcs.state == types.ConnStateConnected {
		return 0
	}
	return time.Since(mcs.disconnectedAt)
}

// Close 关闭状态机
func (mcs *MemberConnectionState) Close() {
	mcs.mu.Lock()
	defer mcs.mu.Unlock()
	mcs.cancelGraceTimer()
}
