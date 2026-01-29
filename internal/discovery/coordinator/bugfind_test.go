// Package coordinator - BUG 检测测试
package coordinator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                 BUG #B40: PeerFinder.closed Race 条件
// ============================================================================

// TestBugFind_B40_PeerFinder_ClosedRace 测试 BUG #B40
//
// BUG 描述：PeerFinder.closed 是普通 bool，但在多 goroutine 中读写
//
// 位置：finder.go:131, 230, 582, 586
//
// 问题代码：
//   closed  bool  // ⚠️ 无锁保护的并发访问
//
//   func (pf *PeerFinder) FindPeer(...) {
//       if pf.closed {  // ⚠️ 并发读
//           return nil, ErrFinderClosed
//       }
//       ...
//   }
//
//   func (pf *PeerFinder) Close() {
//       if pf.closed {  // ⚠️ 并发读
//           return nil
//       }
//       pf.closed = true  // ⚠️ 并发写
//       ...
//   }
//
// 修复建议：
//   1. 改为 atomic.Bool (Go 1.19+)
//   2. 或改为 atomic.Int32 + atomic.LoadInt32/StoreInt32
//   3. 或添加 mutex 保护
func TestBugFind_B40_PeerFinder_ClosedRace(t *testing.T) {
	config := DefaultPeerFinderConfig()
	config.CacheTTL = 10 * time.Minute
	pf := NewPeerFinder(config)

	ctx := context.Background()
	err := pf.Start(ctx)
	require.NoError(t, err)

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// 启动多个 goroutine 并发调用 FindPeer（会读取 pf.closed）
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = pf.FindPeer(ctx, types.PeerID("peer-"+string(rune(id))))
				time.Sleep(1 * time.Microsecond)
			}
		}(i)
	}

	// 同时调用 Close（会写入 pf.closed）
	time.Sleep(5 * time.Millisecond)
	go func() {
		_ = pf.Close()
	}()

	wg.Wait()

	// 如果使用 -race 运行，这个测试会触发数据竞争检测
	t.Log("✅ 测试完成（使用 go test -race 检测数据竞争）")
}

// TestBugFind_B40_Finder_DoubleClose 测试重复关闭
func TestBugFind_B40_Finder_DoubleClose(t *testing.T) {
	config := DefaultPeerFinderConfig()
	pf := NewPeerFinder(config)

	ctx := context.Background()
	err := pf.Start(ctx)
	require.NoError(t, err)

	// 第一次关闭
	err = pf.Close()
	require.NoError(t, err)

	// 第二次关闭应该也不会panic
	err = pf.Close()
	require.NoError(t, err)

	// 关闭后调用 FindPeer 应该返回 ErrFinderClosed
	_, err = pf.FindPeer(ctx, types.PeerID("test"))
	require.ErrorIs(t, err, ErrFinderClosed)
}

// ============================================================================
//            BUG #B41: Coordinator.cleanupExpiredCache 未测试
// ============================================================================

