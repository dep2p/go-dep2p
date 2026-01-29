//go:build integration

package discovery_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/internal/discovery/bootstrap"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestBootstrap_ConnectToBootstrapPeers 测试连接到引导节点列表
//
// 验证:
//   - 节点能连接到多个引导节点
//   - 连接成功后能获取连接数
func TestBootstrap_ConnectToBootstrapPeers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动 3 个引导节点
	bootNodes := make([]*dep2p.Node, 3)
	for i := 0; i < 3; i++ {
		bootNodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			WithPreset("minimal").
			Start()
		t.Logf("引导节点 %d: %s", i, bootNodes[i].ID()[:8])
	}

	// 2. 启动客户端节点
	clientNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	t.Logf("客户端节点: %s", clientNode.ID()[:8])

	// 3. 构建引导节点列表
	bootstrapPeers := make([]types.PeerInfo, len(bootNodes))
	for i, bootNode := range bootNodes {
		addrs := bootNode.ListenAddrs()
		multiaddrs := make([]types.Multiaddr, len(addrs))
		for j, addr := range addrs {
			multiaddr, err := types.ParseMultiaddr(addr)
			require.NoError(t, err)
			multiaddrs[j] = multiaddr
		}
		bootstrapPeers[i] = types.PeerInfo{
			ID:    types.PeerID(bootNode.ID()),
			Addrs: multiaddrs,
		}
	}

	// 4. 创建 Bootstrap 服务
	bootstrapConfig := &bootstrap.Config{
		Peers:    bootstrapPeers,
		Timeout:  10 * time.Second,
		MinPeers: 0, // 不要求最小连接数
		Enabled:  true,
	}

	bootstrapService, err := bootstrap.New(clientNode.Host(), bootstrapConfig)
	require.NoError(t, err, "创建 Bootstrap 服务失败")

	// 5. 启动 Bootstrap 服务
	err = bootstrapService.Start(ctx)
	require.NoError(t, err, "启动 Bootstrap 服务失败")

	// 6. 执行引导流程
	err = bootstrapService.Bootstrap(ctx)
	require.NoError(t, err, "引导流程失败")

	// 7. 验证连接数
	testutil.Eventually(t, 10*time.Second, func() bool {
		return clientNode.ConnectionCount() >= len(bootNodes)
	}, "客户端应该连接到所有引导节点")

	t.Logf("✅ Bootstrap 连接测试通过: 连接数=%d", clientNode.ConnectionCount())
}

// TestBootstrap_MinPeersRequirement 测试最小连接数要求验证
//
// 验证:
//   - 当成功连接数不足 MinPeers 时，Bootstrap 应该返回错误
func TestBootstrap_MinPeersRequirement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动 3 个引导节点
	bootNodes := make([]*dep2p.Node, 3)
	for i := 0; i < 3; i++ {
		bootNodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			WithPreset("minimal").
			Start()
	}

	// 2. 启动客户端节点
	clientNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 3. 构建引导节点列表（包含一个无效地址的节点）
	bootstrapPeers := make([]types.PeerInfo, len(bootNodes))
	for i, bootNode := range bootNodes {
		addrs := bootNode.ListenAddrs()
		multiaddrs := make([]types.Multiaddr, len(addrs))
		for j, addr := range addrs {
			multiaddr, err := types.ParseMultiaddr(addr)
			require.NoError(t, err)
			multiaddrs[j] = multiaddr
		}
		bootstrapPeers[i] = types.PeerInfo{
			ID:    types.PeerID(bootNode.ID()),
			Addrs: multiaddrs,
		}
	}

	// 添加一个无效的引导节点（无法连接）
	invalidAddr, _ := types.ParseMultiaddr("/ip4/127.0.0.1/udp/99999/quic-v1")
	bootstrapPeers = append(bootstrapPeers, types.PeerInfo{
		ID:    types.PeerID("invalid-peer-id"),
		Addrs: []types.Multiaddr{invalidAddr},
	})

	// 4. 创建 Bootstrap 服务，要求至少 4 个连接（但只有 3 个有效）
	bootstrapConfig := &bootstrap.Config{
		Peers:    bootstrapPeers,
		Timeout:  5 * time.Second, // 较短的超时
		MinPeers: 4,                // 要求至少 4 个连接
		Enabled:  true,
	}

	bootstrapService, err := bootstrap.New(clientNode.Host(), bootstrapConfig)
	require.NoError(t, err)

	err = bootstrapService.Start(ctx)
	require.NoError(t, err)

	// 5. 执行引导流程，应该失败（因为只有 3 个有效节点，但要求 4 个）
	err = bootstrapService.Bootstrap(ctx)
	assert.Error(t, err, "应该返回错误，因为连接数不足 MinPeers")
	assert.Contains(t, err.Error(), "minimum peers", "错误信息应该包含 minimum peers")

	t.Logf("✅ MinPeers 要求验证通过: %v", err)
}

