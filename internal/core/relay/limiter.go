package relay

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter 资源限制器接口
type Limiter interface {
	AllowReservation(peer string) error
	AllowCircuit(peer string) error
}

// RequestExpiry 请求记录过期时间
const RequestExpiry = 5 * time.Minute

// ════════════════════════════════════════════════════════════════════════════
// RelayLimiter - 统一中继限流器（v2.0）
// ════════════════════════════════════════════════════════════════════════════

// RelayLimiter 统一中继限流器
//
// v2.0 废弃双层限流器（SystemLimiter/RealmLimiter），
// 统一为单一可配置的 RelayLimiter。
type RelayLimiter struct {
	config RelayLimiterConfig

	bandwidth *rate.Limiter // 带宽限流

	mu       sync.Mutex
	circuits map[string]int       // peer -> active_circuits
	requests map[string]time.Time // peer -> last_request_time
	total    int                  // 总连接数
}

// RelayLimiterConfig 限流器配置
type RelayLimiterConfig struct {
	// MaxBandwidth 最大带宽（字节/秒，0 = 不限制）
	MaxBandwidth int64

	// MaxConnections 最大连接数（0 = 不限制）
	MaxConnections int

	// MaxConnectionsPerPeer 单节点最大连接数（0 = 不限制）
	MaxConnectionsPerPeer int

	// IdleTimeout 空闲超时（0 = 不超时）
	IdleTimeout time.Duration
}

// DefaultRelayLimiterConfig 返回默认配置（不限制）
func DefaultRelayLimiterConfig() RelayLimiterConfig {
	return RelayLimiterConfig{
		MaxBandwidth:          0, // 默认不限制
		MaxConnections:        0,
		MaxConnectionsPerPeer: 0,
		IdleTimeout:           0,
	}
}

// StrictRelayLimiterConfig 返回严格限制配置
//
// 用于公共 Relay 服务器，防止资源滥用。
func StrictRelayLimiterConfig() RelayLimiterConfig {
	return RelayLimiterConfig{
		MaxBandwidth:          10 * 1024, // 10KB/s
		MaxConnections:        128,
		MaxConnectionsPerPeer: 2,
		IdleTimeout:           60 * time.Second,
	}
}

// NewRelayLimiter 创建统一限流器
func NewRelayLimiter(config RelayLimiterConfig) *RelayLimiter {
	l := &RelayLimiter{
		config:   config,
		circuits: make(map[string]int),
		requests: make(map[string]time.Time),
	}

	// 如果配置了带宽限制，创建带宽限流器
	if config.MaxBandwidth > 0 {
		l.bandwidth = rate.NewLimiter(rate.Limit(config.MaxBandwidth), int(config.MaxBandwidth))
	}

	return l
}

// AllowReservation 检查是否允许预约
func (l *RelayLimiter) AllowReservation(peer string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 清理过期的请求记录
	l.cleanupExpiredRequestsLocked()

	// 带宽限流
	if l.bandwidth != nil && !l.bandwidth.Allow() {
		return ErrBandwidthExceeded
	}

	// 总连接数限制
	if l.config.MaxConnections > 0 && l.total >= l.config.MaxConnections {
		return ErrResourceLimitExceeded
	}

	// 单节点连接数限制
	if l.config.MaxConnectionsPerPeer > 0 && l.circuits[peer] >= l.config.MaxConnectionsPerPeer {
		return ErrTooManyCircuits
	}

	// 记录请求时间
	l.requests[peer] = time.Now()

	return nil
}

// AllowCircuit 检查是否允许新电路
func (l *RelayLimiter) AllowCircuit(peer string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 总连接数限制
	if l.config.MaxConnections > 0 && l.total >= l.config.MaxConnections {
		return ErrResourceLimitExceeded
	}

	// 单节点连接数限制
	if l.config.MaxConnectionsPerPeer > 0 && l.circuits[peer] >= l.config.MaxConnectionsPerPeer {
		return ErrTooManyCircuits
	}

	l.circuits[peer]++
	l.total++

	return nil
}

// ReleaseCircuit 释放电路
func (l *RelayLimiter) ReleaseCircuit(peer string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.circuits[peer] > 0 {
		l.circuits[peer]--
		l.total--

		if l.circuits[peer] == 0 {
			delete(l.circuits, peer)
		}
	}
}

// cleanupExpiredRequestsLocked 清理过期的请求记录（需要持有锁）
func (l *RelayLimiter) cleanupExpiredRequestsLocked() {
	now := time.Now()
	for peer, lastTime := range l.requests {
		if now.Sub(lastTime) > RequestExpiry {
			delete(l.requests, peer)
		}
	}
}

// CleanupExpiredRequests 清理过期的请求记录（公开方法）
func (l *RelayLimiter) CleanupExpiredRequests() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.cleanupExpiredRequestsLocked()
}

// Stats 返回限流器统计信息
func (l *RelayLimiter) Stats() LimiterStats {
	l.mu.Lock()
	defer l.mu.Unlock()

	return LimiterStats{
		TotalCircuits:   l.total,
		UniquePeers:     len(l.circuits),
		MaxConnections:  l.config.MaxConnections,
		MaxPerPeer:      l.config.MaxConnectionsPerPeer,
		BandwidthLimit:  l.config.MaxBandwidth,
	}
}

// LimiterStats 限流器统计
type LimiterStats struct {
	TotalCircuits   int
	UniquePeers     int
	MaxConnections  int
	MaxPerPeer      int
	BandwidthLimit  int64
}
