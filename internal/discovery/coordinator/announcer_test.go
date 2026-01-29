package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              配置测试
// ============================================================================

// TestDefaultAddressAnnouncerConfig 测试默认配置
func TestDefaultAddressAnnouncerConfig(t *testing.T) {
	config := DefaultAddressAnnouncerConfig()

	assert.Equal(t, 10*time.Minute, config.RefreshInterval)
	assert.Equal(t, 30*time.Second, config.AnnounceTimeout)
	assert.NotEmpty(t, config.Namespaces)
}

// TestNewAddressAnnouncer 测试创建公告器
func TestNewAddressAnnouncer(t *testing.T) {
	t.Run("with default config", func(t *testing.T) {
		aa := NewAddressAnnouncer(DefaultAddressAnnouncerConfig())
		require.NotNil(t, aa)
		defer aa.Close()

		assert.Equal(t, 10*time.Minute, aa.config.RefreshInterval)
	})

	t.Run("with custom config", func(t *testing.T) {
		config := AddressAnnouncerConfig{
			RefreshInterval: 5 * time.Minute,
			AnnounceTimeout: 15 * time.Second,
			Namespaces:      []string{"custom-ns"},
		}
		aa := NewAddressAnnouncer(config)
		require.NotNil(t, aa)
		defer aa.Close()

		assert.Equal(t, 5*time.Minute, aa.config.RefreshInterval)
		assert.Equal(t, 15*time.Second, aa.config.AnnounceTimeout)
		assert.Equal(t, []string{"custom-ns"}, aa.config.Namespaces)
	})

	t.Run("with zero values uses defaults", func(t *testing.T) {
		config := AddressAnnouncerConfig{}
		aa := NewAddressAnnouncer(config)
		require.NotNil(t, aa)
		defer aa.Close()

		assert.Equal(t, DefaultAddressAnnouncerConfig().RefreshInterval, aa.config.RefreshInterval)
	})
}

// ============================================================================
//                              发现源管理测试
// ============================================================================

// TestAddressAnnouncer_DiscoveryManagement 测试发现源管理
func TestAddressAnnouncer_DiscoveryManagement(t *testing.T) {
	aa := NewAddressAnnouncer(DefaultAddressAnnouncerConfig())
	defer aa.Close()

	// 初始无发现源
	stats := aa.Stats()
	assert.Equal(t, 0, stats.DiscoverySourceCount)

	// 注册 nil 发现源（应该被忽略）
	aa.RegisterDiscovery("nil", nil)
	stats = aa.Stats()
	assert.Equal(t, 0, stats.DiscoverySourceCount)

	// 注销不存在的发现源（不应该 panic）
	aa.UnregisterDiscovery("nonexistent")
}

// ============================================================================
//                              公告测试
// ============================================================================

// TestAddressAnnouncer_AnnounceNoDiscoveries 测试无发现源时公告
func TestAddressAnnouncer_AnnounceNoDiscoveries(t *testing.T) {
	aa := NewAddressAnnouncer(DefaultAddressAnnouncerConfig())
	defer aa.Close()

	// 无发现源时应该不返回错误
	err := aa.Announce(context.Background())
	assert.NoError(t, err)
}

// ============================================================================
//                              生命周期测试
// ============================================================================

// TestAddressAnnouncer_StartClose 测试启动和关闭
func TestAddressAnnouncer_StartClose(t *testing.T) {
	aa := NewAddressAnnouncer(DefaultAddressAnnouncerConfig())

	// 启动
	err := aa.Start(context.Background())
	assert.NoError(t, err)

	// 验证运行状态
	stats := aa.Stats()
	assert.True(t, stats.Running)

	// 重复启动应该是幂等的
	err = aa.Start(context.Background())
	assert.NoError(t, err)

	// 关闭
	err = aa.Close()
	assert.NoError(t, err)

	// 重复关闭应该是幂等的
	err = aa.Close()
	assert.NoError(t, err)
}

// ============================================================================
//                              统计测试
// ============================================================================

// TestAddressAnnouncer_Stats 测试统计信息
func TestAddressAnnouncer_Stats(t *testing.T) {
	aa := NewAddressAnnouncer(DefaultAddressAnnouncerConfig())
	defer aa.Close()

	stats := aa.Stats()
	assert.Equal(t, 0, stats.DiscoverySourceCount)
	assert.Equal(t, 1, stats.NamespaceCount) // 默认有一个命名空间
	assert.False(t, stats.Running)

	// 启动后
	aa.Start(context.Background())
	stats = aa.Stats()
	assert.True(t, stats.Running)
}
