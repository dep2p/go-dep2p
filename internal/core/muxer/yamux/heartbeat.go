// Package yamux 提供基于 yamux 的多路复用实现
//
// 心跳监控：
// - 周期性心跳检测
// - 超时处理
// - 与 Liveness 集成
package yamux

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
)

// 包级别日志实例
var log = logger.Logger("muxer.yamux.heartbeat")

// ============================================================================
//                              HeartbeatConfig 配置
// ============================================================================

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
	// Enabled 是否启用心跳监控
	Enabled bool

	// Interval 心跳间隔
	Interval time.Duration

	// Timeout 心跳超时
	Timeout time.Duration

	// MaxMissed 最大允许丢失心跳次数
	MaxMissed int

	// OnTimeout 超时回调
	OnTimeout func(connID string)

	// OnHeartbeat 心跳回调
	OnHeartbeat func(connID string, latency time.Duration)
}

// DefaultHeartbeatConfig 返回默认心跳配置
func DefaultHeartbeatConfig() HeartbeatConfig {
	return HeartbeatConfig{
		Enabled:   true,
		Interval:  30 * time.Second,
		Timeout:   10 * time.Second,
		MaxMissed: 3,
	}
}

// Validate 验证配置
func (c *HeartbeatConfig) Validate() {
	if c.Interval <= 0 {
		c.Interval = 30 * time.Second
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	if c.MaxMissed <= 0 {
		c.MaxMissed = 3
	}
}

// ============================================================================
//                              HeartbeatMonitor 实现
// ============================================================================

// HeartbeatMonitor 心跳监控器
type HeartbeatMonitor struct {
	config HeartbeatConfig

	// 被监控的连接
	connections   map[string]*connHeartbeatState
	connectionsMu sync.RWMutex

	// 状态
	running  int32
	stopCh   chan struct{}
	stopOnce sync.Once // 防止重复关闭 stopCh
}

// connHeartbeatState 连接心跳状态
type connHeartbeatState struct {
	connID       string
	lastSeen     time.Time
	missedCount  int
	lastLatency  time.Duration
	pingInFlight bool
	pingSentAt   time.Time
}

// NewHeartbeatMonitor 创建心跳监控器
func NewHeartbeatMonitor(config HeartbeatConfig) *HeartbeatMonitor {
	config.Validate()

	return &HeartbeatMonitor{
		config:      config,
		connections: make(map[string]*connHeartbeatState),
		stopCh:      make(chan struct{}),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动心跳监控
func (m *HeartbeatMonitor) Start(ctx context.Context) error {
	if !m.config.Enabled {
		log.Info("心跳监控已禁用")
		return nil
	}

	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return nil
	}

	go m.monitorLoop(ctx)

	log.Info("心跳监控已启动",
		"interval", m.config.Interval,
		"timeout", m.config.Timeout,
		"maxMissed", m.config.MaxMissed)

	return nil
}

// Stop 停止心跳监控
func (m *HeartbeatMonitor) Stop() error {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return nil
	}

	m.stopOnce.Do(func() {
		close(m.stopCh)
	})
	log.Info("心跳监控已停止")
	return nil
}

// ============================================================================
//                              连接管理
// ============================================================================

// AddConnection 添加被监控的连接
func (m *HeartbeatMonitor) AddConnection(connID string) {
	m.connectionsMu.Lock()
	defer m.connectionsMu.Unlock()

	m.connections[connID] = &connHeartbeatState{
		connID:      connID,
		lastSeen:    time.Now(),
		missedCount: 0,
	}

	log.Debug("添加心跳监控连接",
		"connID", connID)
}

// RemoveConnection 移除被监控的连接
func (m *HeartbeatMonitor) RemoveConnection(connID string) {
	m.connectionsMu.Lock()
	defer m.connectionsMu.Unlock()

	delete(m.connections, connID)

	log.Debug("移除心跳监控连接",
		"connID", connID)
}

// RecordHeartbeat 记录心跳响应
func (m *HeartbeatMonitor) RecordHeartbeat(connID string) {
	m.connectionsMu.Lock()
	defer m.connectionsMu.Unlock()

	state, ok := m.connections[connID]
	if !ok {
		return
	}

	now := time.Now()

	// 计算延迟
	if state.pingInFlight && !state.pingSentAt.IsZero() {
		state.lastLatency = now.Sub(state.pingSentAt)
		state.pingInFlight = false

		// 触发心跳回调
		if m.config.OnHeartbeat != nil {
			go m.config.OnHeartbeat(connID, state.lastLatency)
		}
	}

	state.lastSeen = now
	state.missedCount = 0
}

// RecordPingSent 记录发送心跳
func (m *HeartbeatMonitor) RecordPingSent(connID string) {
	m.connectionsMu.Lock()
	defer m.connectionsMu.Unlock()

	state, ok := m.connections[connID]
	if !ok {
		return
	}

	state.pingInFlight = true
	state.pingSentAt = time.Now()
}

// ============================================================================
//                              查询方法
// ============================================================================

// GetLatency 获取连接的最后心跳延迟
func (m *HeartbeatMonitor) GetLatency(connID string) (time.Duration, bool) {
	m.connectionsMu.RLock()
	defer m.connectionsMu.RUnlock()

	state, ok := m.connections[connID]
	if !ok {
		return 0, false
	}

	return state.lastLatency, true
}

// GetMissedCount 获取丢失心跳次数
func (m *HeartbeatMonitor) GetMissedCount(connID string) (int, bool) {
	m.connectionsMu.RLock()
	defer m.connectionsMu.RUnlock()

	state, ok := m.connections[connID]
	if !ok {
		return 0, false
	}

	return state.missedCount, true
}

// GetLastSeen 获取最后活跃时间
func (m *HeartbeatMonitor) GetLastSeen(connID string) (time.Time, bool) {
	m.connectionsMu.RLock()
	defer m.connectionsMu.RUnlock()

	state, ok := m.connections[connID]
	if !ok {
		return time.Time{}, false
	}

	return state.lastSeen, true
}

// ConnectionCount 返回监控的连接数
func (m *HeartbeatMonitor) ConnectionCount() int {
	m.connectionsMu.RLock()
	defer m.connectionsMu.RUnlock()
	return len(m.connections)
}

// ============================================================================
//                              内部方法
// ============================================================================

// monitorLoop 监控循环
func (m *HeartbeatMonitor) monitorLoop(ctx context.Context) {
	ticker := time.NewTicker(m.config.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			m.checkConnections()
		}
	}
}

