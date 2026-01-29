package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     NodeStatus.String 测试
// ============================================================================

func TestNodeStatus_String(t *testing.T) {
	tests := []struct {
		status   NodeStatus
		expected string
	}{
		{NodeStatusUnknown, "Unknown"},
		{NodeStatusOnline, "Online"},
		{NodeStatusOffline, "Offline"},
		{NodeStatus(99), "Invalid"}, // 未知值
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}
}

// ============================================================================
//                     BootstrapError.Unwrap 测试
// ============================================================================

func TestBootstrapError_Unwrap(t *testing.T) {
	innerErr := errors.New("inner error")
	bsErr := NewBootstrapError("connect", "peer123", innerErr, "connection failed")

	// 测试 Unwrap
	unwrapped := bsErr.Unwrap()
	assert.Equal(t, innerErr, unwrapped)

	// 测试 errors.Is
	assert.True(t, errors.Is(bsErr, innerErr))
}

func TestBootstrapError_Unwrap_Nil(t *testing.T) {
	bsErr := NewBootstrapError("connect", "peer123", nil, "connection failed")

	// 没有内部错误时 Unwrap 返回 nil
	unwrapped := bsErr.Unwrap()
	assert.Nil(t, unwrapped)
}

// ============================================================================
//                     Bootstrap.Advertise 测试
// ============================================================================

func TestBootstrap_Advertise(t *testing.T) {
	host := &mockHost{}
	config := &Config{Enabled: true}

	bs, err := New(host, config)
	require.NoError(t, err)
	defer bs.Stop(context.Background())

	// Advertise 不支持，应返回 ErrNotSupported
	ttl, err := bs.Advertise(context.Background(), "test-ns")
	assert.Equal(t, time.Duration(0), ttl)
	assert.ErrorIs(t, err, ErrNotSupported)
}

// ============================================================================
//                     ProbeService 测试
// ============================================================================

func TestProbeService_Options(t *testing.T) {
	// 测试 WithProbeInterval
	t.Run("WithProbeInterval", func(t *testing.T) {
		host := &mockHost{}
		store := NewExtendedNodeStore()
		defer store.Close()

		ps := NewProbeService(host, store, nil, WithProbeInterval(5*time.Minute))
		assert.Equal(t, 5*time.Minute, ps.interval)
	})

	// 测试 WithProbeBatchSize
	t.Run("WithProbeBatchSize", func(t *testing.T) {
		host := &mockHost{}
		store := NewExtendedNodeStore()
		defer store.Close()

		ps := NewProbeService(host, store, nil, WithProbeBatchSize(50))
		assert.Equal(t, 50, ps.batchSize)
	})

	// 测试 WithProbeTimeout
	t.Run("WithProbeTimeout", func(t *testing.T) {
		host := &mockHost{}
		store := NewExtendedNodeStore()
		defer store.Close()

		ps := NewProbeService(host, store, nil, WithProbeTimeout(10*time.Second))
		assert.Equal(t, 10*time.Second, ps.timeout)
	})

	// 测试 WithProbeMaxConcurrent
	t.Run("WithProbeMaxConcurrent", func(t *testing.T) {
		host := &mockHost{}
		store := NewExtendedNodeStore()
		defer store.Close()

		ps := NewProbeService(host, store, nil, WithProbeMaxConcurrent(20))
		assert.Equal(t, 20, ps.maxConcurrent)
	})
}

func TestProbeService_NewProbeService(t *testing.T) {
	host := &mockHost{}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil)
	require.NotNil(t, ps)

	assert.NotNil(t, ps.ctx)
	assert.NotNil(t, ps.ctxCancel)
	assert.Equal(t, host, ps.host)
	assert.Equal(t, store, ps.store)
	assert.False(t, ps.running.Load())
}

func TestProbeService_StartStop(t *testing.T) {
	host := &mockHost{}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil, WithProbeInterval(100*time.Millisecond))

	// 初始状态
	assert.False(t, ps.IsRunning())

	// 启动
	err := ps.Start()
	assert.NoError(t, err)
	assert.True(t, ps.IsRunning())

	// 等待一点时间让循环运行
	time.Sleep(50 * time.Millisecond)

	// 停止
	err = ps.Stop()
	assert.NoError(t, err)
	assert.False(t, ps.IsRunning())
}

