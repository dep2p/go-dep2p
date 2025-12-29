package quic

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              配置测试
// ============================================================================

func TestDefaultMigratorConfig(t *testing.T) {
	config := DefaultMigratorConfig()

	assert.Equal(t, 5*time.Second, config.PollInterval)
	assert.True(t, config.EnableAutoMigration)
}

// ============================================================================
//                              ConnectionMigrator 测试
// ============================================================================

func TestNewConnectionMigrator(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	require.NotNil(t, migrator)
	assert.NotNil(t, migrator.migrationCh)
	assert.NotNil(t, migrator.callbacks)
	assert.NotNil(t, migrator.stopCh)
}

func TestConnectionMigrator_StartStop(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := MigratorConfig{
		PollInterval:        100 * time.Millisecond,
		EnableAutoMigration: true,
	}

	err := migrator.Start(ctx, config)
	require.NoError(t, err)

	// 等待启动
	time.Sleep(50 * time.Millisecond)

	err = migrator.Stop()
	require.NoError(t, err)
}

func TestConnectionMigrator_DoubleStart(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	ctx := context.Background()
	config := DefaultMigratorConfig()

	// 第一次启动
	err := migrator.Start(ctx, config)
	require.NoError(t, err)

	// 第二次启动应该是幂等的
	err = migrator.Start(ctx, config)
	require.NoError(t, err)

	migrator.Stop()
}

func TestConnectionMigrator_DoubleStop(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	ctx := context.Background()
	config := DefaultMigratorConfig()

	err := migrator.Start(ctx, config)
	require.NoError(t, err)

	// 第一次停止
	err = migrator.Stop()
	require.NoError(t, err)

	// 第二次停止应该是幂等的
	err = migrator.Stop()
	require.NoError(t, err)
}

func TestConnectionMigrator_MultipleStopNoPanic(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	ctx := context.Background()
	config := DefaultMigratorConfig()

	err := migrator.Start(ctx, config)
	require.NoError(t, err)

	// 多次调用 Stop 不应 panic (使用 sync.Once 保护)
	for i := 0; i < 10; i++ {
		err = migrator.Stop()
		assert.NoError(t, err)
	}
}

func TestConnectionMigrator_StopWithoutStart(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	// 未启动就停止应该不会有问题
	err := migrator.Stop()
	require.NoError(t, err)
}

// ============================================================================
//                              地址检测测试
// ============================================================================

func TestConnectionMigrator_GetCurrentAddresses(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	ctx := context.Background()
	config := DefaultMigratorConfig()

	err := migrator.Start(ctx, config)
	require.NoError(t, err)
	defer migrator.Stop()

	// 获取当前地址
	addrs := migrator.GetCurrentAddresses()

	// 应该至少有一些地址（取决于系统配置）
	// 注意：在某些环境中可能没有非回环地址
	t.Logf("检测到 %d 个网络地址", len(addrs))
}

func TestConnectionMigrator_GetNetworkAddresses(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	addrs, err := migrator.getNetworkAddresses()
	require.NoError(t, err)

	// 验证返回的地址都不是回环地址
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok {
			assert.False(t, ipNet.IP.IsLoopback(), "不应包含回环地址: %v", addr)
		}
	}
}

// ============================================================================
//                              地址比较测试
// ============================================================================

func TestConnectionMigrator_DiffAddresses(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	// 创建测试地址
	oldAddrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.168.1.2"), Mask: net.CIDRMask(24, 32)},
	}

	newAddrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.2"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("192.168.1.3"), Mask: net.CIDRMask(24, 32)},
	}

	added, removed := migrator.diffAddresses(oldAddrs, newAddrs)

	// 应该有一个新增和一个移除
	assert.Len(t, added, 1)
	assert.Len(t, removed, 1)
	assert.Equal(t, "192.168.1.3/24", added[0].String())
	assert.Equal(t, "192.168.1.1/24", removed[0].String())
}

func TestConnectionMigrator_DiffAddresses_NoChange(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
	}

	added, removed := migrator.diffAddresses(addrs, addrs)

	assert.Empty(t, added)
	assert.Empty(t, removed)
}

func TestConnectionMigrator_DiffAddresses_AllNew(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	oldAddrs := []net.Addr{}
	newAddrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
	}

	added, removed := migrator.diffAddresses(oldAddrs, newAddrs)

	assert.Len(t, added, 1)
	assert.Empty(t, removed)
}

func TestConnectionMigrator_DiffAddresses_AllRemoved(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	oldAddrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
	}
	newAddrs := []net.Addr{}

	added, removed := migrator.diffAddresses(oldAddrs, newAddrs)

	assert.Empty(t, added)
	assert.Len(t, removed, 1)
}