// TestBootstrap_FailureRetry 测试连接失败重试机制
//
// 验证:
//   - 部分引导节点连接失败时，仍能成功连接其他节点
func TestBootstrap_FailureRetry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动 2 个有效的引导节点
	validBootNodes := make([]*dep2p.Node, 2)
	for i := 0; i < 2; i++ {
		validBootNodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			WithPreset("minimal").
			Start()
	}

	// 2. 启动客户端节点
	clientNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 3. 构建引导节点列表（包含有效和无效节点）
	bootstrapPeers := make([]types.PeerInfo, 0)

	// 添加有效节点
	for _, bootNode := range validBootNodes {
		addrs := bootNode.ListenAddrs()
		multiaddrs := make([]types.Multiaddr, len(addrs))
		for j, addr := range addrs {
			multiaddr, err := types.ParseMultiaddr(addr)
			require.NoError(t, err)
			multiaddrs[j] = multiaddr
		}
		bootstrapPeers = append(bootstrapPeers, types.PeerInfo{
			ID:    types.PeerID(bootNode.ID()),
			Addrs: multiaddrs,
		})
	}

	// 添加无效节点（无法连接）
	invalidAddr, _ := types.ParseMultiaddr("/ip4/127.0.0.1/udp/99999/quic-v1")
	bootstrapPeers = append(bootstrapPeers, types.PeerInfo{
		ID:    types.PeerID("invalid-peer-1"),
		Addrs: []types.Multiaddr{invalidAddr},
	})
	bootstrapPeers = append(bootstrapPeers, types.PeerInfo{
		ID:    types.PeerID("invalid-peer-2"),
		Addrs: []types.Multiaddr{invalidAddr},
	})

	// 4. 创建 Bootstrap 服务
	bootstrapConfig := &bootstrap.Config{
		Peers:    bootstrapPeers,
		Timeout:  5 * time.Second,
		MinPeers: 0, // 不要求最小连接数
		Enabled:  true,
	}

	bootstrapService, err := bootstrap.New(clientNode.Host(), bootstrapConfig)
	require.NoError(t, err)

	err = bootstrapService.Start(ctx)
	require.NoError(t, err)

	// 5. 执行引导流程，应该成功（即使部分节点失败）
	err = bootstrapService.Bootstrap(ctx)
	require.NoError(t, err, "应该成功连接有效节点")

	// 6. 验证只连接了有效节点
	testutil.Eventually(t, 10*time.Second, func() bool {
		return clientNode.ConnectionCount() == len(validBootNodes)
	}, "应该只连接到有效节点")

	t.Logf("✅ 失败重试测试通过: 连接数=%d (有效节点数=%d)",
		clientNode.ConnectionCount(), len(validBootNodes))
}

// TestBootstrap_ConcurrentConnect 测试并发连接多个引导节点
//
// 验证:
//   - 并发连接多个引导节点时，所有连接都能成功建立
func TestBootstrap_ConcurrentConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动 5 个引导节点（测试并发连接）
	bootNodeCount := 5
	bootNodes := make([]*dep2p.Node, bootNodeCount)
	for i := 0; i < bootNodeCount; i++ {
		bootNodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			WithPreset("minimal").
			Start()
		t.Logf("引导节点 %d: %s", i, bootNodes[i].ID()[:8])
	}

	// 2. 启动客户端节点
	clientNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 3. 构建引导节点列表
	bootstrapPeers := make([]types.PeerInfo, len(bootNodes))
	for i, bootNode := range bootNodes {
		addrs := bootNode.ListenAddrs()
		multiaddrs := make([]types.Multiaddr, len(addrs))
		for j, addr := range addrs {
			multiaddr, err := types.ParseMultiaddr(addr)
			require.NoError(t, err)
			multiaddrs[j] = multiaddr
		}
		bootstrapPeers[i] = types.PeerInfo{
			ID:    types.PeerID(bootNode.ID()),
			Addrs: multiaddrs,
		}
	}

	// 4. 创建 Bootstrap 服务
	bootstrapConfig := &bootstrap.Config{
		Peers:    bootstrapPeers,
		Timeout:  10 * time.Second,
		MinPeers: bootNodeCount, // 要求连接所有节点
		Enabled:  true,
	}

	bootstrapService, err := bootstrap.New(clientNode.Host(), bootstrapConfig)
	require.NoError(t, err)

	err = bootstrapService.Start(ctx)
	require.NoError(t, err)

	// 5. 执行引导流程（内部会并发连接）
	startTime := time.Now()
	err = bootstrapService.Bootstrap(ctx)
	duration := time.Since(startTime)

	require.NoError(t, err, "并发连接应该成功")

	// 6. 验证所有连接都建立
	testutil.Eventually(t, 10*time.Second, func() bool {
		return clientNode.ConnectionCount() == bootNodeCount
	}, "应该连接到所有引导节点")

	// 7. 验证并发性（如果串行连接，时间会明显更长）
	// 5 个节点并发连接应该在 5 秒内完成（每个节点超时 10 秒）
	assert.Less(t, duration, 15*time.Second, "并发连接应该在合理时间内完成")

	t.Logf("✅ 并发连接测试通过: 连接数=%d, 耗时=%v",
		clientNode.ConnectionCount(), duration)
}
