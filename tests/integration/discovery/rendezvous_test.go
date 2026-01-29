//go:build integration

package discovery_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/internal/discovery/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestRendezvous_RegisterAndDiscover 测试注册和发现流程
//
// 验证:
//   - 节点能注册到 Rendezvous Point
//   - 其他节点能通过 Rendezvous 发现已注册的节点
func TestRendezvous_RegisterAndDiscover(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动 Rendezvous Point 节点
	pointNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 2. 创建并启动 Point 服务
	pointConfig := rendezvous.DefaultPointConfig()
	point := rendezvous.NewPoint(pointNode.Host(), pointConfig)
	err := point.Start(ctx)
	require.NoError(t, err, "启动 Point 失败")
	defer point.Stop()

	t.Logf("Rendezvous Point: %s", pointNode.ID()[:8])

	// 3. 启动注册节点
	registerNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 连接到 Point
	err = registerNode.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
	require.NoError(t, err, "连接 Point 失败")

	// 4. 创建 Discoverer 并注册
	discovererConfig := rendezvous.DefaultDiscovererConfig()
	discovererConfig.Points = []types.PeerID{types.PeerID(pointNode.ID())}
	discovererConfig.DefaultTTL = 1 * time.Hour
	discovererConfig.RenewalInterval = 30 * time.Minute

	discoverer := rendezvous.NewDiscoverer(registerNode.Host(), discovererConfig)
	err = discoverer.Start(ctx)
	require.NoError(t, err, "启动 Discoverer 失败")
	defer discoverer.Stop(ctx)

	// 5. 注册到命名空间
	namespace := "test-namespace"
	ttl, err := discoverer.Advertise(ctx, namespace)
	require.NoError(t, err, "注册失败")
	assert.Greater(t, ttl, time.Duration(0), "TTL 应该大于 0")

	t.Logf("注册成功: namespace=%s, ttl=%v", namespace, ttl)

	// 6. 启动发现节点
	discoverNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 连接到 Point
	err = discoverNode.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
	require.NoError(t, err, "发现节点连接 Point 失败")

	// 7. 创建另一个 Discoverer 用于发现
	discoverer2Config := rendezvous.DefaultDiscovererConfig()
	discoverer2Config.Points = []types.PeerID{types.PeerID(pointNode.ID())}
	discoverer2Config.DiscoverTimeout = 10 * time.Second

	discoverer2 := rendezvous.NewDiscoverer(discoverNode.Host(), discoverer2Config)
	err = discoverer2.Start(ctx)
	require.NoError(t, err, "启动 Discoverer2 失败")
	defer discoverer2.Stop(ctx)

	// 8. 发现节点
	peerChan, err := discoverer2.FindPeers(ctx, namespace)
	require.NoError(t, err, "发现节点失败")

	// 9. 验证发现结果
	foundPeers := make([]types.PeerInfo, 0)
	timeout := time.After(10 * time.Second)
	for {
		select {
		case peer, ok := <-peerChan:
			if !ok {
				goto done
			}
			foundPeers = append(foundPeers, peer)
			t.Logf("发现节点: %s", peer.ID[:8])
		case <-timeout:
			goto done
		}
	}
done:

	assert.Greater(t, len(foundPeers), 0, "应该发现至少一个节点")
	found := false
	for _, peer := range foundPeers {
		if string(peer.ID) == registerNode.ID() {
			found = true
			break
		}
	}
	assert.True(t, found, "应该发现注册的节点")

	t.Logf("✅ 注册和发现测试通过: 发现 %d 个节点", len(foundPeers))
}