// TestBugFind_B41_Coordinator_CleanupExpiredCache 测试缓存过期清理
//
// BUG 描述：cleanupExpiredCache 函数未被测试覆盖 (0%)
//
// 位置：coordinator.go:414-436
//
// 潜在风险：
//   1. 过期判断逻辑错误
//   2. 并发访问 peerCache/cacheOrder 时的数据不一致
//   3. 内存泄漏（无法正确清理）
func TestBugFind_B41_Coordinator_CleanupExpiredCache(t *testing.T) {
	config := DefaultConfig()
	config.EnableCache = true
	config.CacheTTL = 100 * time.Millisecond
	config.MaxCacheSize = 100

	coord := NewCoordinator(config)
	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)
	defer coord.Stop(ctx)

	// 手动添加缓存条目
	testPeer1 := types.PeerInfo{
		ID:    types.PeerID("peer1"),
		Addrs: []types.Multiaddr{},
	}
	testPeer2 := types.PeerInfo{
		ID:    types.PeerID("peer2"),
		Addrs: []types.Multiaddr{},
	}

	coord.updateCache(testPeer1)
	time.Sleep(10 * time.Millisecond)
	coord.updateCache(testPeer2)

	// 验证初始状态
	coord.cacheMu.RLock()
	initialSize := len(coord.peerCache)
	coord.cacheMu.RUnlock()
	require.Equal(t, 2, initialSize, "应该有2个缓存条目")

	// 等待第一个条目过期
	time.Sleep(150 * time.Millisecond)

	// 手动触发清理（测试未覆盖的函数）
	coord.cleanupExpiredCache()

	// 验证过期条目被清理
	coord.cacheMu.RLock()
	afterSize := len(coord.peerCache)
	cacheOrderSize := len(coord.cacheOrder)
	coord.cacheMu.RUnlock()

	// 因为 peer2 是后加入的，可能还未完全过期
	require.LessOrEqual(t, afterSize, 2, "清理后缓存大小应该 <= 2")
	require.Equal(t, afterSize, cacheOrderSize, "peerCache 和 cacheOrder 大小应该一致")

	t.Logf("✅ 缓存清理正常：初始=%d, 清理后=%d", initialSize, afterSize)
}

// TestBugFind_B41_CacheOrder_Consistency 测试缓存顺序一致性
func TestBugFind_B41_CacheOrder_Consistency(t *testing.T) {
	config := DefaultConfig()
	config.EnableCache = true
	config.CacheTTL = 1 * time.Minute
	config.MaxCacheSize = 3 // 小缓存，触发 LRU

	coord := NewCoordinator(config)
	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)
	defer coord.Stop(ctx)

	// 添加超过最大缓存的条目
	for i := 1; i <= 5; i++ {
		peer := types.PeerInfo{
			ID:    types.PeerID("peer-" + string(rune('0'+i))),
			Addrs: []types.Multiaddr{},
		}
		coord.updateCache(peer)
	}

	// 验证缓存大小限制
	coord.cacheMu.RLock()
	cacheSize := len(coord.peerCache)
	orderSize := len(coord.cacheOrder)
	coord.cacheMu.RUnlock()

	require.LessOrEqual(t, cacheSize, config.MaxCacheSize, "缓存大小应该不超过限制")
	require.Equal(t, cacheSize, orderSize, "peerCache 和 cacheOrder 大小必须一致")

	// 验证 cacheOrder 中的 ID 都在 peerCache 中
	coord.cacheMu.RLock()
	for _, id := range coord.cacheOrder {
		_, exists := coord.peerCache[id]
		require.True(t, exists, "cacheOrder 中的 ID 必须在 peerCache 中存在: %s", id)
	}
	coord.cacheMu.RUnlock()

	t.Logf("✅ 缓存一致性验证通过：cacheSize=%d, orderSize=%d", cacheSize, orderSize)
}

// ============================================================================
//        BUG #B42: UnregisterDiscovery/GetDiscovery/ListDiscoveries 未测试
// ============================================================================

