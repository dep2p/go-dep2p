package connmgr

import (
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              测试辅助函数
// ============================================================================

// createTestSubnetLimiter 创建测试用子网限制器
func createTestSubnetLimiter() *SubnetLimiter {
	return NewSubnetLimiter(DefaultSubnetLimiterConfig())
}

// createStrictSubnetLimiter 创建严格限制的子网限制器
func createStrictSubnetLimiter() *SubnetLimiter {
	return NewSubnetLimiter(SubnetLimiterConfig{
		IPv4Limits: []SubnetLimit{
			{PrefixLength: 24, Limit: RateLimit{RPS: 1, Burst: 2}},
		},
		IPv6Limits: []SubnetLimit{
			{PrefixLength: 64, Limit: RateLimit{RPS: 1, Burst: 2}},
		},
		CleanupInterval: 1 * time.Second,
		BucketExpiry:    2 * time.Second,
	})
}

// ============================================================================
//                              构造函数测试
// ============================================================================

// TestNewSubnetLimiter 测试创建子网限制器
func TestNewSubnetLimiter(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		sl := createTestSubnetLimiter()
		require.NotNil(t, sl)
		defer sl.Close()

		stats := sl.Stats()
		assert.Equal(t, 2, stats.IPv4LimitsCount)
		assert.Equal(t, 2, stats.IPv6LimitsCount)
	})

	t.Run("custom config", func(t *testing.T) {
		sl := NewSubnetLimiter(SubnetLimiterConfig{
			IPv4Limits: []SubnetLimit{
				{PrefixLength: 16, Limit: RateLimit{RPS: 5, Burst: 10}},
			},
			IPv6Limits: []SubnetLimit{
				{PrefixLength: 48, Limit: RateLimit{RPS: 5, Burst: 10}},
			},
		})
		require.NotNil(t, sl)
		defer sl.Close()

		stats := sl.Stats()
		assert.Equal(t, 1, stats.IPv4LimitsCount)
		assert.Equal(t, 1, stats.IPv6LimitsCount)
	})

	t.Run("empty config", func(t *testing.T) {
		sl := NewSubnetLimiter(SubnetLimiterConfig{})
		require.NotNil(t, sl)
		defer sl.Close()

		stats := sl.Stats()
		assert.Equal(t, 0, stats.IPv4LimitsCount)
		assert.Equal(t, 0, stats.IPv6LimitsCount)
	})
}

// TestDefaultSubnetLimiterConfig 测试默认配置
func TestDefaultSubnetLimiterConfig(t *testing.T) {
	config := DefaultSubnetLimiterConfig()

	assert.Len(t, config.IPv4Limits, 2)
	assert.Len(t, config.IPv6Limits, 2)
	assert.Equal(t, 5*time.Minute, config.CleanupInterval)
	assert.Equal(t, 10*time.Minute, config.BucketExpiry)
}

// ============================================================================
//                              基本允许测试
// ============================================================================

// TestSubnetLimiter_Allow 测试基本允许功能
func TestSubnetLimiter_Allow(t *testing.T) {
	sl := createTestSubnetLimiter()
	defer sl.Close()

	t.Run("valid IPv4", func(t *testing.T) {
		addr := netip.MustParseAddr("192.168.1.1")
		allowed := sl.Allow(addr)
		assert.True(t, allowed)
	})

	t.Run("valid IPv6", func(t *testing.T) {
		addr := netip.MustParseAddr("2001:db8::1")
		allowed := sl.Allow(addr)
		assert.True(t, allowed)
	})

	t.Run("localhost", func(t *testing.T) {
		addr := netip.MustParseAddr("127.0.0.1")
		allowed := sl.Allow(addr)
		assert.True(t, allowed)
	})
}

// TestSubnetLimiter_AllowString 测试字符串地址允许
func TestSubnetLimiter_AllowString(t *testing.T) {
	sl := createTestSubnetLimiter()
	defer sl.Close()

	t.Run("valid address", func(t *testing.T) {
		allowed := sl.AllowString("10.0.0.1")
		assert.True(t, allowed)
	})

	t.Run("invalid address", func(t *testing.T) {
		// 无效地址默认允许
		allowed := sl.AllowString("invalid")
		assert.True(t, allowed)
	})

	t.Run("empty address", func(t *testing.T) {
		allowed := sl.AllowString("")
		assert.True(t, allowed)
	})
}

// ============================================================================
//                              速率限制测试
// ============================================================================

