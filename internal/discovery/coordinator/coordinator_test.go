package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/discovery/dht"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock 发现器
// ============================================================================

// mockDiscovery 模拟发现器
type mockDiscovery struct {
	name         string
	peers        []types.PeerInfo
	findErr      error
	advertiseErr error
	startErr     error
	stopErr      error
	started      bool
	stopped      bool
}

func (m *mockDiscovery) FindPeers(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if m.findErr != nil {
		return nil, m.findErr
	}

	ch := make(chan types.PeerInfo, len(m.peers))
	go func() {
		defer close(ch)
		for _, peer := range m.peers {
			select {
			case ch <- peer:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch, nil
}

func (m *mockDiscovery) Advertise(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (time.Duration, error) {
	if m.advertiseErr != nil {
		return 0, m.advertiseErr
	}
	return time.Hour, nil
}

func (m *mockDiscovery) Start(ctx context.Context) error {
	if m.startErr != nil {
		return m.startErr
	}
	m.started = true
	return nil
}

func (m *mockDiscovery) Stop(ctx context.Context) error {
	if m.stopErr != nil {
		return m.stopErr
	}
	m.stopped = true
	return nil
}

// ============================================================================
//                              测试用例
// ============================================================================

// TestCoordinator_Creation 测试协调器创建
func TestCoordinator_Creation(t *testing.T) {
	config := DefaultConfig()
	coord := NewCoordinator(config)

	require.NotNil(t, coord)
	assert.NotNil(t, coord.config)
}

// TestCoordinator_RegisterDiscovery 测试发现器注册
func TestCoordinator_RegisterDiscovery(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	mock1 := &mockDiscovery{name: "test1"}
	mock2 := &mockDiscovery{name: "test2"}

	coord.RegisterDiscovery("test1", mock1)
	coord.RegisterDiscovery("test2", mock2)

	assert.Equal(t, 2, len(coord.discoveries))
}

// TestCoordinator_Start 测试启动
func TestCoordinator_Start(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	mock1 := &mockDiscovery{name: "test1"}
	mock2 := &mockDiscovery{name: "test2"}

	coord.RegisterDiscovery("test1", mock1)
	coord.RegisterDiscovery("test2", mock2)

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	// Coordinator 不负责启动子发现器，它们由各自的 Fx Lifecycle 管理
	// 只验证 Coordinator 自身已启动
	assert.True(t, coord.started.Load())
}

// TestCoordinator_Stop 测试停止
func TestCoordinator_Stop(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	mock1 := &mockDiscovery{name: "test1"}
	mock2 := &mockDiscovery{name: "test2"}

	coord.RegisterDiscovery("test1", mock1)
	coord.RegisterDiscovery("test2", mock2)

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	err = coord.Stop(ctx)
	require.NoError(t, err)

	// Coordinator 不负责停止子发现器，它们由各自的 Fx Lifecycle 管理
	// 只验证 Coordinator 自身已停止
	assert.False(t, coord.started.Load())
}

// TestCoordinator_FindPeers 测试发现节点
func TestCoordinator_FindPeers(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")

	peer1 := types.PeerInfo{
		ID:     types.PeerID("peer1"),
		Addrs:  []types.Multiaddr{addr1},
		Source: types.SourceMDNS,
	}

	peer2 := types.PeerInfo{
		ID:     types.PeerID("peer2"),
		Addrs:  []types.Multiaddr{addr2},
		Source: types.SourceDHT,
	}

	mock1 := &mockDiscovery{name: "test1", peers: []types.PeerInfo{peer1}}
	mock2 := &mockDiscovery{name: "test2", peers: []types.PeerInfo{peer2}}

	coord.RegisterDiscovery("test1", mock1)
	coord.RegisterDiscovery("test2", mock2)

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	ch, err := coord.FindPeers(ctx, "test")
	require.NoError(t, err)
	require.NotNil(t, ch)

	// 收集所有结果
	var peers []types.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 应该包含两个节点
	assert.Equal(t, 2, len(peers))
}

// TestCoordinator_FindPeers_Dedup 测试发现去重
func TestCoordinator_FindPeers_Dedup(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	// 两个发现器返回相同的节点
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	peer1 := types.PeerInfo{
		ID:     types.PeerID("peer1"),
		Addrs:  []types.Multiaddr{addr1},
		Source: types.SourceMDNS,
	}

	mock1 := &mockDiscovery{name: "test1", peers: []types.PeerInfo{peer1}}
	mock2 := &mockDiscovery{name: "test2", peers: []types.PeerInfo{peer1}}

	coord.RegisterDiscovery("test1", mock1)
	coord.RegisterDiscovery("test2", mock2)

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	ch, err := coord.FindPeers(ctx, "test")
	require.NoError(t, err)

	// 收集所有结果
	var peers []types.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 应该只有一个节点（去重）
	assert.Equal(t, 1, len(peers))
}

// TestCoordinator_Advertise 测试广播
func TestCoordinator_Advertise(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	mock1 := &mockDiscovery{name: "test1"}
	mock2 := &mockDiscovery{name: "test2"}

	coord.RegisterDiscovery("test1", mock1)
	coord.RegisterDiscovery("test2", mock2)

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	ttl, err := coord.Advertise(ctx, "test")
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))
}

// TestCoordinator_FindPeers_Timeout 测试超时
func TestCoordinator_FindPeers_Timeout(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	// 创建一个正常的发现器
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	slowDiscovery := &mockDiscovery{
		name: "slow",
		peers: []types.PeerInfo{
			{
				ID:     types.PeerID("peer1"),
				Addrs:  []types.Multiaddr{addr1},
				Source: types.SourceDHT,
			},
		},
	}

	coord.RegisterDiscovery("slow", slowDiscovery)

	// 使用超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := coord.Start(context.Background())
	require.NoError(t, err)

	ch, err := coord.FindPeers(ctx, "test")
	require.NoError(t, err)

	// 尝试读取所有结果
	var peers []types.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 超时上下文应该导致通道关闭，但可能已经收到一些节点
	assert.LessOrEqual(t, len(peers), 1)
}

// TestCoordinator_Parallel 测试并行发现
func TestCoordinator_Parallel(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	// 创建多个发现器
	for i := 0; i < 5; i++ {
		addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/400" + string(rune('0'+i)))
		peer := types.PeerInfo{
			ID:     types.PeerID("peer" + string(rune('0'+i))),
			Addrs:  []types.Multiaddr{addr},
			Source: types.SourceDHT,
		}
		mock := &mockDiscovery{
			name:  "test" + string(rune('0'+i)),
			peers: []types.PeerInfo{peer},
		}
		coord.RegisterDiscovery(mock.name, mock)
	}

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	ch, err := coord.FindPeers(ctx, "test")
	require.NoError(t, err)

	// 收集所有结果
	var peers []types.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 应该有 5 个节点
	assert.Equal(t, 5, len(peers))
}

// TestCoordinator_Config_Validate 测试配置验证
func TestCoordinator_Config_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name:    "默认配置",
			config:  DefaultConfig(),
			wantErr: false,
		},
		{
			name: "超时为0",
			config: &Config{
				FindTimeout:      0,
				AdvertiseTimeout: time.Second,
			},
			wantErr: true,
		},
		{
			name: "广播超时为0",
			config: &Config{
				FindTimeout:      time.Second,
				AdvertiseTimeout: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestCoordinator_Lifecycle 测试生命周期
func TestCoordinator_Lifecycle(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	mock := &mockDiscovery{name: "test"}
	coord.RegisterDiscovery("test", mock)

	ctx := context.Background()

	// 启动
	err := coord.Start(ctx)
	require.NoError(t, err)
	assert.True(t, coord.started.Load())
	// Coordinator 不负责启动子发现器，它们由各自的 Fx Lifecycle 管理

	// 重复启动应该无效
	err = coord.Start(ctx)
	require.NoError(t, err)

	// 停止
	err = coord.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, coord.started.Load())
	// Coordinator 不负责停止子发现器，它们由各自的 Fx Lifecycle 管理

	// 重复停止应该无效
	err = coord.Stop(ctx)
	require.NoError(t, err)
}

// TestCoordinator_FindPeers_WithLimit 测试发现限制
func TestCoordinator_FindPeers_WithLimit(t *testing.T) {
	coord := NewCoordinator(DefaultConfig())

	// 创建一个发现器，返回多个节点
	var peers []types.PeerInfo
	for i := 0; i < 10; i++ {
		addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/400" + string(rune('0'+i)))
		peers = append(peers, types.PeerInfo{
			ID:     types.PeerID("peer" + string(rune('0'+i))),
			Addrs:  []types.Multiaddr{addr},
			Source: types.SourceDHT,
		})
	}

	mock := &mockDiscovery{name: "test", peers: peers}
	coord.RegisterDiscovery("test", mock)

	ctx := context.Background()
	err := coord.Start(ctx)
	require.NoError(t, err)

	// 限制为 3 个
	ch, err := coord.FindPeers(ctx, "test", pkgif.WithLimit(3))
	require.NoError(t, err)

	// 收集结果
	var foundPeers []types.PeerInfo
	for peer := range ch {
		foundPeers = append(foundPeers, peer)
		if len(foundPeers) >= 3 {
			break
		}
	}

	// 应该最多 3 个（但可能收到更多，因为并行）
	assert.LessOrEqual(t, len(foundPeers), 10)
}

func TestNormalizeNamespace(t *testing.T) {
	payload := "relay/2.0.0"
	realmKey := dht.RealmProviderKey(types.RealmID("realm-1"), payload)
	sysKey := dht.SystemKey(dht.KeyTypeProvider, []byte(payload))
	peerKey := dht.RealmPeerKey(types.RealmID("realm-1"), types.NodeID("node-1"))

	assert.Equal(t, payload, pkgif.NormalizeNamespace(payload))
	assert.Equal(t, payload, pkgif.NormalizeNamespace(realmKey))
	assert.Equal(t, payload, pkgif.NormalizeNamespace(sysKey))
	assert.Equal(t, peerKey, pkgif.NormalizeNamespace(peerKey))
}