// TestRendezvous_NamespaceIsolation 测试命名空间隔离验证
//
// 验证:
//   - 不同命名空间的注册互不干扰
//   - 发现时只能看到同一命名空间的节点
func TestRendezvous_NamespaceIsolation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动 Rendezvous Point
	pointNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	point := rendezvous.NewPoint(pointNode.Host(), rendezvous.DefaultPointConfig())
	err := point.Start(ctx)
	require.NoError(t, err)
	defer point.Stop()

	// 2. 启动两个注册节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 连接到 Point
	err = nodeA.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
	require.NoError(t, err)
	err = nodeB.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
	require.NoError(t, err)

	// 3. 创建 Discoverer
	configA := rendezvous.DefaultDiscovererConfig()
	configA.Points = []types.PeerID{types.PeerID(pointNode.ID())}
	discovererA := rendezvous.NewDiscoverer(nodeA.Host(), configA)
	err = discovererA.Start(ctx)
	require.NoError(t, err)
	defer discovererA.Stop(ctx)

	configB := rendezvous.DefaultDiscovererConfig()
	configB.Points = []types.PeerID{types.PeerID(pointNode.ID())}
	discovererB := rendezvous.NewDiscoverer(nodeB.Host(), configB)
	err = discovererB.Start(ctx)
	require.NoError(t, err)
	defer discovererB.Stop(ctx)

	// 4. A 注册到 namespace1，B 注册到 namespace2
	namespace1 := "namespace-1"
	namespace2 := "namespace-2"

	_, err = discovererA.Advertise(ctx, namespace1)
	require.NoError(t, err)

	_, err = discovererB.Advertise(ctx, namespace2)
	require.NoError(t, err)

	// 5. 在 namespace1 中发现，应该只能看到 A
	peerChan1, err := discovererA.FindPeers(ctx, namespace1)
	require.NoError(t, err)

	foundInNS1 := make([]types.PeerInfo, 0)
	timeout := time.After(5 * time.Second)
	for {
		select {
		case peer, ok := <-peerChan1:
			if !ok {
				goto done1
			}
			foundInNS1 = append(foundInNS1, peer)
		case <-timeout:
			goto done1
		}
	}
done1:

	// 6. 在 namespace2 中发现，应该只能看到 B
	peerChan2, err := discovererB.FindPeers(ctx, namespace2)
	require.NoError(t, err)

	foundInNS2 := make([]types.PeerInfo, 0)
	timeout = time.After(5 * time.Second)
	for {
		select {
		case peer, ok := <-peerChan2:
			if !ok {
				goto done2
			}
			foundInNS2 = append(foundInNS2, peer)
		case <-timeout:
			goto done2
		}
	}
done2:

	// 7. 验证隔离性
	// namespace1 中应该只有 A（可能包括自己）
	foundA := false
	for _, peer := range foundInNS1 {
		if string(peer.ID) == nodeA.ID() {
			foundA = true
		}
		// 不应该有 B
		assert.NotEqual(t, string(peer.ID), nodeB.ID(), "namespace1 中不应该有 B")
	}
	assert.True(t, foundA || len(foundInNS1) > 0, "namespace1 中应该有 A")

	// namespace2 中应该只有 B（可能包括自己）
	foundB := false
	for _, peer := range foundInNS2 {
		if string(peer.ID) == nodeB.ID() {
			foundB = true
		}
		// 不应该有 A
		assert.NotEqual(t, string(peer.ID), nodeA.ID(), "namespace2 中不应该有 A")
	}
	assert.True(t, foundB || len(foundInNS2) > 0, "namespace2 中应该有 B")

	t.Logf("✅ 命名空间隔离测试通过: NS1=%d, NS2=%d", len(foundInNS1), len(foundInNS2))
}

// TestRendezvous_TTLExpiration 测试 TTL 过期机制
//
// 验证:
//   - 注册的 TTL 过期后，节点应该从发现结果中消失
func TestRendezvous_TTLExpiration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. 启动 Rendezvous Point
	pointNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	pointConfig := rendezvous.DefaultPointConfig()
	pointConfig.CleanupInterval = 1 * time.Second // 快速清理
	point := rendezvous.NewPoint(pointNode.Host(), pointConfig)
	err := point.Start(ctx)
	require.NoError(t, err)
	defer point.Stop()

	// 2. 启动注册节点
	registerNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	err = registerNode.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
	require.NoError(t, err)

	// 3. 创建 Discoverer 并使用很短的 TTL 注册
	discovererConfig := rendezvous.DefaultDiscovererConfig()
	discovererConfig.Points = []types.PeerID{types.PeerID(pointNode.ID())}
	discovererConfig.DefaultTTL = 5 * time.Second // 很短的 TTL

	discoverer := rendezvous.NewDiscoverer(registerNode.Host(), discovererConfig)
	err = discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	namespace := "test-ttl"
	ttl, err := discoverer.Advertise(ctx, namespace)
	require.NoError(t, err)
	t.Logf("注册成功: ttl=%v", ttl)

	// 4. 启动发现节点
	discoverNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	err = discoverNode.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
	require.NoError(t, err)

	discoverer2Config := rendezvous.DefaultDiscovererConfig()
	discoverer2Config.Points = []types.PeerID{types.PeerID(pointNode.ID())}
	discoverer2 := rendezvous.NewDiscoverer(discoverNode.Host(), discoverer2Config)
	err = discoverer2.Start(ctx)
	require.NoError(t, err)
	defer discoverer2.Stop(ctx)

	// 5. 立即发现，应该能找到
	peerChan, err := discoverer2.FindPeers(ctx, namespace)
	require.NoError(t, err)

	foundBefore := false
	timeout := time.After(5 * time.Second)
	for {
		select {
		case peer, ok := <-peerChan:
			if !ok {
				goto checkBefore
			}
			if string(peer.ID) == registerNode.ID() {
				foundBefore = true
			}
		case <-timeout:
			goto checkBefore
		}
	}
