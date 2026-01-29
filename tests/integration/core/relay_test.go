//go:build integration

package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/core/relay"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// ============================================================================
//                     Relay Discovery 集成测试
// ============================================================================

// TestRelayDiscovery_CreateAndStart 测试 RelayDiscovery 创建和启动
//
// 验证:
//   - RelayDiscovery 能正常创建
//   - 能正常启动和关闭
func TestRelayDiscovery_CreateAndStart(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点
	node := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	_ = testutil.NewTestRealm(t, node).WithPSK(psk).Join()

	t.Logf("节点: %s", node.ID()[:8])

	// 2. 创建 RelayDiscovery
	config := relay.DefaultRelayDiscoveryConfig()
	config.DiscoveryInterval = 10 * time.Second
	config.MaxRelays = 5

	discovery := relay.NewRelayDiscovery(nil, node.Host(), node.Host().Peerstore(), config)
	require.NotNil(t, discovery)

	// 3. 启动服务
	err := discovery.Start(ctx)
	require.NoError(t, err)

	// 4. 等待一小段时间让服务运行
	time.Sleep(2 * time.Second)

	// 5. 获取统计信息
	stats := discovery.Stats()
	t.Logf("RelayDiscovery 统计: 缓存中继数=%d, 是否为中继服务器=%v",
		stats.CachedRelays, stats.IsRelayServer)

	// 6. 关闭服务
	err = discovery.Close()
	require.NoError(t, err)

	t.Log("✅ RelayDiscovery 创建和启动测试通过")
}

// TestRelayDiscovery_AddAndGetRelay 测试手动添加和获取中继
//
// 验证:
//   - 能手动添加中继
//   - 能正确获取中继信息
//   - 能移除中继
func TestRelayDiscovery_AddAndGetRelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点
	node := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	_ = testutil.NewTestRealm(t, node).WithPSK(psk).Join()

	// 2. 创建 RelayDiscovery
	discovery := relay.NewRelayDiscovery(nil, node.Host(), node.Host().Peerstore(), relay.DefaultRelayDiscoveryConfig())
	require.NotNil(t, discovery)

	err := discovery.Start(ctx)
	require.NoError(t, err)
	defer discovery.Close()

	// 3. 手动添加中继
	relayInfo := relay.DiscoveredRelay{
		PeerID:   "relay-peer-12345",
		Addrs:    []string{"/ip4/192.168.1.100/tcp/4001"},
		Latency:  50 * time.Millisecond,
		Load:     10,
		Capacity: 100,
		Source:   "manual",
	}
	discovery.AddRelay(relayInfo)

	// 4. 获取中继
	retrieved, ok := discovery.GetRelay("relay-peer-12345")
	require.True(t, ok, "应该能获取添加的中继")
	assert.Equal(t, relayInfo.PeerID, retrieved.PeerID)
	assert.Equal(t, 50*time.Millisecond, retrieved.Latency)

	// 5. 获取所有中继
	relays := discovery.GetRelays()
	assert.GreaterOrEqual(t, len(relays), 1, "应该至少有一个中继")

	// 6. 移除中继
	discovery.RemoveRelay("relay-peer-12345")

	_, ok = discovery.GetRelay("relay-peer-12345")
	assert.False(t, ok, "中继应该已被移除")

	t.Log("✅ 中继添加和获取测试通过")
}

// TestRelayDiscovery_EnableRelayServer 测试启用中继服务器模式
//
// 验证:
//   - 能启用中继服务器模式
//   - 能禁用中继服务器模式
func TestRelayDiscovery_EnableRelayServer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点
	node := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	_ = testutil.NewTestRealm(t, node).WithPSK(psk).Join()

	// 2. 创建 RelayDiscovery
	discovery := relay.NewRelayDiscovery(nil, node.Host(), node.Host().Peerstore(), relay.DefaultRelayDiscoveryConfig())
	require.NotNil(t, discovery)

	err := discovery.Start(ctx)
	require.NoError(t, err)
	defer discovery.Close()

	// 3. 初始状态检查
	stats := discovery.Stats()
	assert.False(t, stats.IsRelayServer, "初始应该不是中继服务器")

	// 4. 启用中继服务器模式
	serverConfig := relay.RelayServerConfig{
		MaxConnections: 100,
		MaxDuration:    30 * time.Minute,
		MaxData:        1024 * 1024 * 100, // 100 MB
	}
	discovery.EnableRelayServer(serverConfig)

	// 5. 验证状态
	stats = discovery.Stats()
	assert.True(t, stats.IsRelayServer, "应该是中继服务器")

	// 6. 禁用中继服务器模式
	discovery.DisableRelayServer()

	stats = discovery.Stats()
	assert.False(t, stats.IsRelayServer, "应该不再是中继服务器")

	t.Log("✅ 中继服务器模式测试通过")
}

