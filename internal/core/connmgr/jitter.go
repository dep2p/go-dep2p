// Package connmgr 提供连接管理模块的实现
//
// 连接抖动容错：
// - 短暂断连不立即移除节点
// - 状态保持窗口
// - 指数退避重连
package connmgr

import (
	"context"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              JitterTolerance 配置
// ============================================================================

// JitterConfig 抖动容错配置
type JitterConfig struct {
	// Enabled 是否启用抖动容错
	Enabled bool

	// ToleranceWindow 容错窗口（在此期间不移除断连节点）
	ToleranceWindow time.Duration

	// StateHoldTime 状态保持时间（断连后保持节点状态的时间）
	StateHoldTime time.Duration

	// ReconnectEnabled 是否启用自动重连
	ReconnectEnabled bool

	// InitialReconnectDelay 初始重连延迟
	InitialReconnectDelay time.Duration

	// MaxReconnectDelay 最大重连延迟
	MaxReconnectDelay time.Duration

	// MaxReconnectAttempts 最大重连次数（0 表示无限）
	MaxReconnectAttempts int

	// BackoffMultiplier 退避乘数
	BackoffMultiplier float64
}

// DefaultJitterConfig 返回默认抖动容错配置
func DefaultJitterConfig() JitterConfig {
	return JitterConfig{
		Enabled:               true,
		ToleranceWindow:       5 * time.Second,
		StateHoldTime:         30 * time.Second,
		ReconnectEnabled:      true,
		InitialReconnectDelay: 1 * time.Second,
		MaxReconnectDelay:     60 * time.Second,
		MaxReconnectAttempts:  5,
		BackoffMultiplier:     2.0,
	}
}

// Validate 验证配置
func (c *JitterConfig) Validate() {
	if c.ToleranceWindow <= 0 {
		c.ToleranceWindow = 5 * time.Second
	}
	if c.StateHoldTime <= 0 {
		c.StateHoldTime = 30 * time.Second
	}
	if c.InitialReconnectDelay <= 0 {
		c.InitialReconnectDelay = 1 * time.Second
	}
	if c.MaxReconnectDelay <= 0 {
		c.MaxReconnectDelay = 60 * time.Second
	}
	if c.BackoffMultiplier <= 1 {
		c.BackoffMultiplier = 2.0
	}
}

// ============================================================================
//                              JitterTolerance 实现
// ============================================================================

// JitterTolerance 连接抖动容错器
type JitterTolerance struct {
	config JitterConfig

	// 断连节点状态
	disconnectedPeers map[types.NodeID]*disconnectedPeerState
	mu                sync.RWMutex

	// 重连回调
	reconnectCallback func(ctx context.Context, nodeID types.NodeID) error
	// 状态变更回调
	onStateChange func(nodeID types.NodeID, state PeerJitterState)
	callbackMu    sync.RWMutex

	// 停止通道和状态
	stopCh  chan struct{}
	stopped int32 // 原子变量，防止重复关闭
}

// disconnectedPeerState 断连节点状态
type disconnectedPeerState struct {
	NodeID            types.NodeID
	DisconnectedAt    time.Time
	ReconnectAttempts int
	NextReconnectAt   time.Time
	State             PeerJitterState
	LastError         error
}

// PeerJitterState 节点抖动状态
type PeerJitterState int

const (
	// StateConnected 已连接
	StateConnected PeerJitterState = iota
	// StateDisconnected 已断连（在容错窗口内）
	StateDisconnected
	// StateReconnecting 正在重连
	StateReconnecting
	// StateHeld 状态保持中
	StateHeld
	// StateRemoved 已移除
	StateRemoved
)

// String 返回状态字符串
func (s PeerJitterState) String() string {
	switch s {
	case StateConnected:
		return "connected"
	case StateDisconnected:
		return "disconnected"
	case StateReconnecting:
		return "reconnecting"
	case StateHeld:
		return "held"
	case StateRemoved:
		return "removed"
	default:
		return "unknown"
	}
}

// NewJitterTolerance 创建抖动容错器
func NewJitterTolerance(config JitterConfig) *JitterTolerance {
	config.Validate()

	return &JitterTolerance{
		config:            config,
		disconnectedPeers: make(map[types.NodeID]*disconnectedPeerState),
		stopCh:            make(chan struct{}),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动抖动容错器
func (j *JitterTolerance) Start(ctx context.Context) error {
	if !j.config.Enabled {
		log.Info("抖动容错已禁用")
		return nil
	}

	// 启动监控循环
	go j.monitorLoop(ctx)

	log.Info("抖动容错器已启动",
		"toleranceWindow", j.config.ToleranceWindow,
		"stateHoldTime", j.config.StateHoldTime,
		"reconnectEnabled", j.config.ReconnectEnabled)

	return nil
}

// Stop 停止抖动容错器（安全支持重复调用）
func (j *JitterTolerance) Stop() error {
	// 使用 atomic 保证只关闭一次
	if !atomic.CompareAndSwapInt32(&j.stopped, 0, 1) {
		return nil // 已经停止
	}
	close(j.stopCh)
	log.Info("抖动容错器已停止")
	return nil
}

// ============================================================================
//                              回调设置
// ============================================================================

// SetReconnectCallback 设置重连回调（线程安全）
func (j *JitterTolerance) SetReconnectCallback(callback func(ctx context.Context, nodeID types.NodeID) error) {
	j.callbackMu.Lock()
	j.reconnectCallback = callback
	j.callbackMu.Unlock()
}

// SetStateChangeCallback 设置状态变更回调（线程安全）
func (j *JitterTolerance) SetStateChangeCallback(callback func(nodeID types.NodeID, state PeerJitterState)) {
	j.callbackMu.Lock()
	j.onStateChange = callback
	j.callbackMu.Unlock()
}

// ============================================================================
//                              断连处理
// ============================================================================

// NotifyDisconnected 通知节点断连
//
// 返回 true 表示应该移除节点，false 表示进入抖动容错
func (j *JitterTolerance) NotifyDisconnected(nodeID types.NodeID) bool {
	if !j.config.Enabled {
		return true // 未启用，直接移除
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()

	// 检查是否已经在断连状态
	if state, ok := j.disconnectedPeers[nodeID]; ok {
		// 更新断连时间
		state.DisconnectedAt = now
		state.State = StateDisconnected
		log.Debug("节点再次断连",
			"nodeID", nodeID.ShortString(),
			"attempts", state.ReconnectAttempts)
		return false
	}

	// 新断连节点
	j.disconnectedPeers[nodeID] = &disconnectedPeerState{
		NodeID:            nodeID,
		DisconnectedAt:    now,
		ReconnectAttempts: 0,
		NextReconnectAt:   now.Add(j.config.InitialReconnectDelay),
		State:             StateDisconnected,
	}

	log.Info("节点断连，进入抖动容错",
		"nodeID", nodeID.ShortString(),
		"window", j.config.ToleranceWindow)

	j.notifyStateChange(nodeID, StateDisconnected)

	return false
}

// NotifyReconnected 通知节点重连成功
func (j *JitterTolerance) NotifyReconnected(nodeID types.NodeID) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if state, ok := j.disconnectedPeers[nodeID]; ok {
		log.Info("节点重连成功",
			"nodeID", nodeID.ShortString(),
			"attempts", state.ReconnectAttempts)
		delete(j.disconnectedPeers, nodeID)
		j.notifyStateChange(nodeID, StateConnected)
	}
}

// ShouldRemove 检查是否应该移除节点
func (j *JitterTolerance) ShouldRemove(nodeID types.NodeID) bool {
	if !j.config.Enabled {
		return true
	}

	j.mu.RLock()
	defer j.mu.RUnlock()

	state, ok := j.disconnectedPeers[nodeID]
	if !ok {
		return false // 不在断连列表中
	}

	now := time.Now()

	// 检查是否超过状态保持时间
	if now.Sub(state.DisconnectedAt) > j.config.StateHoldTime {
		return true
	}

	// 检查是否超过最大重连次数
	if j.config.MaxReconnectAttempts > 0 && state.ReconnectAttempts >= j.config.MaxReconnectAttempts {
		return true
	}

	return false
}

// GetState 获取节点抖动状态
func (j *JitterTolerance) GetState(nodeID types.NodeID) (PeerJitterState, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	state, ok := j.disconnectedPeers[nodeID]
	if !ok {
		return StateConnected, false
	}

	return state.State, true
}

// GetDisconnectedPeers 获取所有断连节点
func (j *JitterTolerance) GetDisconnectedPeers() []types.NodeID {
	j.mu.RLock()
	defer j.mu.RUnlock()

	peers := make([]types.NodeID, 0, len(j.disconnectedPeers))
	for nodeID := range j.disconnectedPeers {
		peers = append(peers, nodeID)
	}
	return peers
}

// ============================================================================
//                              内部方法
// ============================================================================

// monitorLoop 监控循环
func (j *JitterTolerance) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-j.stopCh:
			return
		case <-ticker.C:
			j.processDisconnectedPeers(ctx)
		}
	}
}

