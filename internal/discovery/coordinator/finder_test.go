package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              配置测试
// ============================================================================

// TestDefaultPeerFinderConfig 测试默认配置
func TestDefaultPeerFinderConfig(t *testing.T) {
	config := DefaultPeerFinderConfig()

	assert.Equal(t, 10*time.Minute, config.CacheTTL)
	assert.Equal(t, 1000, config.MaxCacheSize)
	assert.Equal(t, 30*time.Second, config.NetworkTimeout)
	assert.True(t, config.EnableLocalPriority)
	assert.Equal(t, 5*time.Minute, config.CacheCleanupInterval)
}

// TestNewPeerFinder 测试创建查找器
func TestNewPeerFinder(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		pf := NewPeerFinder(DefaultPeerFinderConfig())
		require.NotNil(t, pf)
		defer pf.Close()

		assert.Equal(t, 10*time.Minute, pf.config.CacheTTL)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := PeerFinderConfig{
			CacheTTL:     5 * time.Minute,
			MaxCacheSize: 500,
		}
		pf := NewPeerFinder(config)
		require.NotNil(t, pf)
		defer pf.Close()

		assert.Equal(t, 5*time.Minute, pf.config.CacheTTL)
		assert.Equal(t, 500, pf.config.MaxCacheSize)
	})

	t.Run("with zero values uses defaults", func(t *testing.T) {
		config := PeerFinderConfig{}
		pf := NewPeerFinder(config)
		require.NotNil(t, pf)
		defer pf.Close()

		assert.Equal(t, DefaultPeerFinderConfig().CacheTTL, pf.config.CacheTTL)
	})
}

// ============================================================================
//                              缓存测试
// ============================================================================

// TestPeerFinder_Cache 测试缓存功能
func TestPeerFinder_Cache(t *testing.T) {
	pf := NewPeerFinder(DefaultPeerFinderConfig())
	defer pf.Close()

	peerID := types.PeerID("test-peer-1")
	addrs := []string{"/ip4/1.2.3.4/tcp/4001", "/ip4/5.6.7.8/tcp/4001"}

	// 缓存地址
	pf.cacheAddrs(peerID, addrs, "test")

	// 从缓存获取
	cached := pf.findInCache(peerID)
	assert.Equal(t, addrs, cached)

	// 检查缓存大小
	assert.Equal(t, 1, pf.CacheSize())

	// 获取缓存条目
	entry, ok := pf.GetCachedPeer(peerID)
	require.True(t, ok)
	assert.Equal(t, peerID, entry.PeerID)
	assert.Equal(t, "test", entry.Source)
}

// TestPeerFinder_CacheExpiry 测试缓存过期
func TestPeerFinder_CacheExpiry(t *testing.T) {
	config := PeerFinderConfig{
		CacheTTL: 100 * time.Millisecond,
	}
	pf := NewPeerFinder(config)
	defer pf.Close()

	peerID := types.PeerID("test-peer-1")
	addrs := []string{"/ip4/1.2.3.4/tcp/4001"}

	// 缓存地址
	pf.cacheAddrs(peerID, addrs, "test")

	// 立即获取应该成功
	cached := pf.findInCache(peerID)
	assert.NotEmpty(t, cached)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后应该获取不到
	cached = pf.findInCache(peerID)
	assert.Empty(t, cached)
}

// TestPeerFinder_CacheEviction 测试缓存驱逐
func TestPeerFinder_CacheEviction(t *testing.T) {
	config := PeerFinderConfig{
		MaxCacheSize: 3,
	}
	pf := NewPeerFinder(config)
	defer pf.Close()

	// 添加 4 个条目（超过最大值）
	for i := 0; i < 4; i++ {
		peerID := types.PeerID(string(rune('a' + i)))
		pf.cacheAddrs(peerID, []string{"/ip4/1.2.3.4/tcp/4001"}, "test")
	}

	// 应该只有 3 个条目
	assert.Equal(t, 3, pf.CacheSize())

	// 第一个（最旧的）应该被驱逐
	_, ok := pf.GetCachedPeer(types.PeerID("a"))
	assert.False(t, ok)
}

