package relay

import (
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                     候选池测试
// ============================================================================

// TestCandidatePool_ConcurrentAccess 测试候选池并发安全
func TestCandidatePool_ConcurrentAccess(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")

	var wg sync.WaitGroup
	const goroutines = 100
	const iterations = 100

	// 并发添加
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				pool.Add(&RelayCandidate{
					PeerID:       string(rune('a' + id%26)),
					Reachability: 2, // ReachabilityPublic
				})
			}
		}(i)
	}

	// 并发读取
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = pool.Count()
				_ = pool.GetAll()
				_ = pool.SelectBest()
			}
		}()
	}

	// 并发删除
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				pool.Remove(string(rune('a' + id%26)))
			}
		}(i)
	}

	wg.Wait()
	// 测试通过 = 无 panic
}

// ============================================================================
//                     Config 测试
// ============================================================================

func TestConfig_DefaultValues(t *testing.T) {
	cfg := DefaultConfig()

	assert.True(t, cfg.EnableClient)
	assert.False(t, cfg.EnableServer)
	assert.Greater(t, cfg.MaxReservations, 0)
	assert.Greater(t, cfg.MaxCircuits, 0)
	assert.Greater(t, cfg.ReservationTTL, time.Duration(0))
	assert.Greater(t, cfg.BufferSize, 0)
}

