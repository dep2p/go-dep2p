// Package metrics 提供带宽和性能指标
package metrics

import (
	"sync"
	"time"
)

// ============================================================================
// RateMeter - 速率计算器
// ============================================================================

// RateMeter 速率计算器（基于滑动窗口）
//
// 使用 60 个 1 秒桶来计算最近 60 秒的平均速率。
type RateMeter struct {
	mu       sync.RWMutex
	buckets  [60]int64  // 60 个 1 秒桶
	lastIdx  int        // 最后写入的桶索引
	lastTime time.Time  // 最后更新时间
}

// NewRateMeter 创建速率计算器
func NewRateMeter() *RateMeter {
	return &RateMeter{
		lastTime: time.Now(),
	}
}

// Add 添加字节数到当前桶
func (r *RateMeter) Add(bytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastTime)

	// 如果超过 1 秒，移动到下一个桶
	if elapsed >= time.Second {
		seconds := int(elapsed.Seconds())
		if seconds >= 60 {
			// 清空所有桶（超过 60 秒没有数据）
			r.buckets = [60]int64{}
			r.lastIdx = 0
		} else {
			// 清空中间的桶
			for i := 0; i < seconds && i < 60; i++ {
				r.lastIdx = (r.lastIdx + 1) % 60
				r.buckets[r.lastIdx] = 0
			}
		}
		r.lastTime = now
	}

	// 添加到当前桶
	r.buckets[r.lastIdx] += bytes
}

// Rate 返回平均速率（字节/秒）
func (r *RateMeter) Rate() float64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 计算最近 60 秒的总量
	var total int64
	for _, v := range r.buckets {
		total += v
	}

	// 返回平均速率
	return float64(total) / 60.0
}

// Total 返回累计总量
func (r *RateMeter) Total() int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var total int64
	for _, v := range r.buckets {
		total += v
	}
	return total
}

// Reset 重置速率计算器
func (r *RateMeter) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.buckets = [60]int64{}
	r.lastIdx = 0
	r.lastTime = time.Now()
}

// LastUpdate 返回最后更新时间
func (r *RateMeter) LastUpdate() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastTime
}