// TestPeerFinder_ClearCache 测试清空缓存
func TestPeerFinder_ClearCache(t *testing.T) {
	pf := NewPeerFinder(DefaultPeerFinderConfig())
	defer pf.Close()

	// 添加一些条目
	for i := 0; i < 5; i++ {
		peerID := types.PeerID(string(rune('a' + i)))
		pf.cacheAddrs(peerID, []string{"/ip4/1.2.3.4/tcp/4001"}, "test")
	}

	assert.Equal(t, 5, pf.CacheSize())

	// 清空
	pf.ClearCache()
	assert.Equal(t, 0, pf.CacheSize())
}

// ============================================================================
//                              发现源管理测试
// ============================================================================

// TestPeerFinder_DiscoveryManagement 测试发现源管理
func TestPeerFinder_DiscoveryManagement(t *testing.T) {
	pf := NewPeerFinder(DefaultPeerFinderConfig())
	defer pf.Close()

	// 初始无发现源
	stats := pf.Stats()
	assert.Equal(t, 0, stats.DiscoverySourceCount)

	// 注册 nil 发现源（应该被忽略）
	pf.RegisterDiscovery("nil", nil)
	stats = pf.Stats()
	assert.Equal(t, 0, stats.DiscoverySourceCount)

	// 注销不存在的发现源（不应该 panic）
	pf.UnregisterDiscovery("nonexistent")
}

// ============================================================================
//                              生命周期测试
// ============================================================================

// TestPeerFinder_StartClose 测试启动和关闭
func TestPeerFinder_StartClose(t *testing.T) {
	pf := NewPeerFinder(DefaultPeerFinderConfig())

	// 启动
	err := pf.Start(context.Background())
	assert.NoError(t, err)

	// 关闭
	err = pf.Close()
	assert.NoError(t, err)

	// 重复关闭应该是幂等的
	err = pf.Close()
	assert.NoError(t, err)
}

// TestPeerFinder_FindPeerAfterClose 测试关闭后查找
func TestPeerFinder_FindPeerAfterClose(t *testing.T) {
	pf := NewPeerFinder(DefaultPeerFinderConfig())
	pf.Close()

	_, err := pf.FindPeer(context.Background(), types.PeerID("test"))
	assert.ErrorIs(t, err, ErrFinderClosed)
}

// ============================================================================
//                              统计测试
// ============================================================================

// TestPeerFinder_Stats 测试统计信息
func TestPeerFinder_Stats(t *testing.T) {
	pf := NewPeerFinder(DefaultPeerFinderConfig())
	defer pf.Close()

	stats := pf.Stats()
	assert.Equal(t, 0, stats.CacheSize)
	assert.Equal(t, 0, stats.DiscoverySourceCount)
	assert.False(t, stats.HasPeerstore)
	assert.False(t, stats.HasSwarm)

	// 添加缓存
	pf.cacheAddrs(types.PeerID("test"), []string{"/ip4/1.2.3.4/tcp/4001"}, "test")

	stats = pf.Stats()
	assert.Equal(t, 1, stats.CacheSize)
}

// ============================================================================
//                              PeerCacheEntry 测试
// ============================================================================

// TestPeerCacheEntry_IsExpired 测试缓存条目过期
func TestPeerCacheEntry_IsExpired(t *testing.T) {
	t.Run("not expired", func(t *testing.T) {
		entry := &PeerCacheEntry{
			LastSeen: time.Now(),
			TTL:      10 * time.Minute,
		}
		assert.False(t, entry.IsExpired())
	})

	t.Run("expired", func(t *testing.T) {
		entry := &PeerCacheEntry{
			LastSeen: time.Now().Add(-20 * time.Minute),
			TTL:      10 * time.Minute,
		}
		assert.True(t, entry.IsExpired())
	})

	t.Run("no TTL", func(t *testing.T) {
		entry := &PeerCacheEntry{
			LastSeen: time.Now().Add(-1 * time.Hour),
			TTL:      0,
		}
		assert.False(t, entry.IsExpired()) // TTL=0 表示永不过期
	})
}