func TestConfig_Validate_Extended(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"Valid_Default", func(c *Config) {}, false},
		{"MaxReservations_Zero", func(c *Config) { c.MaxReservations = 0 }, true},
		{"MaxReservations_Negative", func(c *Config) { c.MaxReservations = -1 }, true},
		// MaxCircuits = 0 表示不限制，是合法值
		{"MaxCircuits_Zero", func(c *Config) { c.MaxCircuits = 0 }, false},
		{"MaxCircuits_Negative", func(c *Config) { c.MaxCircuits = -1 }, true},
		{"ReservationTTL_TooShort", func(c *Config) { c.ReservationTTL = 30 * time.Second }, true},
		{"ReservationTTL_Boundary", func(c *Config) { c.ReservationTTL = 59 * time.Second }, true},
		{"ReservationTTL_ExactMinute", func(c *Config) { c.ReservationTTL = 60 * time.Second }, false},
		{"BufferSize_TooSmall", func(c *Config) { c.BufferSize = 1023 }, true},
		{"BufferSize_Boundary", func(c *Config) { c.BufferSize = 1024 }, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tt.mutate(cfg)
			err := cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// ============================================================================
//                     RelayDefaults 测试
// ============================================================================

func TestRelayDefaults_Values(t *testing.T) {
	defaults := GetRelayDefaults()

	assert.GreaterOrEqual(t, defaults.MaxBandwidth, int64(0))
	assert.GreaterOrEqual(t, defaults.MaxReservations, 0)
	assert.Greater(t, defaults.MaxCircuitsPerPeer, 0)
	assert.Greater(t, defaults.MaxCircuitsTotal, 0)
	assert.Greater(t, defaults.ReservationTTL, time.Duration(0))
	assert.Greater(t, defaults.AddressBookSize, 0)
	assert.Greater(t, defaults.AddressEntryTTL, time.Duration(0))
	assert.Greater(t, defaults.CleanupInterval, time.Duration(0))
	assert.Greater(t, defaults.BufferSize, 0)
	assert.Greater(t, defaults.ConnectTimeout, time.Duration(0))
	assert.Greater(t, defaults.IdleTimeout, time.Duration(0))
}

// ============================================================================
//
// ============================================================================

// TestServerLimiterAdapter_BUG18_ExpiredReservationAllowsReReserve 测试
//
// 多次预约时总是返回 false，错误地报告 "resource limit exceeded"
func TestServerLimiterAdapter_BUG18_ExpiredReservationAllowsReReserve(t *testing.T) {
	// 创建一个配置，使用短 TTL
	cfg := &Config{
		MaxReservations:    10,
		MaxCircuits:        128,
		ReservationTTL:     100 * time.Millisecond, // 100ms TTL 用于测试
		BufferSize:         4096,
		MaxCircuitsPerPeer: 16,
	}

	// 使用 newServerLimiterFromConfig 创建限流器
	limiter := newServerLimiterFromConfig(cfg)
	adapter := limiter.(*serverLimiterAdapter)

	peer := types.PeerID("test-peer-bug18")

	// 第一次预约应该成功
	result1 := adapter.CanReserve(peer)
	assert.True(t, result1, "第一次预约应该成功")

	// 立即再次预约应该成功（续期）
	result2 := adapter.CanReserve(peer)
	assert.True(t, result2, "未过期时重复预约应视为续期成功")

	// 等待预约过期
	time.Sleep(150 * time.Millisecond)

	// 过期后再次预约应该成功
	result3 := adapter.CanReserve(peer)
	assert.True(t, result3, "预约过期后应该允许重新预约")

	t.Log("✅ 预约过期后允许重新预约")
}

// TestServerLimiterAdapter_BUG18_ExpiredReservationsCleanedOnCount 测试过期预约在计数时被清理
func TestServerLimiterAdapter_BUG18_ExpiredReservationsCleanedOnCount(t *testing.T) {
	// 创建一个 TTL 很短且 MaxReservations 很小的配置
	cfg := &Config{
		MaxReservations:    2, // 只允许 2 个预约
		MaxCircuits:        128,
		ReservationTTL:     50 * time.Millisecond, // 50ms TTL
		BufferSize:         4096,
		MaxCircuitsPerPeer: 16,
	}

	limiter := newServerLimiterFromConfig(cfg)
	adapter := limiter.(*serverLimiterAdapter)

	// 预约两个节点
	assert.True(t, adapter.CanReserve(types.PeerID("peer1")), "peer1 预约应该成功")
	assert.True(t, adapter.CanReserve(types.PeerID("peer2")), "peer2 预约应该成功")

	// 第三个节点应该被拒绝（达到上限）
	assert.False(t, adapter.CanReserve(types.PeerID("peer3")), "peer3 应该被拒绝（达到上限）")

	// 等待预约过期
	time.Sleep(100 * time.Millisecond)

	// 现在 peer3 应该可以预约（过期的预约应该被清理）
	assert.True(t, adapter.CanReserve(types.PeerID("peer3")), "过期预约被清理后，新预约应该成功")

	t.Log("✅ 过期预约在计数时被自动清理")
}

// TestServerLimiterAdapter_BUG20_ReReserveDoesNotConsumeExtraSlots 测试重复预约不会占用额外配额
func TestServerLimiterAdapter_BUG20_ReReserveDoesNotConsumeExtraSlots(t *testing.T) {
	cfg := &Config{
		MaxReservations:    1, // 只允许 1 个预约
		MaxCircuits:        128,
		ReservationTTL:     100 * time.Millisecond,
		BufferSize:         4096,
		MaxCircuitsPerPeer: 16,
	}

	limiter := newServerLimiterFromConfig(cfg)
	adapter := limiter.(*serverLimiterAdapter)

	peer1 := types.PeerID("peer-bug20-1")
	peer2 := types.PeerID("peer-bug20-2")

	// 第一次预约成功
	assert.True(t, adapter.CanReserve(peer1), "peer1 第一次预约应该成功")
	// 续期应该成功，且不占用额外配额
	assert.True(t, adapter.CanReserve(peer1), "1 续期应该成功")
	// 因为配额仍为 1，peer2 应该被拒绝
	assert.False(t, adapter.CanReserve(peer2), "peer2 应该被拒绝（配额已被 peer1 占用）")

	// 等待 peer1 过期后，peer2 应该可以预约
	time.Sleep(150 * time.Millisecond)
	assert.True(t, adapter.CanReserve(peer2), "peer1 过期后，peer2 预约应成功")
}