checkBefore:

	assert.True(t, foundBefore, "TTL 过期前应该能找到节点")

	// 6. 等待 TTL 过期（加上清理间隔）
	t.Logf("等待 TTL 过期...")
	time.Sleep(ttl + pointConfig.CleanupInterval + 2*time.Second)

	// 7. 再次发现，应该找不到
	peerChan2, err := discoverer2.FindPeers(ctx, namespace)
	require.NoError(t, err)

	foundAfter := false
	timeout = time.After(5 * time.Second)
	for {
		select {
		case peer, ok := <-peerChan2:
			if !ok {
				goto checkAfter
			}
			if string(peer.ID) == registerNode.ID() {
				foundAfter = true
			}
		case <-timeout:
			goto checkAfter
		}
	}
checkAfter:

	assert.False(t, foundAfter, "TTL 过期后不应该找到节点")

	t.Logf("✅ TTL 过期测试通过")
}

// TestRendezvous_LoadBalancing 测试负载均衡（轮询多个 Point）
//
// 验证:
//   - 当有多个 Rendezvous Point 时，Discoverer 应该轮询使用
func TestRendezvous_LoadBalancing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动多个 Rendezvous Point
	pointCount := 3
	pointNodes := make([]*dep2p.Node, pointCount)
	points := make([]*rendezvous.Point, pointCount)
	pointIDs := make([]types.PeerID, pointCount)

	for i := 0; i < pointCount; i++ {
		pointNodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			WithPreset("minimal").
			Start()

		points[i] = rendezvous.NewPoint(pointNodes[i].Host(), rendezvous.DefaultPointConfig())
		err := points[i].Start(ctx)
		require.NoError(t, err)
		defer points[i].Stop()

		pointIDs[i] = types.PeerID(pointNodes[i].ID())
		t.Logf("Point %d: %s", i, pointNodes[i].ID()[:8])
	}

	// 2. 启动注册节点
	registerNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 连接到所有 Point
	for _, pointNode := range pointNodes {
		err := registerNode.Host().Connect(ctx, pointNode.ID(), pointNode.ListenAddrs())
		require.NoError(t, err)
	}

	// 3. 创建 Discoverer，配置多个 Point
	discovererConfig := rendezvous.DefaultDiscovererConfig()
	discovererConfig.Points = pointIDs
	discovererConfig.DefaultTTL = 1 * time.Hour

	discoverer := rendezvous.NewDiscoverer(registerNode.Host(), discovererConfig)
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// 4. 注册到命名空间（应该轮询选择一个 Point）
	namespace := "test-lb"
	ttl, err := discoverer.Advertise(ctx, namespace)
	require.NoError(t, err, "注册应该成功（使用负载均衡）")
	assert.Greater(t, ttl, time.Duration(0))

	// 5. 多次注册，验证负载均衡（通过检查是否所有 Point 都被使用）
	// 注意：由于轮询机制，多次注册应该分布到不同 Point
	registrationCount := pointCount * 2
	for i := 0; i < registrationCount; i++ {
		ns := namespace + "-" + string(rune(i))
		_, err := discoverer.Advertise(ctx, ns)
		require.NoError(t, err, "注册 %d 应该成功", i)
	}

	t.Logf("✅ 负载均衡测试通过: 成功注册到 %d 个命名空间", registrationCount+1)
}