// checkConnections 检查所有连接
func (m *HeartbeatMonitor) checkConnections() {
	m.connectionsMu.Lock()
	defer m.connectionsMu.Unlock()

	now := time.Now()
	timedOut := make([]string, 0)

	for connID, state := range m.connections {
		// 检查是否有未响应的心跳
		if state.pingInFlight && now.Sub(state.pingSentAt) > m.config.Timeout {
			state.missedCount++
			state.pingInFlight = false

			log.Debug("心跳超时",
				"connID", connID,
				"missedCount", state.missedCount)

			// 检查是否超过最大丢失次数
			if state.missedCount >= m.config.MaxMissed {
				timedOut = append(timedOut, connID)
			}
		}
	}

	// 触发超时回调
	for _, connID := range timedOut {
		log.Warn("连接心跳失败",
			"connID", connID,
			"maxMissed", m.config.MaxMissed)

		if m.config.OnTimeout != nil {
			go m.config.OnTimeout(connID)
		}
	}
}

// ============================================================================
//                              统计信息
// ============================================================================

// Stats 返回统计信息
func (m *HeartbeatMonitor) Stats() HeartbeatStats {
	m.connectionsMu.RLock()
	defer m.connectionsMu.RUnlock()

	stats := HeartbeatStats{
		TotalConnections: len(m.connections),
	}

	var totalLatency time.Duration
	latencyCount := 0

	for _, state := range m.connections {
		if state.lastLatency > 0 {
			totalLatency += state.lastLatency
			latencyCount++

			if state.lastLatency > stats.MaxLatency {
				stats.MaxLatency = state.lastLatency
			}
			if stats.MinLatency == 0 || state.lastLatency < stats.MinLatency {
				stats.MinLatency = state.lastLatency
			}
		}

		if state.missedCount > 0 {
			stats.ConnectionsWithMissed++
		}
	}

	if latencyCount > 0 {
		stats.AvgLatency = totalLatency / time.Duration(latencyCount)
	}

	return stats
}

// HeartbeatStats 心跳统计
type HeartbeatStats struct {
	TotalConnections      int
	ConnectionsWithMissed int
	AvgLatency            time.Duration
	MaxLatency            time.Duration
	MinLatency            time.Duration
}

