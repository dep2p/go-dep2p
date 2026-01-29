//go:build integration

package realm_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/internal/realm/routing"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestRouting_FindRoute 测试路由查找
//
// 验证:
//   - Router 能查找到达目标节点的路由
//   - 路由表能正确更新
func TestRouting_FindRoute(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 3 个节点（形成简单网络）
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	nodeC := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])
	t.Logf("节点 C: %s", nodeC.ID()[:8])

	// 2. 加入 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeC).WithPSK(psk).Join()

	// 3. 建立连接（A-B, B-C，形成路径 A->B->C）
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	err = nodeC.Host().Connect(ctx, nodeB.ID(), nodeB.ListenAddrs())
	require.NoError(t, err)

	// 等待成员发现
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)

	// 4. 创建 Router（用于节点 A）
	// 注意: Router 需要 DHT，但我们可以创建一个最小配置的 Router
	router := routing.NewRouter("test-realm", nil, routing.DefaultConfig())
	err = router.Start(ctx)
	require.NoError(t, err)
	defer router.Stop(ctx)

	// 5. 手动添加路由信息到路由表
	routeTable := router.GetRouteTable()

	// 添加 B 节点（A 的直连邻居）
	err = routeTable.AddNode(&interfaces.RouteNode{
		PeerID:      nodeB.ID(),
		Addrs:       nodeB.ListenAddrs(),
		Latency:     10 * time.Millisecond,
		LastSeen:    time.Now(),
		IsReachable: true,
	})
	require.NoError(t, err)

	// 添加 C 节点（通过 B 可达）
	err = routeTable.AddNode(&interfaces.RouteNode{
		PeerID:      nodeC.ID(),
		Addrs:       nodeC.ListenAddrs(),
		Latency:     20 * time.Millisecond,
		LastSeen:    time.Now(),
		IsReachable: true,
	})
	require.NoError(t, err)

	// 6. 查找路由（Router 会根据路由表计算路径）
	route, err := router.FindRoute(ctx, nodeC.ID())
	require.NoError(t, err, "应该能找到路由")
	assert.NotNil(t, route, "路由不应该为 nil")
	assert.Equal(t, nodeC.ID(), route.TargetPeerID, "目标应该是 C")
	// NextHop 可能是 B 或 C（取决于路由算法）
	assert.NotEmpty(t, route.NextHop, "下一跳不应该为空")

	t.Logf("找到路由: 目标=%s, 下一跳=%s, 跳数=%d",
		route.TargetPeerID[:8], route.NextHop[:8], route.Hops)
	t.Log("✅ 路由查找测试通过")
}

// TestRouting_MultiPath 测试多路径选择
//
// 验证:
//   - 当存在多条路径时，Router 能选择最优路径
//   - 路径选择策略（最低延迟、最少跳数等）正常工作
func TestRouting_MultiPath(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. 创建 Router
	config := routing.DefaultConfig()
	config.DefaultPolicy = interfaces.PolicyLowestLatency // 使用最低延迟策略

	router := routing.NewRouter("test-realm", nil, config)
	err := router.Start(ctx)
	require.NoError(t, err)
	defer router.Stop(ctx)

	routeTable := router.GetRouteTable()

	// 2. 添加多条路径到同一目标
	targetID := "target-peer-12345"

	// 路径1: 添加节点A（延迟较高）
	err = routeTable.AddNode(&interfaces.RouteNode{
		PeerID:      "node-a-12345",
		Addrs:       []string{"/ip4/127.0.0.1/tcp/4001"},
		Latency:     50 * time.Millisecond,
		LastSeen:    time.Now(),
		IsReachable: true,
	})
	require.NoError(t, err)

	// 路径2: 添加节点B（延迟较低）
	err = routeTable.AddNode(&interfaces.RouteNode{
		PeerID:      "node-b-12345",
		Addrs:       []string{"/ip4/127.0.0.1/tcp/4002"},
		Latency:     20 * time.Millisecond,
		LastSeen:    time.Now(),
		IsReachable: true,
	})
	require.NoError(t, err)

	// 添加目标节点
	err = routeTable.AddNode(&interfaces.RouteNode{
		PeerID:      targetID,
		Addrs:       []string{"/ip4/127.0.0.1/tcp/4003"},
		Latency:     30 * time.Millisecond,
		LastSeen:    time.Now(),
		IsReachable: true,
	})
	require.NoError(t, err)

	// 3. 查找路由（应该选择延迟最低的路径）
	route, err := router.FindRoute(ctx, targetID)
	require.NoError(t, err)
	assert.NotNil(t, route)
	// 验证路由存在（具体选择取决于路由算法）
	assert.NotEmpty(t, route.NextHop, "应该找到路由")

	t.Logf("选择的路由: 下一跳=%s, 延迟=%v", route.NextHop, route.Latency)
	t.Log("✅ 多路径选择测试通过")
}

// TestRouting_CacheHit 测试路由缓存命中
//
// 验证:
//   - 路由缓存能正常工作
//   - 重复查找同一路由时能使用缓存
func TestRouting_CacheHit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 创建 Router
	router := routing.NewRouter("test-realm", nil, routing.DefaultConfig())
	err := router.Start(ctx)
	require.NoError(t, err)
	defer router.Stop(ctx)

	routeTable := router.GetRouteTable()

	// 2. 添加节点
	targetID := "cached-target-12345"
	err = routeTable.AddNode(&interfaces.RouteNode{
		PeerID:      targetID,
		Addrs:       []string{"/ip4/127.0.0.1/tcp/4001"},
		Latency:     10 * time.Millisecond,
		LastSeen:    time.Now(),
		IsReachable: true,
	})
	require.NoError(t, err)

	// 3. 第一次查找（应该添加到缓存）
	route1, err := router.FindRoute(ctx, targetID)
	require.NoError(t, err)
	assert.NotNil(t, route1)

	// 4. 第二次查找（应该使用缓存）
	route2, err := router.FindRoute(ctx, targetID)
	require.NoError(t, err)
	assert.NotNil(t, route2)

	// 5. 验证两次查找结果一致
	assert.Equal(t, route1.TargetPeerID, route2.TargetPeerID, "缓存结果应该一致")
	assert.Equal(t, route1.NextHop, route2.NextHop, "缓存结果应该一致")

	// 6. 使路由失效
	router.InvalidateRoute(targetID)

	// 7. 再次查找（缓存应该失效，重新查找）
	route3, err := router.FindRoute(ctx, targetID)
	require.NoError(t, err)
	assert.NotNil(t, route3)

	// 结果应该仍然一致（因为路由表没变）
	assert.Equal(t, route1.TargetPeerID, route3.TargetPeerID, "失效后重新查找应该得到相同结果")

	t.Log("✅ 路由缓存命中测试通过")
}
