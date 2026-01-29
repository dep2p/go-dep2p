package relay

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

// TestDefaultRelayDiscoveryConfig 测试默认配置
func TestDefaultRelayDiscoveryConfig(t *testing.T) {
	config := DefaultRelayDiscoveryConfig()

	assert.Equal(t, DefaultDiscoveryInterval, config.DiscoveryInterval)
	assert.Equal(t, DefaultAdvertiseInterval, config.AdvertiseInterval)
	assert.Equal(t, DefaultDiscoveryTimeout, config.DiscoveryTimeout)
	assert.Equal(t, DefaultMaxRelays, config.MaxRelays)
}

// TestNewRelayDiscovery 测试创建发现服务
func TestNewRelayDiscovery(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
		require.NotNil(t, rd)
		defer rd.Close()

		assert.Equal(t, DefaultDiscoveryInterval, rd.discoveryInterval)
		assert.Equal(t, DefaultAdvertiseInterval, rd.advertiseInterval)
		assert.Equal(t, DefaultDiscoveryTimeout, rd.discoveryTimeout)
		assert.Equal(t, DefaultMaxRelays, rd.maxRelays)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := RelayDiscoveryConfig{
			DiscoveryInterval: 1 * time.Minute,
			AdvertiseInterval: 2 * time.Minute,
			DiscoveryTimeout:  10 * time.Second,
			MaxRelays:         5,
		}
		rd := NewRelayDiscovery(nil, nil, nil, config)
		require.NotNil(t, rd)
		defer rd.Close()

		assert.Equal(t, 1*time.Minute, rd.discoveryInterval)
		assert.Equal(t, 2*time.Minute, rd.advertiseInterval)
		assert.Equal(t, 10*time.Second, rd.discoveryTimeout)
		assert.Equal(t, 5, rd.maxRelays)
	})

	t.Run("with zero values uses defaults", func(t *testing.T) {
		config := RelayDiscoveryConfig{}
		rd := NewRelayDiscovery(nil, nil, nil, config)
		require.NotNil(t, rd)
		defer rd.Close()

		assert.Equal(t, DefaultDiscoveryInterval, rd.discoveryInterval)
		assert.Equal(t, DefaultAdvertiseInterval, rd.advertiseInterval)
		assert.Equal(t, DefaultDiscoveryTimeout, rd.discoveryTimeout)
		assert.Equal(t, DefaultMaxRelays, rd.maxRelays)
	})
}

// ============================================================================
//                              缓存管理测试
// ============================================================================

// TestRelayDiscovery_AddRemoveRelay 测试添加和移除中继
func TestRelayDiscovery_AddRemoveRelay(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	// 添加中继
	relay1 := DiscoveredRelay{
		PeerID:  types.PeerID("relay-1"),
		Addrs:   []string{"/ip4/1.2.3.4/tcp/4001"},
		Latency: 50 * time.Millisecond,
		Source:  "dht",
	}
	rd.AddRelay(relay1)

	// 验证添加成功
	got, ok := rd.GetRelay(types.PeerID("relay-1"))
	require.True(t, ok)
	assert.Equal(t, relay1.PeerID, got.PeerID)
	assert.NotZero(t, got.Score) // 应该自动计算评分

	// 添加第二个中继
	relay2 := DiscoveredRelay{
		PeerID:  types.PeerID("relay-2"),
		Addrs:   []string{"/ip4/5.6.7.8/tcp/4001"},
		Latency: 100 * time.Millisecond,
		Source:  "realm",
	}
	rd.AddRelay(relay2)

	// 获取所有中继
	relays := rd.GetRelays()
	assert.Len(t, relays, 2)

	// 移除中继
	rd.RemoveRelay(types.PeerID("relay-1"))
	_, ok = rd.GetRelay(types.PeerID("relay-1"))
	assert.False(t, ok)

	relays = rd.GetRelays()
	assert.Len(t, relays, 1)

	// 清空
	rd.ClearRelays()
	relays = rd.GetRelays()
	assert.Empty(t, relays)
}