// TestSubnetLimiter_RateLimit 测试速率限制
func TestSubnetLimiter_RateLimit(t *testing.T) {
	sl := createStrictSubnetLimiter()
	defer sl.Close()

	// 同一子网的 IP
	addr1 := netip.MustParseAddr("192.168.1.1")
	addr2 := netip.MustParseAddr("192.168.1.2")

	// 第一次请求应该被允许
	assert.True(t, sl.Allow(addr1), "first request should be allowed")

	// 第二次请求应该被允许（burst = 2）
	assert.True(t, sl.Allow(addr2), "second request should be allowed")

	// 第三次请求应该被拒绝（超过 burst）
	assert.False(t, sl.Allow(addr1), "third request should be denied")

	// 等待令牌恢复
	time.Sleep(1100 * time.Millisecond)

	// 应该再次被允许
	assert.True(t, sl.Allow(addr1), "request after wait should be allowed")
}

// TestSubnetLimiter_DifferentSubnets 测试不同子网
func TestSubnetLimiter_DifferentSubnets(t *testing.T) {
	sl := createStrictSubnetLimiter()
	defer sl.Close()

	// 不同 /24 子网的 IP
	subnet1 := netip.MustParseAddr("192.168.1.1")
	subnet2 := netip.MustParseAddr("192.168.2.1")

	// 消耗 subnet1 的配额
	sl.Allow(subnet1)
	sl.Allow(subnet1)
	assert.False(t, sl.Allow(subnet1))

	// subnet2 应该不受影响
	assert.True(t, sl.Allow(subnet2))
	assert.True(t, sl.Allow(subnet2))
	assert.False(t, sl.Allow(subnet2))
}

// ============================================================================
//                              IPv4/IPv6 测试
// ============================================================================

// TestSubnetLimiter_IPv4 测试 IPv4 地址
func TestSubnetLimiter_IPv4(t *testing.T) {
	sl := createStrictSubnetLimiter()
	defer sl.Close()

	// 各种 IPv4 地址
	addrs := []string{
		"1.2.3.4",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.0.1",
	}

	for _, addr := range addrs {
		ip := netip.MustParseAddr(addr)
		allowed := sl.Allow(ip)
		assert.True(t, allowed, "IPv4 address %s should be initially allowed", addr)
	}
}

// TestSubnetLimiter_IPv6 测试 IPv6 地址
func TestSubnetLimiter_IPv6(t *testing.T) {
	sl := createStrictSubnetLimiter()
	defer sl.Close()

	// 各种 IPv6 地址
	addrs := []string{
		"2001:db8::1",
		"fe80::1",
		"::1",
		"fd00::1",
	}

	for _, addr := range addrs {
		ip := netip.MustParseAddr(addr)
		allowed := sl.Allow(ip)
		assert.True(t, allowed, "IPv6 address %s should be initially allowed", addr)
	}
}

// TestSubnetLimiter_IPv6_Subnet 测试 IPv6 子网限制
func TestSubnetLimiter_IPv6_Subnet(t *testing.T) {
	sl := createStrictSubnetLimiter()
	defer sl.Close()

	// 同一 /64 子网的地址
	addr1 := netip.MustParseAddr("2001:db8:1:2::1")
	addr2 := netip.MustParseAddr("2001:db8:1:2::2")

	// 消耗配额
	sl.Allow(addr1)
	sl.Allow(addr2)

	// 第三次应该被拒绝
	assert.False(t, sl.Allow(addr1))
}

// ============================================================================
//                              清理测试
// ============================================================================

// TestSubnetLimiter_Cleanup 测试清理功能
func TestSubnetLimiter_Cleanup(t *testing.T) {
	sl := NewSubnetLimiter(SubnetLimiterConfig{
		IPv4Limits: []SubnetLimit{
			{PrefixLength: 24, Limit: RateLimit{RPS: 10, Burst: 10}},
		},
		CleanupInterval: 100 * time.Millisecond,
		BucketExpiry:    200 * time.Millisecond,
	})
	defer sl.Close()

	// 创建一些桶
	sl.Allow(netip.MustParseAddr("1.1.1.1"))
	sl.Allow(netip.MustParseAddr("2.2.2.2"))
	sl.Allow(netip.MustParseAddr("3.3.3.3"))

	stats := sl.Stats()
	assert.Equal(t, 3, stats.ActiveBuckets)

	// 等待清理
	time.Sleep(500 * time.Millisecond)

	// 桶应该被清理
	stats = sl.Stats()
	assert.Equal(t, 0, stats.ActiveBuckets)
}

// TestSubnetLimiter_Close 测试关闭
func TestSubnetLimiter_Close(t *testing.T) {
	sl := createTestSubnetLimiter()

	// 使用后关闭
	sl.Allow(netip.MustParseAddr("1.1.1.1"))
	sl.Close()

	// 关闭后仍然可以调用（不会 panic）
	sl.Close() // 重复关闭
}

// ============================================================================
//                              统计信息测试
// ============================================================================

