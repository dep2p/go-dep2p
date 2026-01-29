package routing

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              路由指标
// ============================================================================

// Metrics 路由指标
type Metrics struct {
	mu sync.RWMutex

	// 查询统计
	totalQueries   atomic.Int64
	successQueries atomic.Int64
	failedQueries  atomic.Int64

	// 缓存统计
	cacheHits   atomic.Int64
	cacheMisses atomic.Int64

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
//                              查询统计
// ============================================================================

// RecordQuery 记录查询
func (m *Metrics) RecordQuery() {
	m.totalQueries.Add(1)
}

// RecordSuccess 记录成功
func (m *Metrics) RecordSuccess() {
	m.successQueries.Add(1)
}

// RecordFailure 记录失败
func (m *Metrics) RecordFailure() {
	m.failedQueries.Add(1)
}

// ============================================================================
//                              缓存统计
// ============================================================================

// RecordCacheHit 记录缓存命中
func (m *Metrics) RecordCacheHit() {
	m.cacheHits.Add(1)
}

// RecordCacheMiss 记录缓存未命中
func (m *Metrics) RecordCacheMiss() {
	m.cacheMisses.Add(1)
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

// GetMetrics 获取路由指标
func (m *Metrics) GetMetrics() *interfaces.RoutingMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	avgLatency := time.Duration(0)
	if m.latencyCount > 0 {
		avgLatency = m.totalLatency / time.Duration(m.latencyCount)
	}

	return &interfaces.RoutingMetrics{
		TotalQueries:   m.totalQueries.Load(),
		SuccessQueries: m.successQueries.Load(),
		FailedQueries:  m.failedQueries.Load(),
		CacheHits:      m.cacheHits.Load(),
		CacheMisses:    m.cacheMisses.Load(),
		AvgLatency:     avgLatency,
	}
}

// Reset 重置指标
func (m *Metrics) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.totalQueries.Store(0)
	m.successQueries.Store(0)
	m.failedQueries.Store(0)
	m.cacheHits.Store(0)
	m.cacheMisses.Store(0)
	m.totalLatency = 0
	m.latencyCount = 0
	m.latencyHistory = make([]time.Duration, 0, 100)
}
