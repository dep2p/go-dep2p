package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              带宽限流器
// ============================================================================

// BandwidthLimiter 带宽限流器（Token Bucket 算法）
type BandwidthLimiter struct {
	mu sync.Mutex

	// Token Bucket 参数
	rate     int64 // 字节/秒
	capacity int64 // 突发容量
	tokens   int64 // 当前令牌数
	lastTime time.Time

	// 统计
	totalAcquired  atomic.Int64
	totalReleased  atomic.Int64
	throttledCount atomic.Int64
	totalWaitTime  atomic.Int64
}

// NewBandwidthLimiter 创建带宽限流器
func NewBandwidthLimiter(rate, capacity int64) *BandwidthLimiter {
	return &BandwidthLimiter{
		rate:     rate,
		capacity: capacity,
		tokens:   capacity,
		lastTime: time.Now(),
	}
}

// ============================================================================
//                              令牌管理
// ============================================================================

// Acquire 获取流量配额
func (bl *BandwidthLimiter) Acquire(ctx context.Context, bytes int64) (*interfaces.BandwidthToken, error) {
	startTime := time.Now()

	bl.mu.Lock()
	defer bl.mu.Unlock()

	// 更新令牌
	bl.refillTokens()

	// 等待令牌充足
	for bl.tokens < bytes {
		bl.mu.Unlock()

		// 等待一小段时间
		select {
		case <-time.After(10 * time.Millisecond):
		case <-ctx.Done():
			bl.throttledCount.Add(1)
			return nil, ctx.Err()
		}

		bl.mu.Lock()
		bl.refillTokens()
	}

	// 消耗令牌
	bl.tokens -= bytes
	bl.totalAcquired.Add(1)

	waitTime := time.Since(startTime)
	bl.totalWaitTime.Add(int64(waitTime))

	token := &interfaces.BandwidthToken{
		Bytes:     bytes,
		Timestamp: time.Now(),
	}

	return token, nil
}

// Release 释放配额
func (bl *BandwidthLimiter) Release(token *interfaces.BandwidthToken) {
	if token == nil {
		return
	}

	bl.mu.Lock()
	defer bl.mu.Unlock()

	// 归还令牌
	bl.tokens += token.Bytes
	if bl.tokens > bl.capacity {
		bl.tokens = bl.capacity
	}

	bl.totalReleased.Add(1)
}

// refillTokens 补充令牌
func (bl *BandwidthLimiter) refillTokens() {
	now := time.Now()
	elapsed := now.Sub(bl.lastTime)

	if elapsed <= 0 {
		return
	}

	// 计算新增令牌
	newTokens := int64(float64(bl.rate) * elapsed.Seconds())
	bl.tokens += newTokens

	if bl.tokens > bl.capacity {
		bl.tokens = bl.capacity
	}

	bl.lastTime = now
}

// ============================================================================
//                              速率调整
// ============================================================================

// UpdateRate 动态调整速率
func (bl *BandwidthLimiter) UpdateRate(bytesPerSec int64) {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	bl.rate = bytesPerSec
}

// ============================================================================
//                              统计
// ============================================================================

// GetStats 获取限流统计
func (bl *BandwidthLimiter) GetStats() *interfaces.LimiterStats {
	bl.mu.Lock()
	defer bl.mu.Unlock()

	totalAcquired := bl.totalAcquired.Load()
	avgWaitTime := time.Duration(0)
	if totalAcquired > 0 {
		avgWaitTime = time.Duration(bl.totalWaitTime.Load() / totalAcquired)
	}

	return &interfaces.LimiterStats{
		Rate:            bl.rate,
		Capacity:        bl.capacity,
		CurrentTokens:   bl.tokens,
		TotalAcquired:   totalAcquired,
		TotalReleased:   bl.totalReleased.Load(),
		ThrottledCount:  bl.throttledCount.Load(),
		AverageWaitTime: avgWaitTime,
	}
}

// ============================================================================
//                              关闭
// ============================================================================

// Close 关闭限流器
func (bl *BandwidthLimiter) Close() error {
	// 清理资源
	return nil
}

// 确保实现接口
var _ interfaces.BandwidthLimiter = (*BandwidthLimiter)(nil)
