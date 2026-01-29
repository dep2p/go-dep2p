package gateway

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              指标收集
// ============================================================================

// Metrics 网关指标
type Metrics struct {
	mu sync.RWMutex

	// 中继统计
	relayCount   atomic.Int64
	relaySuccess atomic.Int64
	relayFailed  atomic.Int64

	// 流量统计
	bytesTransferred atomic.Int64
	bytesSent        atomic.Int64
	bytesRecv        atomic.Int64

	// 连接统计
	activeConnections atomic.Int64

	// 延迟统计
	totalLatency   time.Duration
	latencyCount   int64
	latencyHistory []time.Duration
}

// NewMetrics 创建指标收集器
func NewMetrics() *Metrics {
	return &Metrics{
		latencyHistory: make([]time.Duration, 0, 100),
	}
}

// ============================================================================
//                              中继统计
// ============================================================================

// RecordRelay 记录中继请求
func (m *Metrics) RecordRelay() {
	m.relayCount.Add(1)
}

// RecordSuccess 记录成功
func (m *Metrics) RecordSuccess() {
	m.relaySuccess.Add(1)
}

// RecordFailure 记录失败
func (m *Metrics) RecordFailure() {
	m.relayFailed.Add(1)
}

// ============================================================================
//                              流量统计
// ============================================================================

// RecordBytes 记录字节传输
func (m *Metrics) RecordBytes(sent, recv int64) {
	m.bytesSent.Add(sent)
	m.bytesRecv.Add(recv)
	m.bytesTransferred.Add(sent + recv)
}

// ============================================================================
//                              连接统计
// ============================================================================

// SetActiveConnections 设置活跃连接数
func (m *Metrics) SetActiveConnections(count int) {
	m.activeConnections.Store(int64(count))
}

// ============================================================================
//                              延迟统计
// ============================================================================

// RecordLatency 记录延迟
func (m *Metrics) RecordLatency(latency time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalLatency += latency
	m.latencyCount++

	m.latencyHistory = append(m.latencyHistory, latency)

	// 保持最近 100 条记录
	if len(m.latencyHistory) > 100 {
		m.latencyHistory = m.latencyHistory[1:]
	}
}

// ============================================================================
//                              获取指标
// ============================================================================

// GetMetrics 获取网关指标
func (m *Metrics) GetMetrics() *interfaces.GatewayMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgLatency := time.Duration(0)
	if m.latencyCount > 0 {
		avgLatency = m.totalLatency / time.Duration(m.latencyCount)
	}

	return &interfaces.GatewayMetrics{
		RelayCount:        m.relayCount.Load(),
		RelaySuccess:      m.relaySuccess.Load(),
		RelayFailed:       m.relayFailed.Load(),
		BytesTransferred:  m.bytesTransferred.Load(),
		ActiveConnections: int(m.activeConnections.Load()),
		AvgLatency:        avgLatency,
		BandwidthUsage:    m.bytesSent.Load() + m.bytesRecv.Load(),
	}
}

// Reset 重置指标
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.relayCount.Store(0)
	m.relaySuccess.Store(0)
	m.relayFailed.Store(0)
	m.bytesTransferred.Store(0)
	m.bytesSent.Store(0)
	m.bytesRecv.Store(0)
	m.activeConnections.Store(0)
	m.totalLatency = 0
	m.latencyCount = 0
	m.latencyHistory = make([]time.Duration, 0, 100)
}