// processDisconnectedPeers 处理断连节点
func (j *JitterTolerance) processDisconnectedPeers(ctx context.Context) {
	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()
	toRemove := make([]types.NodeID, 0)

	for nodeID, state := range j.disconnectedPeers {
		// 检查是否超过状态保持时间
		if now.Sub(state.DisconnectedAt) > j.config.StateHoldTime {
			toRemove = append(toRemove, nodeID)
			continue
		}

		// 检查是否超过最大重连次数
		if j.config.MaxReconnectAttempts > 0 && state.ReconnectAttempts >= j.config.MaxReconnectAttempts {
			toRemove = append(toRemove, nodeID)
			continue
		}

		// 检查是否需要重连
		if j.config.ReconnectEnabled && now.After(state.NextReconnectAt) {
			go j.attemptReconnect(ctx, nodeID)
		}
	}

	// 移除超时节点
	for _, nodeID := range toRemove {
		delete(j.disconnectedPeers, nodeID)
		log.Info("节点状态保持超时，已移除",
			"nodeID", nodeID.ShortString())
		j.notifyStateChange(nodeID, StateRemoved)
	}
}

// attemptReconnect 尝试重连
func (j *JitterTolerance) attemptReconnect(ctx context.Context, nodeID types.NodeID) {
	// 获取回调（线程安全）
	j.callbackMu.RLock()
	reconnectCallback := j.reconnectCallback
	j.callbackMu.RUnlock()

	if reconnectCallback == nil {
		return
	}

	j.mu.Lock()
	state, ok := j.disconnectedPeers[nodeID]
	if !ok {
		j.mu.Unlock()
		return
	}

	// 更新状态
	state.State = StateReconnecting
	state.ReconnectAttempts++
	j.mu.Unlock()

	log.Debug("尝试重连",
		"nodeID", nodeID.ShortString(),
		"attempt", state.ReconnectAttempts)

	j.notifyStateChange(nodeID, StateReconnecting)

	// 执行重连
	err := reconnectCallback(ctx, nodeID)

	j.mu.Lock()
	defer j.mu.Unlock()

	state, ok = j.disconnectedPeers[nodeID]
	if !ok {
		return // 已经被移除或重连成功
	}

	if err != nil {
		// 重连失败
		state.LastError = err
		state.State = StateHeld

		// 计算下次重连时间（指数退避）
		delay := j.calculateBackoff(state.ReconnectAttempts)
		state.NextReconnectAt = time.Now().Add(delay)

		log.Debug("重连失败",
			"nodeID", nodeID.ShortString(),
			"attempt", state.ReconnectAttempts,
			"nextRetry", delay,
			"err", err)

		j.notifyStateChange(nodeID, StateHeld)
	}
	// 重连成功的情况会在 NotifyReconnected 中处理
}