// TestSubnetLimiter_Stats 测试统计信息
func TestSubnetLimiter_Stats(t *testing.T) {
	sl := createTestSubnetLimiter()
	defer sl.Close()

	// 初始状态
	stats := sl.Stats()
	assert.Equal(t, 0, stats.ActiveBuckets)
	assert.Equal(t, 2, stats.IPv4LimitsCount)
	assert.Equal(t, 2, stats.IPv6LimitsCount)

	// 创建桶
	sl.Allow(netip.MustParseAddr("1.1.1.1"))
	sl.Allow(netip.MustParseAddr("2.2.2.2"))

	stats = sl.Stats()
	assert.Equal(t, 4, stats.ActiveBuckets) // 每个 IP 对应 2 个子网级别
}

// ============================================================================
//                              便捷函数测试
// ============================================================================

// TestSubnetLimiter_ParseAndCheck 测试解析并检查
func TestSubnetLimiter_ParseAndCheck(t *testing.T) {
	sl := createTestSubnetLimiter()
	defer sl.Close()

	t.Run("valid address", func(t *testing.T) {
		allowed, err := sl.ParseAndCheck("192.168.1.1")
		assert.NoError(t, err)
		assert.True(t, allowed)
	})

	t.Run("invalid address", func(t *testing.T) {
		_, err := sl.ParseAndCheck("invalid")
		assert.Error(t, err)
	})
}

// TestSubnetLimiter_AddLimit 测试动态添加限制
func TestSubnetLimiter_AddLimit(t *testing.T) {
	sl := NewSubnetLimiter(SubnetLimiterConfig{})
	defer sl.Close()

	// 初始无限制
	stats := sl.Stats()
	assert.Equal(t, 0, stats.IPv4LimitsCount)
	assert.Equal(t, 0, stats.IPv6LimitsCount)

	// 添加 IPv4 限制
	sl.AddIPv4Limit(24, 10, 50)
	stats = sl.Stats()
	assert.Equal(t, 1, stats.IPv4LimitsCount)

	// 添加 IPv6 限制
	sl.AddIPv6Limit(64, 10, 50)
	stats = sl.Stats()
	assert.Equal(t, 1, stats.IPv6LimitsCount)
}

// ============================================================================
//                              边界条件测试
// ============================================================================

// TestSubnetLimiter_NoLimits 测试无限制配置
func TestSubnetLimiter_NoLimits(t *testing.T) {
	sl := NewSubnetLimiter(SubnetLimiterConfig{
		IPv4Limits: nil,
		IPv6Limits: nil,
	})
	defer sl.Close()

	// 无限制时所有地址都应该被允许
	for i := 0; i < 100; i++ {
		assert.True(t, sl.Allow(netip.MustParseAddr("192.168.1.1")))
	}
}

// TestSubnetLimiter_MultipleLevels 测试多级子网限制
func TestSubnetLimiter_MultipleLevels(t *testing.T) {
	sl := NewSubnetLimiter(SubnetLimiterConfig{
		IPv4Limits: []SubnetLimit{
			{PrefixLength: 32, Limit: RateLimit{RPS: 1, Burst: 2}},  // 单 IP, burst=2
			{PrefixLength: 24, Limit: RateLimit{RPS: 5, Burst: 10}}, // /24, burst=10
		},
	})
	defer sl.Close()

	addr := netip.MustParseAddr("192.168.1.1")

	// 前两次允许（burst=2）
	assert.True(t, sl.Allow(addr), "first request should be allowed")
	assert.True(t, sl.Allow(addr), "second request should be allowed")

	// 第三次被 /32 限制拒绝
	assert.False(t, sl.Allow(addr), "third request should be denied by /32 limit")

	// 换一个同 /24 的 IP，应该被允许（不同 /32）
	addr2 := netip.MustParseAddr("192.168.1.2")
	assert.True(t, sl.Allow(addr2), "different /32 should be allowed")
}

// ============================================================================
//                              排序测试
// ============================================================================

// TestSubnetLimiter_LimitSorting 测试限制排序
func TestSubnetLimiter_LimitSorting(t *testing.T) {
	// 创建乱序的限制
	sl := NewSubnetLimiter(SubnetLimiterConfig{
		IPv4Limits: []SubnetLimit{
			{PrefixLength: 16, Limit: RateLimit{RPS: 10, Burst: 10}},
			{PrefixLength: 32, Limit: RateLimit{RPS: 1, Burst: 1}},
			{PrefixLength: 24, Limit: RateLimit{RPS: 5, Burst: 5}},
		},
	})
	defer sl.Close()

	// 验证排序（更具体的前缀在前）
	assert.Equal(t, 32, sl.ipv4Limits[0].PrefixLength)
	assert.Equal(t, 24, sl.ipv4Limits[1].PrefixLength)
	assert.Equal(t, 16, sl.ipv4Limits[2].PrefixLength)
}
