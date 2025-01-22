package mocknet

import (
	"sync"
	"time"
)

// RateLimiter 用于链路根据带宽限制确定发送数据前需要等待的时间
type RateLimiter struct {
	lock         sync.Mutex    // 互斥锁
	bandwidth    float64       // 带宽(字节/纳秒)
	allowance    float64       // 当前可用流量(字节)
	maxAllowance float64       // 最大可用流量(字节)
	lastUpdate   time.Time     // 上次更新流量的时间
	count        int           // 应用速率限制的次数
	duration     time.Duration // 由于速率限制引入的总延迟
}

// NewRateLimiter 创建一个新的速率限制器
// 参数:
//   - bandwidth: float64 带宽(字节/秒)
//
// 返回值:
//   - *RateLimiter: 新创建的速率限制器
func NewRateLimiter(bandwidth float64) *RateLimiter {
	// 将带宽转换为字节/纳秒
	b := bandwidth / float64(time.Second)
	return &RateLimiter{
		bandwidth:    b,
		allowance:    0,
		maxAllowance: bandwidth,
		lastUpdate:   time.Now(),
	}
}

// UpdateBandwidth 更新速率限制器的带宽并重置可用流量
// 参数:
//   - bandwidth: float64 新的带宽(字节/秒)
func (r *RateLimiter) UpdateBandwidth(bandwidth float64) {
	r.lock.Lock()
	defer r.lock.Unlock()
	// 将带宽从字节/秒转换为字节/纳秒
	b := bandwidth / float64(time.Second)
	r.bandwidth = b
	// 重置可用流量
	r.allowance = 0
	r.maxAllowance = bandwidth
	r.lastUpdate = time.Now()
}

// Limit 计算发送指定大小数据前需要等待的时间
// 参数:
//   - dataSize: int 要发送的数据大小(字节)
//
// 返回值:
//   - time.Duration: 需要等待的时间
func (r *RateLimiter) Limit(dataSize int) time.Duration {
	r.lock.Lock()
	defer r.lock.Unlock()
	// 初始化等待时间
	var duration time.Duration = time.Duration(0)
	if r.bandwidth == 0 {
		return duration
	}
	// 更新时间
	current := time.Now()
	elapsedTime := current.Sub(r.lastUpdate)
	r.lastUpdate = current

	// 计算当前可用流量
	allowance := r.allowance + float64(elapsedTime)*r.bandwidth
	// 可用流量不能超过最大限制
	if allowance > r.maxAllowance {
		allowance = r.maxAllowance
	}

	// 减去要发送的数据大小
	allowance -= float64(dataSize)
	if allowance < 0 {
		// 计算需要等待的时间直到可用流量恢复到0
		duration = time.Duration(-allowance / r.bandwidth)
		// 记录速率限制统计信息
		r.count++
		r.duration += duration
	}

	r.allowance = allowance
	return duration
}
