// Package connmgr 提供连接管理功能
//
// SubnetLimiter 提供基于 IP 子网的速率限制：
//   - 防止 Sybil 攻击（同一子网的大量恶意节点）
//   - 支持 IPv4 和 IPv6 子网配置
//   - 使用令牌桶算法进行速率限制
package connmgr

import (
	"net/netip"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var subnetLogger = log.Logger("core/connmgr/subnet")

// ============================================================================
//                              速率限制配置
// ============================================================================

// RateLimit 速率限制配置
type RateLimit struct {
	// RPS 每秒请求数（稳态速率）
	RPS float64

	// Burst 突发容量（令牌桶大小）
	Burst int
}

// SubnetLimit 子网限制配置
type SubnetLimit struct {
	// PrefixLength 子网前缀长度
	// IPv4: 8, 16, 24, 32
	// IPv6: 32, 48, 64, 128
	PrefixLength int

	// Limit 速率限制
	Limit RateLimit
}

// ============================================================================
//                              SubnetLimiter 结构
// ============================================================================

// SubnetLimiter IP 子网速率限制器
//
// 用于限制来自同一子网的连接请求，防止 Sybil 攻击。
// 使用令牌桶算法，支持 IPv4 和 IPv6。
type SubnetLimiter struct {
	// IPv4 子网限制（按前缀长度从大到小排序）
	ipv4Limits []SubnetLimit

	// IPv6 子网限制（按前缀长度从大到小排序）
	ipv6Limits []SubnetLimit

	// 令牌桶存储：子网前缀 -> 令牌桶
	buckets sync.Map

	// 清理间隔
	cleanupInterval time.Duration

	// 桶过期时间
	bucketExpiry time.Duration

	// 关闭通道
	closeCh chan struct{}

	// 等待组
	wg sync.WaitGroup

	// 初始化标记
	initOnce sync.Once

	// 关闭标记
	closeOnce sync.Once
}

// tokenBucket 令牌桶
type tokenBucket struct {
	tokens     float64   // 当前令牌数
	lastUpdate time.Time // 上次更新时间
	rps        float64   // 每秒令牌数
	burst      int       // 桶容量
	mu         sync.Mutex
}

// SubnetLimiterConfig 子网限制器配置
type SubnetLimiterConfig struct {
	// IPv4Limits IPv4 子网限制
	IPv4Limits []SubnetLimit

	// IPv6Limits IPv6 子网限制
	IPv6Limits []SubnetLimit

	// CleanupInterval 清理间隔（默认 5 分钟）
	CleanupInterval time.Duration

	// BucketExpiry 桶过期时间（默认 10 分钟）
	BucketExpiry time.Duration
}

// DefaultSubnetLimiterConfig 返回默认配置
func DefaultSubnetLimiterConfig() SubnetLimiterConfig {
	return SubnetLimiterConfig{
		IPv4Limits: []SubnetLimit{
			{PrefixLength: 24, Limit: RateLimit{RPS: 10, Burst: 50}},   // /24 子网
			{PrefixLength: 16, Limit: RateLimit{RPS: 100, Burst: 500}}, // /16 子网
		},
		IPv6Limits: []SubnetLimit{
			{PrefixLength: 64, Limit: RateLimit{RPS: 10, Burst: 50}},   // /64 子网
			{PrefixLength: 48, Limit: RateLimit{RPS: 100, Burst: 500}}, // /48 子网
		},
		CleanupInterval: 5 * time.Minute,
		BucketExpiry:    10 * time.Minute,
	}
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewSubnetLimiter 创建子网限制器
func NewSubnetLimiter(config SubnetLimiterConfig) *SubnetLimiter {
	sl := &SubnetLimiter{
		ipv4Limits:      config.IPv4Limits,
		ipv6Limits:      config.IPv6Limits,
		cleanupInterval: config.CleanupInterval,
		bucketExpiry:    config.BucketExpiry,
		closeCh:         make(chan struct{}),
	}

	// 默认值
	if sl.cleanupInterval == 0 {
		sl.cleanupInterval = 5 * time.Minute
	}
	if sl.bucketExpiry == 0 {
		sl.bucketExpiry = 10 * time.Minute
	}

	// 按前缀长度降序排序（更具体的子网先匹配）
	sl.sortLimits()

	subnetLogger.Info("子网限制器已创建",
		"ipv4Limits", len(config.IPv4Limits),
		"ipv6Limits", len(config.IPv6Limits))

	return sl
}

// sortLimits 排序限制（前缀长度降序）
func (sl *SubnetLimiter) sortLimits() {
	// 简单的冒泡排序（列表通常很小）
	for i := 0; i < len(sl.ipv4Limits)-1; i++ {
		for j := 0; j < len(sl.ipv4Limits)-1-i; j++ {
			if sl.ipv4Limits[j].PrefixLength < sl.ipv4Limits[j+1].PrefixLength {
				sl.ipv4Limits[j], sl.ipv4Limits[j+1] = sl.ipv4Limits[j+1], sl.ipv4Limits[j]
			}
		}
	}
	for i := 0; i < len(sl.ipv6Limits)-1; i++ {
		for j := 0; j < len(sl.ipv6Limits)-1-i; j++ {
			if sl.ipv6Limits[j].PrefixLength < sl.ipv6Limits[j+1].PrefixLength {
				sl.ipv6Limits[j], sl.ipv6Limits[j+1] = sl.ipv6Limits[j+1], sl.ipv6Limits[j]
			}
		}
	}
}

// ============================================================================
//                              速率限制检查
// ============================================================================

// Allow 检查 IP 是否允许
//
// 对每个配置的子网级别进行检查，如果任何级别的限制被超过，
// 则返回 false。
//
// 参数：
//   - addr: IP 地址
//
// 返回：
//   - bool: 如果允许则返回 true
func (sl *SubnetLimiter) Allow(addr netip.Addr) bool {
	sl.initOnce.Do(sl.startCleanup)

	now := time.Now()

	// 选择对应的限制列表
	var limits []SubnetLimit
	if addr.Is4() {
		limits = sl.ipv4Limits
	} else if addr.Is6() {
		limits = sl.ipv6Limits
	} else {
		// 无效地址，允许
		return true
	}

	// 检查每个子网级别
	for _, limit := range limits {
		prefix, err := addr.Prefix(limit.PrefixLength)
		if err != nil {
			continue
		}

		// 获取或创建令牌桶
		bucket := sl.getOrCreateBucket(prefix, limit.Limit)

		// 尝试获取令牌
		if !bucket.take(now) {
			subnetLogger.Debug("子网速率限制",
				"addr", addr.String(),
				"prefix", prefix.String(),
				"prefixLen", limit.PrefixLength)
			return false
		}
	}

	return true
}

// AllowString 检查 IP 字符串是否允许
func (sl *SubnetLimiter) AllowString(addrStr string) bool {
	addr, err := netip.ParseAddr(addrStr)
	if err != nil {
		// 无法解析，允许
		return true
	}
	return sl.Allow(addr)
}

// ============================================================================
//                              令牌桶管理
// ============================================================================

// getOrCreateBucket 获取或创建令牌桶
func (sl *SubnetLimiter) getOrCreateBucket(prefix netip.Prefix, limit RateLimit) *tokenBucket {
	key := prefix.String()

	// 尝试获取现有桶
	if v, ok := sl.buckets.Load(key); ok {
		return v.(*tokenBucket)
	}

	// 创建新桶
	bucket := &tokenBucket{
		tokens:     float64(limit.Burst),
		lastUpdate: time.Now(),
		rps:        limit.RPS,
		burst:      limit.Burst,
	}

	// 存储（如果其他 goroutine 已创建，使用现有的）
	actual, _ := sl.buckets.LoadOrStore(key, bucket)
	return actual.(*tokenBucket)
}

// take 尝试从令牌桶获取一个令牌
func (b *tokenBucket) take(now time.Time) bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	// 补充令牌
	elapsed := now.Sub(b.lastUpdate).Seconds()
	b.tokens += elapsed * b.rps
	if b.tokens > float64(b.burst) {
		b.tokens = float64(b.burst)
	}
	b.lastUpdate = now

	// 检查是否有可用令牌
	if b.tokens < 1 {
		return false
	}

	// 消耗一个令牌
	b.tokens--
	return true
}

// ============================================================================
//                              清理
// ============================================================================

// startCleanup 启动清理协程
func (sl *SubnetLimiter) startCleanup() {
	sl.wg.Add(1)
	go sl.cleanupLoop()
}

// cleanupLoop 清理循环
func (sl *SubnetLimiter) cleanupLoop() {
	defer sl.wg.Done()

	ticker := time.NewTicker(sl.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sl.closeCh:
			return
		case <-ticker.C:
			sl.cleanup()
		}
	}
}