func TestProbeService_Stats(t *testing.T) {
	host := &mockHost{}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil)

	stats := ps.Stats()
	assert.Equal(t, int64(0), stats.TotalProbes)
	assert.Equal(t, int64(0), stats.SuccessProbes)
	assert.Equal(t, int64(0), stats.FailedProbes)
}

func TestProbeService_LastRun(t *testing.T) {
	host := &mockHost{}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil)

	// 初始 LastRun 应该是零值
	lastRun := ps.LastRun()
	assert.True(t, lastRun.IsZero())
}

func TestProbeService_ProbeNow(t *testing.T) {
	host := &mockHost{}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil, WithProbeInterval(1*time.Hour))

	// 启动服务
	err := ps.Start()
	require.NoError(t, err)
	defer ps.Stop()

	// 触发立即探测
	ps.ProbeNow()

	// 等待探测完成
	time.Sleep(100 * time.Millisecond)

	// 不检查具体结果，只验证没有 panic
}

func TestProbeService_ProbeOne(t *testing.T) {
	host := &mockHost{
		connectFunc: func(ctx context.Context, peerID string, addrs []string) error {
			return errors.New("connection failed")
		},
	}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil)

	// 添加一个测试节点
	entry := &NodeEntry{
		ID:       "test-peer",
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
		Status:   NodeStatusOnline,
	}
	store.Put(entry)

	// 探测单个节点 - mock host 返回连接失败
	alive, err := ps.ProbeOne("test-peer")
	// 连接失败时，节点不应被认为是活跃的
	assert.False(t, alive, "连接失败时节点不应被标记为活跃")
	// 连接失败可能返回错误，也可能返回 nil（取决于实现）
	// 但状态应该正确反映在 alive 中
	t.Logf("ProbeOne 结果: alive=%v, err=%v", alive, err)
}

func TestProbeService_ProbeOne_NotFound(t *testing.T) {
	host := &mockHost{}
	store := NewExtendedNodeStore()
	defer store.Close()

	ps := NewProbeService(host, store, nil)

	// 探测不存在的节点
	alive, err := ps.ProbeOne("nonexistent")
	assert.False(t, alive)
	assert.ErrorIs(t, err, ErrNodeNotFound)
}

// ============================================================================
//                     BootstrapService 测试
// ============================================================================

func TestBootstrapService_Options(t *testing.T) {
	host := &mockHost{}

	t.Run("WithDataDir", func(t *testing.T) {
		svc := NewBootstrapService(host, WithDataDir("/tmp/bootstrap"))
		assert.Equal(t, "/tmp/bootstrap", svc.dataDir)
	})
}

func TestBootstrapService_NewBootstrapService(t *testing.T) {
	host := &mockHost{}

	svc := NewBootstrapService(host)
	require.NotNil(t, svc)

	assert.NotNil(t, svc.ctx)
	assert.NotNil(t, svc.ctxCancel)
	assert.Equal(t, host, svc.host)
	assert.False(t, svc.enabled.Load())
	assert.Equal(t, ".", svc.dataDir)
}

func TestBootstrapService_IsEnabled(t *testing.T) {
	host := &mockHost{}
	svc := NewBootstrapService(host)

	// 初始应该是禁用的
	assert.False(t, svc.IsEnabled())
}

func TestBootstrapService_Enable_NotPubliclyReachable(t *testing.T) {
	// Mock host 返回空的对外通告地址（模拟没有公网地址的情况）
	host := &mockHost{
		advertisedAddrs: []string{}, // 空数组表示没有对外通告地址
	}
	svc := NewBootstrapService(host)

	ctx := context.Background()
	err := svc.Enable(ctx)

	// 应该因为不是公网可达而失败
	assert.ErrorIs(t, err, ErrNotPubliclyReachable)
	assert.False(t, svc.IsEnabled())
}

func TestBootstrapService_Enable_WithUserConfiguredAddress(t *testing.T) {
	// Mock host 返回用户配置的私网地址（通过 --public-addr 配置）
	// 这种情况下应该允许启用，因为用户明确配置了地址
	host := &mockHost{
		advertisedAddrs: []string{"/ip4/192.168.0.100/tcp/4001"}, // 私网地址但用户明确配置
	}
	svc := NewBootstrapService(host)

	ctx := context.Background()
	err := svc.Enable(ctx)

	// 应该成功启用（用户配置的地址被信任）
	assert.NoError(t, err)
	assert.True(t, svc.IsEnabled())
}

