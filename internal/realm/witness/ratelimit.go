package witness

import (
	"sync"
	"time"
)

// RateLimiter 见证人限速器
//
// 限制每分钟的见证报告数量，防止 DoS 攻击。
// 默认限制：每分钟最多 10 个报告。
type RateLimiter struct {
	mu sync.Mutex

	// 配置
	maxReports int           // 窗口内最大报告数
	window     time.Duration // 时间窗口

	// 状态
	reports map[string][]time.Time // peerID -> 报告时间列表
}

// NewRateLimiter 创建限速器
//
// 参数：
//   - maxReports: 时间窗口内允许的最大报告数
//   - window: 时间窗口长度
func NewRateLimiter(maxReports int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		maxReports: maxReports,
		window:     window,
		reports:    make(map[string][]time.Time),
	}
}

// AllowReport 检查是否允许发送报告
//
// 返回 true 表示允许，同时记录本次报告。
// 返回 false 表示已达到限速上限，应拒绝发送。
func (rl *RateLimiter) AllowReport(targetPeerID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// 获取该目标的报告历史
	reports := rl.reports[targetPeerID]

	// 清理过期记录
	validReports := make([]time.Time, 0, len(reports))
	for _, t := range reports {
		if t.After(windowStart) {
			validReports = append(validReports, t)
		}
	}

	// 检查是否超过限制
	if len(validReports) >= rl.maxReports {
		rl.reports[targetPeerID] = validReports
		return false
	}

	// 记录本次报告
	validReports = append(validReports, now)
	rl.reports[targetPeerID] = validReports

	return true
}

// AllowGlobalReport 检查全局报告限速
//
// 用于限制节点发送的总报告数，防止被滥用。
func (rl *RateLimiter) AllowGlobalReport() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	// 统计所有目标的报告总数
	totalReports := 0
	for peerID, reports := range rl.reports {
		validReports := make([]time.Time, 0, len(reports))
		for _, t := range reports {
			if t.After(windowStart) {
				validReports = append(validReports, t)
			}
		}
		rl.reports[peerID] = validReports
		totalReports += len(validReports)
	}

	// 全局限制为单目标限制的 3 倍
	globalLimit := rl.maxReports * 3
	return totalReports < globalLimit
}

// GetReportCount 获取指定目标的报告计数
func (rl *RateLimiter) GetReportCount(targetPeerID string) int {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	reports := rl.reports[targetPeerID]
	count := 0
	for _, t := range reports {
		if t.After(windowStart) {
			count++
		}
	}

	return count
}

// Reset 重置限速器状态
func (rl *RateLimiter) Reset() {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	rl.reports = make(map[string][]time.Time)
}

// Cleanup 清理过期记录
func (rl *RateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-rl.window)

	for peerID, reports := range rl.reports {
		validReports := make([]time.Time, 0, len(reports))
		for _, t := range reports {
			if t.After(windowStart) {
				validReports = append(validReports, t)
			}
		}

		if len(validReports) == 0 {
			delete(rl.reports, peerID)
		} else {
			rl.reports[peerID] = validReports
		}
	}
}