// TestRelayDiscovery_GetRelay 测试获取中继
func TestRelayDiscovery_GetRelay(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	// 不存在的中继
	got, ok := rd.GetRelay(types.PeerID("nonexistent"))
	assert.False(t, ok)
	assert.Nil(t, got)

	// 添加后获取
	relay := DiscoveredRelay{
		PeerID: types.PeerID("relay-1"),
		Addrs:  []string{"/ip4/1.2.3.4/tcp/4001"},
	}
	rd.AddRelay(relay)

	got, ok = rd.GetRelay(types.PeerID("relay-1"))
	assert.True(t, ok)
	assert.NotNil(t, got)
	assert.Equal(t, relay.PeerID, got.PeerID)
}

// ============================================================================
//                              评分测试
// ============================================================================

// TestRelayDiscovery_CalculateScore 测试评分计算
func TestRelayDiscovery_CalculateScore(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	tests := []struct {
		name    string
		relay   *DiscoveredRelay
		minScore float64
		maxScore float64
	}{
		{
			name: "low latency dht",
			relay: &DiscoveredRelay{
				Latency: 10 * time.Millisecond,
				Load:    0,
				Source:  "dht",
			},
			minScore: 90,
			maxScore: 110,
		},
		{
			name: "high latency dht",
			relay: &DiscoveredRelay{
				Latency: 500 * time.Millisecond,
				Load:    50,
				Source:  "dht",
			},
			minScore: 0,
			maxScore: 50,
		},
		{
			name: "realm source bonus",
			relay: &DiscoveredRelay{
				Latency: 100 * time.Millisecond,
				Load:    0,
				Source:  "realm",
			},
			minScore: 100, // 100 - 10 + 20 = 110
			maxScore: 120,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := rd.calculateScore(tt.relay)
			assert.GreaterOrEqual(t, score, tt.minScore)
			assert.LessOrEqual(t, score, tt.maxScore)
		})
	}
}

// TestRelayDiscovery_SortRelays 测试中继排序
func TestRelayDiscovery_SortRelays(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	relays := []DiscoveredRelay{
		{PeerID: types.PeerID("low"), Score: 50},
		{PeerID: types.PeerID("high"), Score: 100},
		{PeerID: types.PeerID("mid"), Score: 75},
	}

	rd.sortRelays(relays)

	// 应该按评分降序排列
	assert.Equal(t, types.PeerID("high"), relays[0].PeerID)
	assert.Equal(t, types.PeerID("mid"), relays[1].PeerID)
	assert.Equal(t, types.PeerID("low"), relays[2].PeerID)
}

// ============================================================================
//                              中继服务器模式测试
// ============================================================================

// TestRelayDiscovery_RelayServerMode 测试中继服务器模式
func TestRelayDiscovery_RelayServerMode(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	// 默认不是中继服务器
	assert.False(t, rd.isRelayServer)

	// 启用中继服务器模式
	config := RelayServerConfig{
		MaxConnections: 100,
		MaxDuration:    1 * time.Hour,
		MaxData:        10 * 1024 * 1024,
	}
	rd.EnableRelayServer(config)

	assert.True(t, rd.isRelayServer)
	assert.Equal(t, 100, rd.relayServerConfig.MaxConnections)

	// 禁用中继服务器模式
	rd.DisableRelayServer()
	assert.False(t, rd.isRelayServer)
}

// TestRelayDiscovery_AdvertiseNotServer 测试非服务器模式发布
func TestRelayDiscovery_AdvertiseNotServer(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	err := rd.Advertise(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured as relay server")
}

// ============================================================================
//                              生命周期测试
// ============================================================================

// TestRelayDiscovery_StartClose 测试启动和关闭
func TestRelayDiscovery_StartClose(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())

	// 启动
	err := rd.Start(context.Background())
	assert.NoError(t, err)

	// 重复启动应该是幂等的
	err = rd.Start(context.Background())
	assert.NoError(t, err)

	// 关闭
	err = rd.Close()
	assert.NoError(t, err)

	// 重复关闭应该是幂等的
	err = rd.Close()
	assert.NoError(t, err)
}