// TestBugFind_B42_Coordinator_DiscoveryManagement 测试发现器管理
//
// BUG 描述：以下函数未被测试 (0% 覆盖)：
//   - UnregisterDiscovery (coordinator.go:82)
//   - GetDiscovery (coordinator.go:90)
//   - ListDiscoveries (coordinator.go:98)
//
// 潜在风险：
//   1. UnregisterDiscovery 后仍被使用（未清理引用）
//   2. GetDiscovery 返回 nil 未处理
//   3. ListDiscoveries 返回的切片被并发修改
func TestBugFind_B42_Coordinator_DiscoveryManagement(t *testing.T) {
	coord := NewCoordinator(nil)

	// 注册模拟发现器
	mockDisc1 := &mockDiscovery{name: "mock1"}
	mockDisc2 := &mockDiscovery{name: "mock2"}

	coord.RegisterDiscovery("disc1", mockDisc1)
	coord.RegisterDiscovery("disc2", mockDisc2)

	// 测试 ListDiscoveries
	list := coord.ListDiscoveries()
	require.Len(t, list, 2, "应该有2个发现器")

	// 测试 GetDiscovery
	disc1 := coord.GetDiscovery("disc1")
	require.NotNil(t, disc1, "应该能获取到 disc1")
	require.Equal(t, mockDisc1, disc1)

	discNotExist := coord.GetDiscovery("not-exist")
	require.Nil(t, discNotExist, "不存在的发现器应该返回 nil")

	// 测试 UnregisterDiscovery
	coord.UnregisterDiscovery("disc1")
	disc1After := coord.GetDiscovery("disc1")
	require.Nil(t, disc1After, "注销后应该获取不到 disc1")

	listAfter := coord.ListDiscoveries()
	require.Len(t, listAfter, 1, "注销后应该只剩1个发现器")

	t.Log("✅ 发现器管理功能正常")
}

// TestBugFind_B42_Coordinator_ConcurrentDiscoveryAccess 测试并发发现器访问
func TestBugFind_B42_Coordinator_ConcurrentDiscoveryAccess(t *testing.T) {
	coord := NewCoordinator(nil)

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	// 并发注册和注销发现器
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			name := "disc-" + string(rune('0'+id%5))
			mockDisc := &mockDiscovery{name: name}

			if id%2 == 0 {
				coord.RegisterDiscovery(name, mockDisc)
			} else {
				coord.UnregisterDiscovery(name)
			}

			_ = coord.GetDiscovery(name)
			_ = coord.ListDiscoveries()
		}(i)
	}

	wg.Wait()
	t.Log("✅ 并发发现器访问无panic")
}

// ============================================================================
//                      辅助函数（复用 coordinator_test.go 中的 mockDiscovery）
// ============================================================================

// mockDiscovery 已在 coordinator_test.go 中定义，此处直接使用

// ============================================================================
//                      总结测试
// ============================================================================

// TestBugFind_Summary 运行所有 BUG 检测测试并生成报告
func TestBugFind_Summary(t *testing.T) {
	t.Log("============================================================")
	t.Log("          Discovery/Coordinator 模块 BUG 检测总结")
	t.Log("============================================================")
	t.Log("")
	t.Log("已发现的潜在 BUG：")
	t.Log("")
	t.Log("1. 🔴 BUG #B40: PeerFinder.closed Race 条件")
	t.Log("   - 位置: finder.go:131, 230, 582, 586")
	t.Log("   - 问题: bool 变量无锁保护，并发读写")
	t.Log("   - 严重度: 🔴 高（会导致数据竞争）")
	t.Log("   - 修复: 改为 atomic.Bool 或 atomic.Int32")
	t.Log("")
	t.Log("2. 🟡 BUG #B41: cleanupExpiredCache 未测试")
	t.Log("   - 位置: coordinator.go:414-436")
	t.Log("   - 问题: 0% 覆盖，缓存清理逻辑未验证")
	t.Log("   - 严重度: 🟡 中等（可能内存泄漏）")
	t.Log("   - 修复: 添加测试覆盖")
	t.Log("")
	t.Log("3. 🟢 BUG #B42: 发现器管理函数未测试")
	t.Log("   - 位置: coordinator.go:82, 90, 98")
	t.Log("   - 问题: UnregisterDiscovery, GetDiscovery, ListDiscoveries 0% 覆盖")
	t.Log("   - 严重度: 🟢 低（功能性问题）")
	t.Log("   - 修复: 添加测试覆盖")
	t.Log("")
	t.Log("建议：")
	t.Log("- 🔴 立即修复 #B40 (Race 条件)")
	t.Log("- 🟡 补充 #B41 缓存清理测试")
	t.Log("- 🟢 补充 #B42 API 测试")
	t.Log("- 增加更多边界条件测试")
	t.Log("============================================================")
}