// ============================================================================
//                              回调测试
// ============================================================================

func TestConnectionMigrator_OnAddressChange(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	var mu sync.Mutex
	callCount := 0
	var lastEvent MigrationEvent

	migrator.OnAddressChange(func(event MigrationEvent) {
		mu.Lock()
		callCount++
		lastEvent = event
		mu.Unlock()
	})

	// 模拟触发回调
	event := MigrationEvent{
		OldAddrs:     []net.Addr{},
		NewAddrs:     []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)}},
		AddedAddrs:   []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)}},
		RemovedAddrs: []net.Addr{},
		Timestamp:    time.Now(),
	}

	migrator.triggerCallbacks(event)

	// 等待回调执行
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	assert.Equal(t, 1, callCount)
	assert.Len(t, lastEvent.AddedAddrs, 1)
	mu.Unlock()
}

func TestConnectionMigrator_MigrationEvents(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	eventCh := migrator.MigrationEvents()
	require.NotNil(t, eventCh)
}

// ============================================================================
//                              手动触发测试
// ============================================================================

func TestConnectionMigrator_TriggerMigration(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	// 初始化地址
	migrator.currentAddrs = []net.Addr{}

	err := migrator.TriggerMigration()
	require.NoError(t, err)

	// 检查地址是否已更新
	addrs := migrator.GetCurrentAddresses()
	// 地址可能已更新
	t.Logf("触发迁移后检测到 %d 个地址", len(addrs))
}

// ============================================================================
//                              IsPublicAddr 测试
// ============================================================================

func TestIsPublicAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     net.Addr
		expected bool
	}{
		{
			name:     "公网 IPv4",
			addr:     &net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(32, 32)},
			expected: true,
		},
		{
			name:     "私网 IPv4 (10.x.x.x)",
			addr:     &net.IPNet{IP: net.ParseIP("10.0.0.1"), Mask: net.CIDRMask(8, 32)},
			expected: false,
		},
		{
			name:     "私网 IPv4 (192.168.x.x)",
			addr:     &net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
			expected: false,
		},
		{
			name:     "私网 IPv4 (172.16.x.x)",
			addr:     &net.IPNet{IP: net.ParseIP("172.16.0.1"), Mask: net.CIDRMask(12, 32)},
			expected: false,
		},
		{
			name:     "回环地址",
			addr:     &net.IPNet{IP: net.ParseIP("127.0.0.1"), Mask: net.CIDRMask(8, 32)},
			expected: false,
		},
		{
			name:     "链路本地地址",
			addr:     &net.IPNet{IP: net.ParseIP("169.254.1.1"), Mask: net.CIDRMask(16, 32)},
			expected: false,
		},
		{
			name:     "非 IPNet 类型",
			addr:     &net.TCPAddr{IP: net.ParseIP("8.8.8.8"), Port: 80},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPublicAddr(tt.addr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFilterPublicAddrs(t *testing.T) {
	addrs := []net.Addr{
		&net.IPNet{IP: net.ParseIP("8.8.8.8"), Mask: net.CIDRMask(32, 32)},
		&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("1.1.1.1"), Mask: net.CIDRMask(32, 32)},
	}

	public := FilterPublicAddrs(addrs)

	assert.Len(t, public, 2)
}

// ============================================================================
//                              MigrationEvent 测试
// ============================================================================

func TestMigrationEvent(t *testing.T) {
	event := MigrationEvent{
		OldAddrs:     []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)}},
		NewAddrs:     []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.2"), Mask: net.CIDRMask(24, 32)}},
		AddedAddrs:   []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.2"), Mask: net.CIDRMask(24, 32)}},
		RemovedAddrs: []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.1.1"), Mask: net.CIDRMask(24, 32)}},
		Timestamp:    time.Now(),
	}

	assert.Len(t, event.OldAddrs, 1)
	assert.Len(t, event.NewAddrs, 1)
	assert.Len(t, event.AddedAddrs, 1)
	assert.Len(t, event.RemovedAddrs, 1)
	assert.False(t, event.Timestamp.IsZero())
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestConnectionMigrator_ConcurrentAccess(t *testing.T) {
	migrator := NewConnectionMigrator(nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	config := MigratorConfig{
		PollInterval:        50 * time.Millisecond,
		EnableAutoMigration: true,
	}

	err := migrator.Start(ctx, config)
	require.NoError(t, err)
	defer migrator.Stop()

	var wg sync.WaitGroup
	n := 50

	// 并发获取地址
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			migrator.GetCurrentAddresses()
		}()
	}

	// 并发触发迁移
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			migrator.TriggerMigration()
		}()
	}

	// 并发注册回调
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			migrator.OnAddressChange(func(event MigrationEvent) {})
		}()
	}

	wg.Wait()
}