// cleanup 清理过期的令牌桶
func (sl *SubnetLimiter) cleanup() {
	now := time.Now()
	expiredCount := 0

	sl.buckets.Range(func(key, value interface{}) bool {
		bucket := value.(*tokenBucket)
		bucket.mu.Lock()
		expired := now.Sub(bucket.lastUpdate) > sl.bucketExpiry
		bucket.mu.Unlock()

		if expired {
			sl.buckets.Delete(key)
			expiredCount++
		}
		return true
	})

	if expiredCount > 0 {
		subnetLogger.Debug("清理过期令牌桶", "count", expiredCount)
	}
}

// Close 关闭限制器
func (sl *SubnetLimiter) Close() {
	sl.closeOnce.Do(func() {
		close(sl.closeCh)
		sl.wg.Wait()
	})
}

// ============================================================================
//                              统计信息
// ============================================================================

// SubnetLimiterStats 限制器统计
type SubnetLimiterStats struct {
	// ActiveBuckets 活跃的令牌桶数
	ActiveBuckets int

	// IPv4LimitsCount IPv4 限制配置数
	IPv4LimitsCount int

	// IPv6LimitsCount IPv6 限制配置数
	IPv6LimitsCount int
}

// Stats 返回统计信息
func (sl *SubnetLimiter) Stats() SubnetLimiterStats {
	count := 0
	sl.buckets.Range(func(_ interface{}, _ interface{}) bool {
		count++
		return true
	})

	return SubnetLimiterStats{
		ActiveBuckets:   count,
		IPv4LimitsCount: len(sl.ipv4Limits),
		IPv6LimitsCount: len(sl.ipv6Limits),
	}
}

// ============================================================================
//                              便捷函数
// ============================================================================

// ParseAndCheck 解析地址并检查是否允许
func (sl *SubnetLimiter) ParseAndCheck(addrStr string) (bool, error) {
	addr, err := netip.ParseAddr(addrStr)
	if err != nil {
		return false, err
	}
	return sl.Allow(addr), nil
}

// AddIPv4Limit 添加 IPv4 限制
func (sl *SubnetLimiter) AddIPv4Limit(prefixLength int, rps float64, burst int) {
	sl.ipv4Limits = append(sl.ipv4Limits, SubnetLimit{
		PrefixLength: prefixLength,
		Limit:        RateLimit{RPS: rps, Burst: burst},
	})
	sl.sortLimits()
}

// AddIPv6Limit 添加 IPv6 限制
func (sl *SubnetLimiter) AddIPv6Limit(prefixLength int, rps float64, burst int) {
	sl.ipv6Limits = append(sl.ipv6Limits, SubnetLimit{
		PrefixLength: prefixLength,
		Limit:        RateLimit{RPS: rps, Burst: burst},
	})
	sl.sortLimits()
}