// calculateBackoff 计算退避时间
func (j *JitterTolerance) calculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return j.config.InitialReconnectDelay
	}

	// 指数退避
	backoff := float64(j.config.InitialReconnectDelay) * math.Pow(j.config.BackoffMultiplier, float64(attempt-1))

	// 限制最大值
	if backoff > float64(j.config.MaxReconnectDelay) {
		backoff = float64(j.config.MaxReconnectDelay)
	}

	return time.Duration(backoff)
}

// notifyStateChange 通知状态变更（线程安全）
func (j *JitterTolerance) notifyStateChange(nodeID types.NodeID, state PeerJitterState) {
	j.callbackMu.RLock()
	callback := j.onStateChange
	j.callbackMu.RUnlock()

	if callback != nil {
		go callback(nodeID, state)
	}
}

// ============================================================================
//                              统计信息
// ============================================================================

// Stats 返回统计信息
func (j *JitterTolerance) Stats() JitterStats {
	j.mu.RLock()
	defer j.mu.RUnlock()

	stats := JitterStats{
		TotalDisconnected: len(j.disconnectedPeers),
	}

	for _, state := range j.disconnectedPeers {
		stats.TotalReconnectAttempts += state.ReconnectAttempts
		switch state.State {
		case StateReconnecting:
			stats.Reconnecting++
		case StateHeld:
			stats.Held++
		}
	}

	return stats
}

// JitterStats 抖动统计
type JitterStats struct {
	TotalDisconnected      int
	Reconnecting           int
	Held                   int
	TotalReconnectAttempts int
}

