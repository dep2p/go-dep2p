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

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// logger 在 manager.go 中定义

// ============================================================================
//                              JitterTolerance 实现
// ============================================================================

// JitterTolerance 连接抖动容错器
type JitterTolerance struct {
	config pkgif.JitterConfig

	// 断连节点状态
	disconnectedPeers map[string]*disconnectedPeerState
	mu                sync.RWMutex

	// 重连回调
	reconnectCallback pkgif.ReconnectCallback
	// 状态变更回调
	onStateChange pkgif.StateChangeCallback
	callbackMu    sync.RWMutex

	// 停止通道和状态
	stopCh  chan struct{}
	stopped int32 // 原子变量，防止重复关闭
}

// disconnectedPeerState 断连节点状态
type disconnectedPeerState struct {
	PeerID            string
	DisconnectedAt    time.Time
	ReconnectAttempts int
	NextReconnectAt   time.Time
	State             pkgif.JitterState
	LastError         error
}

// NewJitterTolerance 创建抖动容错器
func NewJitterTolerance(config pkgif.JitterConfig) *JitterTolerance {
	validateJitterConfig(&config)

	return &JitterTolerance{
		config:            config,
		disconnectedPeers: make(map[string]*disconnectedPeerState),
		stopCh:            make(chan struct{}),
	}
}