func TestBootstrapService_Stats(t *testing.T) {
	host := &mockHost{}
	svc := NewBootstrapService(host)

	stats := svc.Stats()
	// 未启用时统计应该是默认值
	assert.False(t, stats.Enabled)
	assert.Equal(t, 0, stats.TotalNodes)
	assert.Equal(t, 0, stats.OnlineNodes)
}

func TestBootstrapService_Close(t *testing.T) {
	host := &mockHost{}
	svc := NewBootstrapService(host)

	err := svc.Close()
	assert.NoError(t, err)
}

// ============================================================================
//                     Store 扩展测试
// ============================================================================

func TestNodeEntry_IsOffline(t *testing.T) {
	entry := &NodeEntry{
		ID:        "test",
		FailCount: 5,
	}

	// threshold = 3，FailCount = 5，应该是离线
	assert.True(t, entry.IsOffline(3))

	// threshold = 10，FailCount = 5，不是离线
	assert.False(t, entry.IsOffline(10))
}

func TestExtendedNodeStore_WithPersister(t *testing.T) {
	persister := NewMemoryPersister()
	store := NewExtendedNodeStore(WithPersister(persister))
	defer store.Close()

	assert.NotNil(t, store)
}

func TestExtendedNodeStore_LoadFromPersister(t *testing.T) {
	persister := NewMemoryPersister()

	// 先保存一些数据
	entry := &NodeEntry{
		ID:       "test-peer",
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
		Status:   NodeStatusOnline,
	}
	persister.Save(entry)

	// 创建新的 store 并从 persister 加载
	store := NewExtendedNodeStore(WithPersister(persister))
	defer store.Close()

	err := store.LoadFromPersister()
	assert.NoError(t, err)

	// 验证数据已加载
	loaded, ok := store.Get("test-peer")
	require.True(t, ok)
	assert.Equal(t, types.NodeID("test-peer"), loaded.ID)
}

func TestExtendedNodeStore_LoadFromPersister_NoPersister(t *testing.T) {
	store := NewExtendedNodeStore()
	defer store.Close()

	// 没有 persister 时应该返回 nil
	err := store.LoadFromPersister()
	assert.NoError(t, err)
}

// ============================================================================
//                     MemoryPersister 测试
// ============================================================================

func TestMemoryPersister_New(t *testing.T) {
	p := NewMemoryPersister()
	require.NotNil(t, p)
}

func TestMemoryPersister_Save(t *testing.T) {
	p := NewMemoryPersister()

	entry := &NodeEntry{
		ID:       "test-peer",
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
	}

	err := p.Save(entry)
	assert.NoError(t, err)
}

func TestMemoryPersister_Load(t *testing.T) {
	p := NewMemoryPersister()

	// 先保存
	entry := &NodeEntry{
		ID:       "test-peer",
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
	}
	p.Save(entry)

	// 再加载
	loaded, err := p.Load("test-peer")
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, types.NodeID("test-peer"), loaded.ID)
}

func TestMemoryPersister_Load_NotFound(t *testing.T) {
	p := NewMemoryPersister()

	loaded, err := p.Load("nonexistent")
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestMemoryPersister_Delete(t *testing.T) {
	p := NewMemoryPersister()

	// 先保存
	entry := &NodeEntry{
		ID:       "test-peer",
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
	}
	p.Save(entry)

	// 删除
	err := p.Delete("test-peer")
	assert.NoError(t, err)

	// 验证已删除
	loaded, err := p.Load("test-peer")
	assert.Error(t, err)
	assert.Nil(t, loaded)
}

func TestMemoryPersister_LoadAll(t *testing.T) {
	p := NewMemoryPersister()

	// 保存多个节点
	for i := 0; i < 5; i++ {
		entry := &NodeEntry{
			ID:       types.NodeID(string(rune('a' + i))),
			Addrs:    []string{"/ip4/127.0.0.1/tcp/400" + string(rune('1'+i))},
			LastSeen: time.Now(),
		}
		p.Save(entry)
	}

	// 加载所有
	all, err := p.LoadAll()
	require.NoError(t, err)
	assert.Len(t, all, 5)
}

func TestMemoryPersister_Close(t *testing.T) {
	p := NewMemoryPersister()

	err := p.Close()
	assert.NoError(t, err)
}