// TestRelayDiscovery_DiscoverAfterClose 测试关闭后发现
func TestRelayDiscovery_DiscoverAfterClose(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	rd.Close()

	_, err := rd.Discover(context.Background())
	assert.ErrorIs(t, err, ErrDiscoveryClosed)
}

// TestRelayDiscovery_AdvertiseAfterClose 测试关闭后发布
func TestRelayDiscovery_AdvertiseAfterClose(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	rd.EnableRelayServer(RelayServerConfig{})
	rd.Close()

	err := rd.Advertise(context.Background())
	assert.ErrorIs(t, err, ErrDiscoveryClosed)
}

// ============================================================================
//                              统计测试
// ============================================================================

// TestRelayDiscovery_Stats 测试统计信息
func TestRelayDiscovery_Stats(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	// 初始状态
	stats := rd.Stats()
	assert.Equal(t, 0, stats.CachedRelays)
	assert.False(t, stats.IsRelayServer)

	// 添加中继
	rd.AddRelay(DiscoveredRelay{PeerID: types.PeerID("relay-1")})
	rd.AddRelay(DiscoveredRelay{PeerID: types.PeerID("relay-2")})

	stats = rd.Stats()
	assert.Equal(t, 2, stats.CachedRelays)

	// 启用中继服务器
	rd.EnableRelayServer(RelayServerConfig{})
	stats = rd.Stats()
	assert.True(t, stats.IsRelayServer)
}

// ============================================================================
//                              Realm 设置测试
// ============================================================================

// TestRelayDiscovery_SetRealm 测试设置 Realm
func TestRelayDiscovery_SetRealm(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	// v2.0: 统一 Relay 架构，realm 字段已移除
	// 测试基本创建不会 panic
	assert.NotNil(t, rd)
}

// ============================================================================
//                              并发测试
// ============================================================================

// TestRelayDiscovery_Concurrent 测试并发安全
func TestRelayDiscovery_Concurrent(t *testing.T) {
	rd := NewRelayDiscovery(nil, nil, nil, DefaultRelayDiscoveryConfig())
	defer rd.Close()

	done := make(chan bool)

	// 并发添加
	go func() {
		for i := 0; i < 100; i++ {
			rd.AddRelay(DiscoveredRelay{
				PeerID: types.PeerID(string(rune('a' + i%26))),
			})
		}
		done <- true
	}()

	// 并发移除
	go func() {
		for i := 0; i < 100; i++ {
			rd.RemoveRelay(types.PeerID(string(rune('a' + i%26))))
		}
		done <- true
	}()

	// 并发读取
	go func() {
		for i := 0; i < 100; i++ {
			_ = rd.GetRelays()
			_, _ = rd.GetRelay(types.PeerID("a"))
			_ = rd.Stats()
		}
		done <- true
	}()

	// 等待完成
	for i := 0; i < 3; i++ {
		<-done
	}
}

// ============================================================================
//                              常量测试
// ============================================================================

// TestRelayDiscoveryConstants 测试常量值
func TestRelayDiscoveryConstants(t *testing.T) {
	assert.Equal(t, "relay/1.0.0", RelayNamespaceLocal)
	assert.Equal(t, 5*time.Minute, DefaultDiscoveryInterval)
	assert.Equal(t, 10*time.Minute, DefaultAdvertiseInterval)
	// v2.0.1: DefaultDiscoveryTimeout 从 30s 增加到 60s，配合 DHT QueryTimeout 增加
	assert.Equal(t, 60*time.Second, DefaultDiscoveryTimeout)
	assert.Equal(t, 10, DefaultMaxRelays)
	assert.Equal(t, 30*time.Minute, RelayTTL)
}