// validateJitterConfig 验证并填充默认配置
func validateJitterConfig(c *pkgif.JitterConfig) {
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

// DefaultJitterConfig 返回默认抖动容错配置
func DefaultJitterConfig() pkgif.JitterConfig {
	return pkgif.JitterConfig{
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

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动抖动容错器
func (j *JitterTolerance) Start(ctx context.Context) error {
	if !j.config.Enabled {
		logger.Info("抖动容错已禁用")
		return nil
	}

	// 启动监控循环
	go j.monitorLoop(ctx)

	logger.Info("抖动容错器已启动",
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
	logger.Info("抖动容错器已停止")
	return nil
}

// ============================================================================
//                              回调设置
// ============================================================================

// SetReconnectCallback 设置重连回调（线程安全）
func (j *JitterTolerance) SetReconnectCallback(callback pkgif.ReconnectCallback) {
	j.callbackMu.Lock()
	j.reconnectCallback = callback
	j.callbackMu.Unlock()
}

// SetStateChangeCallback 设置状态变更回调（线程安全）
func (j *JitterTolerance) SetStateChangeCallback(callback pkgif.StateChangeCallback) {
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
func (j *JitterTolerance) NotifyDisconnected(peerID string) bool {
	if !j.config.Enabled {
		return true // 未启用，直接移除
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	now := time.Now()

	// 检查是否已经在断连状态
	if state, ok := j.disconnectedPeers[peerID]; ok {
		// 更新断连时间
		state.DisconnectedAt = now
		state.State = pkgif.StateDisconnected
		logger.Debug("节点再次断连",
			"peerID", peerID,
			"attempts", state.ReconnectAttempts)
		return false
	}

	// 新断连节点
	j.disconnectedPeers[peerID] = &disconnectedPeerState{
		PeerID:            peerID,
		DisconnectedAt:    now,
		ReconnectAttempts: 0,
		NextReconnectAt:   now.Add(j.config.InitialReconnectDelay), // 使用初始重连延迟
		State:             pkgif.StateDisconnected,
	}

	logger.Info("节点断连，进入抖动容错",
		"peerID", peerID,
		"window", j.config.ToleranceWindow)

	j.notifyStateChange(peerID, pkgif.StateDisconnected)

	return false
}

// NotifyReconnected 通知节点重连成功
func (j *JitterTolerance) NotifyReconnected(peerID string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if state, ok := j.disconnectedPeers[peerID]; ok {
		logger.Info("节点重连成功",
			"peerID", peerID,
			"attempts", state.ReconnectAttempts)
		delete(j.disconnectedPeers, peerID)
		j.notifyStateChange(peerID, pkgif.StateConnected)
	}
}

// ShouldRemove 检查是否应该移除节点
func (j *JitterTolerance) ShouldRemove(peerID string) bool {
	if !j.config.Enabled {
		return true
	}

	j.mu.RLock()
	defer j.mu.RUnlock()

	state, ok := j.disconnectedPeers[peerID]
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
func (j *JitterTolerance) GetState(peerID string) (pkgif.JitterState, bool) {
	j.mu.RLock()
	defer j.mu.RUnlock()

	state, ok := j.disconnectedPeers[peerID]
	if !ok {
		return pkgif.StateConnected, false
	}

	return state.State, true
}

// GetDisconnectedPeers 获取所有断连节点
func (j *JitterTolerance) GetDisconnectedPeers() []string {
	j.mu.RLock()
	defer j.mu.RUnlock()

	peers := make([]string, 0, len(j.disconnectedPeers))
	for peerID := range j.disconnectedPeers {
		peers = append(peers, peerID)
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
	toRemove := make([]string, 0)

	for peerID, state := range j.disconnectedPeers {
		// 检查是否超过状态保持时间
		if now.Sub(state.DisconnectedAt) > j.config.StateHoldTime {
			toRemove = append(toRemove, peerID)
			continue
		}

		// 检查是否超过最大重连次数
		if j.config.MaxReconnectAttempts > 0 && state.ReconnectAttempts >= j.config.MaxReconnectAttempts {
			toRemove = append(toRemove, peerID)
			continue
		}

		// 检查是否需要重连
		if j.config.ReconnectEnabled && now.After(state.NextReconnectAt) {
			go j.attemptReconnect(ctx, peerID)
		}
	}

	// 移除超时节点
	for _, peerID := range toRemove {
		delete(j.disconnectedPeers, peerID)
		logger.Info("节点状态保持超时，已移除",
			"peerID", peerID)
		j.notifyStateChange(peerID, pkgif.StateRemoved)
	}
}

// attemptReconnect 尝试重连
func (j *JitterTolerance) attemptReconnect(ctx context.Context, peerID string) {
	// 获取回调（线程安全）
	j.callbackMu.RLock()
	reconnectCallback := j.reconnectCallback
	j.callbackMu.RUnlock()

	if reconnectCallback == nil {
		return
	}

	j.mu.Lock()
	state, ok := j.disconnectedPeers[peerID]
	if !ok {
		j.mu.Unlock()
		return
	}

	// 更新状态
	state.State = pkgif.StateReconnecting
	state.ReconnectAttempts++
	j.mu.Unlock()

	logger.Debug("尝试重连",
		"peerID", peerID,
		"attempt", state.ReconnectAttempts)

	j.notifyStateChange(peerID, pkgif.StateReconnecting)

	// 执行重连
	err := reconnectCallback(ctx, peerID)

	j.mu.Lock()
	defer j.mu.Unlock()

	state, ok = j.disconnectedPeers[peerID]
	if !ok {
		return // 已经被移除或重连成功
	}

	if err != nil {
		// 重连失败
		state.LastError = err
		state.State = pkgif.StateHeld

		// 计算下次重连时间（指数退避）
		delay := j.calculateBackoff(state.ReconnectAttempts)
		state.NextReconnectAt = time.Now().Add(delay)

		logger.Debug("重连失败",
			"peerID", peerID,
			"attempt", state.ReconnectAttempts,
			"nextRetry", delay,
			"err", err)

		j.notifyStateChange(peerID, pkgif.StateHeld)
	}
	// 重连成功的情况会在 NotifyReconnected 中处理
}

// calculateBackoff 计算退避时间
func (j *JitterTolerance) calculateBackoff(attempt int) time.Duration {
	if attempt <= 0 {
		return j.config.InitialReconnectDelay
	}

	// 指数退避: delay = initial * multiplier^(attempt-1)
	backoff := float64(j.config.InitialReconnectDelay) * math.Pow(j.config.BackoffMultiplier, float64(attempt-1))

	// 限制最大值
	if backoff > float64(j.config.MaxReconnectDelay) {
		backoff = float64(j.config.MaxReconnectDelay)
	}

	return time.Duration(backoff)
}

// notifyStateChange 通知状态变更（线程安全）
func (j *JitterTolerance) notifyStateChange(peerID string, state pkgif.JitterState) {
	j.callbackMu.RLock()
	callback := j.onStateChange
	j.callbackMu.RUnlock()

	if callback != nil {
		go callback(peerID, state)
	}
}

// ============================================================================
//                              统计信息
// ============================================================================

// GetStats 返回统计信息
func (j *JitterTolerance) GetStats() pkgif.JitterStats {
	j.mu.RLock()
	defer j.mu.RUnlock()

	stats := pkgif.JitterStats{
		TotalDisconnected: len(j.disconnectedPeers),
	}

	for _, state := range j.disconnectedPeers {
		stats.TotalReconnectAttempts += state.ReconnectAttempts
		switch state.State {
		case pkgif.StateReconnecting:
			stats.Reconnecting++
		case pkgif.StateHeld:
			stats.Held++
		}
	}

	return stats
}
