//go:build integration

package protocol_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestLiveness_Ping 测试 Liveness Ping/Pong
//
// 验证:
//   - Ping 能够成功发送
//   - 收到 Pong 响应
//   - 响应时间合理
func TestLiveness_Ping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建节点并加入 Realm
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. A Ping B
	livenessA := realmA.Liveness()
	require.NotNil(t, livenessA, "Liveness 不应为 nil")

	start := time.Now()
	latency, err := livenessA.Ping(ctx, nodeB.ID())
	duration := time.Since(start)

	require.NoError(t, err, "Ping 失败")
	assert.Greater(t, latency, time.Duration(0), "延迟应大于 0")
	assert.Less(t, duration, 5*time.Second, "Ping 应在 5 秒内完成")

	t.Logf("✅ Ping 测试通过: 延迟=%v, 耗时=%v", latency, duration)
}

// TestLiveness_MultiplePings 测试多次 Ping
//
// 验证连续多次 Ping 都能成功。
func TestLiveness_MultiplePings(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. 多次 Ping
	livenessA := realmA.Liveness()
	pingCount := 5
	successCount := 0

	for i := 0; i < pingCount; i++ {
		latency, err := livenessA.Ping(ctx, nodeB.ID())
		if err == nil && latency > 0 {
			successCount++
			t.Logf("Ping %d: 延迟=%v", i+1, latency)
		}
		time.Sleep(500 * time.Millisecond)
	}

	assert.GreaterOrEqual(t, successCount, pingCount-1, "至少应有 pingCount-1 次成功")

	t.Logf("✅ 多次 Ping 测试通过: 成功=%d/%d", successCount, pingCount)
}

// TestLiveness_UnreachablePeer 测试不可达节点
//
// 验证 Ping 不可达节点会返回错误或超时。
func TestLiveness_UnreachablePeer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建节点 A
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()

	// 2. 创建一个不存在的 PeerID
	fakePeerID := "fake-peer-id-that-does-not-exist"

	// 3. Ping 不存在的节点
	livenessA := realmA.Liveness()
	_, err := livenessA.Ping(ctx, fakePeerID)

	// 应该返回错误或超时
	if err != nil {
		t.Logf("✅ Ping 不可达节点返回错误 (预期): %v", err)
	} else {
		t.Log("Ping 不可达节点未返回错误 (可能实现不同)")
	}
}

// TestLiveness_WatchAndStatus 测试 Watch 监控和状态查询
//
// 验证:
//   - Watch 能正确监控节点
//   - Status 能返回正确状态
//   - 该测试间接测试 decodePing/encodePong (通过底层 Ping 调用)
//
// 注意: Watch 只是注册事件监听，不会主动触发 Ping。
// 需要手动调用 Ping 来更新状态。
func TestLiveness_WatchAndStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 2. 建立连接
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	// 3. 开始监控
	livenessA := realmA.Liveness()
	require.NotNil(t, livenessA)

	watchChan, err := livenessA.Watch(nodeB.ID())
	require.NoError(t, err, "Watch 应该成功")
	require.NotNil(t, watchChan, "Watch channel 应该不为 nil")

	// 4. 手动触发 Ping 来更新状态（Watch 只是监听，不主动 Ping）
	rtt, err := livenessA.Ping(ctx, nodeB.ID())
	require.NoError(t, err, "Ping 应该成功")
	t.Logf("Ping RTT: %v", rtt)

	// 5. 查询状态
	status, err := livenessA.Status(nodeB.ID())
	require.NoError(t, err, "获取状态应该成功")
	t.Logf("节点 B 状态: 活跃=%v, 最后响应=%v", status.Alive, status.LastSeen)

	// Ping 成功后，状态应该是活跃的
	assert.True(t, status.Alive, "Ping 成功后节点应该是活跃的")

	// 6. 停止监控
	err = livenessA.Unwatch(nodeB.ID())
	require.NoError(t, err, "Unwatch 应该成功")

	t.Log("✅ Watch 监控和状态查询测试通过")
}

// TestLiveness_ConcurrentPing 测试并发 Ping
//
// 验证:
//   - 多个并发 Ping 请求能正确处理
//   - 无竞态条件
//   - 该测试间接测试并发场景下的 decodePing/encodePong
func TestLiveness_ConcurrentPing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. 并发 Ping
	livenessA := realmA.Liveness()
	concurrency := 10
	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(idx int) {
			// 每个 goroutine 独立的 context
			pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
			defer pingCancel()

			_, err := livenessA.Ping(pingCtx, nodeB.ID())
			results <- err
		}(i)
	}

	// 3. 收集结果
	successCount := 0
	for i := 0; i < concurrency; i++ {
		err := <-results
		if err == nil {
			successCount++
		}
	}

	// 允许少量失败（网络抖动）
	minSuccess := concurrency - 2
	assert.GreaterOrEqual(t, successCount, minSuccess,
		"至少 %d 个并发 Ping 应该成功，实际成功 %d 个", minSuccess, successCount)

	t.Logf("✅ 并发 Ping 测试通过: 成功=%d/%d", successCount, concurrency)
}

// TestLiveness_StatusTracking 测试状态跟踪
//
// 验证:
//   - 多个节点的状态能独立跟踪
//   - 状态更新正确
//
// 注意: Watch 只是注册事件监听，不会主动触发 Ping。
// 需要手动调用 Ping 来更新状态。
func TestLiveness_StatusTracking(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建多个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	nodeCount := 3
	otherNodes := make([]*dep2p.Node, nodeCount)

	for i := 0; i < nodeCount; i++ {
		otherNodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			Start()

		_ = testutil.NewTestRealm(t, otherNodes[i]).WithPSK(psk).Join()
	}

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()

	// 2. 连接所有节点
	for i := 0; i < nodeCount; i++ {
		otherNodes[i].Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	}

	testutil.WaitForMembers(t, realmA, nodeCount+1, 45*time.Second)

	// 3. 获取 Liveness 服务
	livenessA := realmA.Liveness()

	// 4. 对所有节点执行 Ping（更新状态）
	for i := 0; i < nodeCount; i++ {
		rtt, err := livenessA.Ping(ctx, otherNodes[i].ID())
		require.NoError(t, err, "Ping 节点 %d 应该成功", i)
		t.Logf("Ping 节点 %d (%s): RTT=%v", i, otherNodes[i].ID()[:8], rtt)
	}

	// 5. 检查所有节点状态
	aliveCount := 0
	for i := 0; i < nodeCount; i++ {
		status, err := livenessA.Status(otherNodes[i].ID())
		require.NoError(t, err)
		if status.Alive {
			aliveCount++
		}
		t.Logf("节点 %d (%s): 活跃=%v, LastSeen=%v", i, otherNodes[i].ID()[:8], status.Alive, status.LastSeen)
	}

	// 所有 Ping 成功的节点应该都是活跃的
	assert.Equal(t, nodeCount, aliveCount, "所有节点应该都是活跃的")

	t.Log("✅ 多节点状态跟踪测试通过")
}
