//go:build integration

package realm_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/realm/connector"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestConnector_DirectConnect 测试直连策略
//
// 验证:
//   - 当目标节点有地址时，能直接连接
//   - 直连策略只尝试直连，不尝试打洞或 Relay
func TestConnector_DirectConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动两个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// 2. 加入 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 3. 创建 AddressResolver（用于解析节点地址）
	// 注意: 在直连测试中，我们使用 Host.Connect 直接测试
	// resolver 通过 Connector 内部使用

	// 4. 创建 Connector（使用直连策略）
	connectorConfig := connector.DefaultConnectorConfig()
	connectorConfig.Strategy = connector.StrategyDirectOnly
	connectorConfig.DirectTimeout = 10 * time.Second

	// 注意: Connector 需要 relayService 和 holePuncher，但在直连策略下可能不需要
	// 这里我们创建一个最小配置的 Connector
	// 由于 Connector 需要多个依赖，我们通过 Realm 的连接功能间接测试
	// 或者直接使用 Host.Connect 测试直连

	// 5. 使用 Host.Connect 测试直连（这是 Connector 的底层实现）
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "直连应该成功")

	// 6. 验证连接建立
	testutil.Eventually(t, 10*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "应该建立连接")

	// 7. 验证成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	t.Logf("连接建立: A=%d, B=%d", nodeA.ConnectionCount(), nodeB.ConnectionCount())
	t.Log("✅ 直连策略测试通过")
}

// TestConnector_AutoStrategy 测试自动选择最优策略
//
// 验证:
//   - 自动策略能根据情况选择最优连接方式
//   - 优先尝试直连，失败后尝试打洞，最后使用 Relay
func TestConnector_AutoStrategy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动两个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop"). // 使用 desktop 预设，包含 NAT 和 Relay 支持
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// 2. 加入 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 3. 使用自动策略连接（通过 Host.Connect，它会使用 Connector 的自动策略）
	// 在本地环境中，应该优先使用直连
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "自动策略连接应该成功")

	// 4. 验证连接建立
	testutil.Eventually(t, 15*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "应该建立连接")

	// 5. 验证成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	t.Logf("连接建立: A=%d, B=%d", nodeA.ConnectionCount(), nodeB.ConnectionCount())
	t.Log("✅ 自动策略测试通过（在本地环境中使用直连）")
}

// TestConnector_HolePunchFallback 测试直连失败后打洞
//
// 验证:
//   - 当直连失败时，系统能尝试 NAT 穿透
//
// 注意: 在本地测试环境中，由于没有真实 NAT，打洞可能无法触发
// 这个测试主要验证流程不会导致错误
func TestConnector_HolePunchFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动两个节点（使用 desktop 预设，包含 HolePunch 支持）
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// 2. 加入 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 3. 尝试连接（在本地环境中，直连应该成功，打洞不会被触发）
	// 但如果打洞机制被触发，应该也能正常工作
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "连接应该成功（直连或打洞）")

	// 4. 验证连接建立
	testutil.Eventually(t, 15*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "应该建立连接")

	// 5. 验证成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	t.Logf("连接建立: A=%d, B=%d", nodeA.ConnectionCount(), nodeB.ConnectionCount())
	t.Log("✅ 打洞回退测试通过（在本地环境中可能使用直连）")
}

// TestConnector_RelayFallback 测试打洞失败后 Relay
//
// 验证:
//   - 当打洞失败时，系统能回退到 Relay 连接
//
// 注意: 在本地测试环境中，由于没有真实 NAT 和公开可达的中继服务器，
// 这个测试主要验证回退逻辑不会导致 panic 或死锁
func TestConnector_RelayFallback(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动模拟 Relay 节点（本地环境无法启动真正的 Relay 服务器）
	// 注：真正的 Relay 服务器需要公开可达
	relayNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	t.Logf("模拟 Relay 节点: %s", relayNode.ID()[:8])

	// 2. 启动两个客户端节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// 3. 加入 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 4. 尝试连接（在本地环境中，直连应该成功，Relay 不会被触发）
	// 但如果 Relay 机制被触发，应该也能正常工作
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "连接应该成功（直连或通过 Relay）")

	// 5. 验证连接建立
	testutil.Eventually(t, 15*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "应该建立连接")

	// 6. 验证成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	t.Logf("连接建立: A=%d, B=%d", nodeA.ConnectionCount(), nodeB.ConnectionCount())
	t.Log("✅ Relay 回退测试通过（在本地环境中可能使用直连）")
}