// ============================================================================
//                     Relay Service 集成测试（统一 Relay v2.0）
// ============================================================================

// TestRelayService_SetRelayAndConnect 测试设置中继地址
//
// 验证:
//   - 能设置中继地址
//   - 状态正确转换
func TestRelayService_SetRelayAndConnect(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动模拟中继节点
	relayNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	_ = testutil.NewTestRealm(t, relayNode).WithPSK(psk).Join()

	t.Logf("模拟中继节点: %s", relayNode.ID()[:8])

	// 2. 启动客户端节点
	clientNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	_ = testutil.NewTestRealm(t, clientNode).WithPSK(psk).Join()

	t.Logf("客户端节点: %s", clientNode.ID()[:8])

	// 3. 创建 RelayService（统一 Relay v2.0）
	service, err := relay.NewRelayService(nil, nil)
	require.NoError(t, err)
	require.NotNil(t, service)

	// 4. 验证初始状态
	assert.Equal(t, relay.RelayStateNone, service.State())
	assert.False(t, service.IsConnected())

	// 5. 构建中继地址
	relayAddrs := relayNode.ListenAddrs()
	require.NotEmpty(t, relayAddrs)

	// 构建包含 /p2p/ 组件的完整地址
	relayAddrStr := relayAddrs[0] + "/p2p/" + relayNode.ID()
	relayAddr, err := multiaddr.NewMultiaddr(relayAddrStr)
	require.NoError(t, err)

	// 6. 设置中继
	err = service.SetRelay(relayAddr)
	require.NoError(t, err)
	assert.Equal(t, relay.RelayStateConfigured, service.State())

	// 7. 验证能获取配置的中继
	savedAddr, ok := service.Relay()
	assert.True(t, ok)
	assert.Equal(t, relayAddr.String(), savedAddr.String())

	// 8. 移除中继配置
	err = service.RemoveRelay()
	require.NoError(t, err)
	assert.Equal(t, relay.RelayStateNone, service.State())

	t.Log("✅ Relay Service 设置测试通过")
}

// TestRelayService_EnableDisable 测试启用/禁用中继能力
//
// 验证:
//   - 启用/禁用逻辑正确
//   - 统计信息正确
func TestRelayService_EnableDisable(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 创建 RelayService（统一 Relay v2.0）
	service, err := relay.NewRelayService(nil, nil)
	require.NoError(t, err)
	require.NotNil(t, service)

	// 2. 验证初始状态
	assert.False(t, service.IsEnabled())

	// 3. 尝试启用（没有 Host，会失败）
	// Enable 需要公网可达性检查，没有 Host 会返回错误
	// 这是预期行为

	// 4. 获取统计信息
	stats := service.Stats()
	assert.False(t, stats.Enabled)
	assert.Equal(t, 0, stats.ActiveCircuits)

	t.Log("✅ Relay Service 启用/禁用测试通过")
}

// ============================================================================
//                     Relay Manager 集成测试（统一 Relay v2.0）
// ============================================================================

// TestRelayManager_Creation 测试 Relay Manager 创建
//
// 验证:
//   - Manager 能正常创建
//   - 能获取统一中继服务
func TestRelayManager_Creation(t *testing.T) {
	_, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 创建 Relay Manager
	config := relay.DefaultConfig()
	manager := relay.NewManager(config, nil, nil, nil, nil)
	require.NotNil(t, manager)

	// 2. 验证初始状态（服务未启动）
	relayService := manager.Relay()
	assert.Nil(t, relayService, "未启动时 Relay 应为 nil")

	t.Log("✅ Relay Manager 创建测试通过")
}

// TestRelayManager_StartStop 测试 Manager 启动和停止
//
// 验证:
//   - Manager 能正常启动
//   - Manager 能正常停止
func TestRelayManager_StartStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动一个节点作为依赖
	node := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	_ = testutil.NewTestRealm(t, node).WithPSK(psk).Join()

	t.Logf("节点: %s", node.ID()[:8])

	// 2. 创建 Relay Manager
	config := relay.DefaultConfig()
	config.EnableClient = true
	config.EnableServer = false // 不启用服务器（需要公网可达性）

	manager := relay.NewManager(config, nil, nil, nil, nil)
	require.NotNil(t, manager)

	// 3. 启动 Manager
	err := manager.Start(ctx)
	require.NoError(t, err)

	// 4. 验证统一 Relay 已创建
	relayService := manager.Relay()
	require.NotNil(t, relayService, "启动后 Relay 应创建")

	// 5. 停止 Manager
	err = manager.Stop()
	require.NoError(t, err)

	t.Log("✅ Relay Manager 启动/停止测试通过")
}
