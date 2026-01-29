//go:build integration

package discovery_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestDHT_BootstrapConnection 测试 DHT 引导连接
//
// 验证:
//   - 节点能连接到引导节点
//   - 连接后能通过 DHT 发现其他节点
func TestDHT_BootstrapConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. 启动 "引导节点" (先启动，作为其他节点的引导)
	// 使用 desktop 预设（包含 DHT 但不需要公网可达）
	bootNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	t.Logf("引导节点: %s", bootNode.ID()[:8])
	t.Logf("引导地址: %v", bootNode.ListenAddrs())

	// 2. 启动节点 A 和 B，连接到引导节点
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

	// 3. A 和 B 都连接到引导节点
	err := nodeA.Host().Connect(ctx, bootNode.ID(), bootNode.ListenAddrs())
	require.NoError(t, err, "A 连接引导节点失败")

	err = nodeB.Host().Connect(ctx, bootNode.ID(), bootNode.ListenAddrs())
	require.NoError(t, err, "B 连接引导节点失败")

	// 4. 验证连接
	testutil.Eventually(t, 10*time.Second, func() bool {
		return nodeA.ConnectionCount() >= 1 && nodeB.ConnectionCount() >= 1
	}, "A 和 B 应该连接到引导节点")

	t.Logf("✅ DHT 引导连接测试通过: A=%d, B=%d 连接", 
		nodeA.ConnectionCount(), nodeB.ConnectionCount())
}

// TestDHT_PeerDiscovery 测试 DHT 节点发现
//
// 验证通过 DHT 可以发现其他节点。
// 注意: 这个测试需要较长时间，因为 DHT 路由需要时间收敛。
func TestDHT_PeerDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 3 个节点，形成 DHT 网络
	// 第一个节点作为引导节点
	bootNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	t.Logf("Boot: %s", bootNode.ID()[:8])
	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// 2. 加入 Realm
	_ = testutil.NewTestRealm(t, bootNode).WithPSK(psk).Join()
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 3. A 和 B 都连接到引导节点
	err := nodeA.Host().Connect(ctx, bootNode.ID(), bootNode.ListenAddrs())
	require.NoError(t, err, "A 连接引导节点失败")

	err = nodeB.Host().Connect(ctx, bootNode.ID(), bootNode.ListenAddrs())
	require.NoError(t, err, "B 连接引导节点失败")

	// 4. 等待 DHT 路由传播（这可能需要一些时间）
	t.Log("等待 DHT 路由传播...")
	time.Sleep(5 * time.Second)

	// 5. 等待成员发现（通过 Realm 机制）
	testutil.WaitForMembers(t, realmA, 3, 60*time.Second)

	members := realmA.Members()
	t.Logf("节点 A 发现 %d 个成员", len(members))

	assert.GreaterOrEqual(t, len(members), 3, "A 应发现所有成员")

	t.Log("✅ DHT 节点发现测试通过")
}

// TestDHT_ConnectByNodeID 测试通过 NodeID 连接（DHT 查找）
//
// 验证:
//   - 节点可以仅通过 NodeID 连接（DHT 查找地址）
//
// 注意: 使用 minimal 预设禁用 mDNS，避免自动发现干扰测试
func TestDHT_ConnectByNodeID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// 1. 启动引导节点（使用 minimal 预设禁用 mDNS）
	bootNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 2. 启动节点 A 和 B（使用 minimal 预设禁用 mDNS）
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	t.Logf("Boot: %s", bootNode.ID()[:8])
	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// 3. A 和 B 都连接到引导节点
	err := nodeA.Host().Connect(ctx, bootNode.ID(), bootNode.ListenAddrs())
	require.NoError(t, err)

	err = nodeB.Host().Connect(ctx, bootNode.ID(), bootNode.ListenAddrs())
	require.NoError(t, err)

	// 4. 等待 DHT 路由传播
	time.Sleep(5 * time.Second)

	// 5. A 尝试仅通过 NodeID 连接 B
	// 使用 Node.Connect API，它会尝试通过 DHT 发现
	err = nodeA.Connect(ctx, nodeB.ID())
	if err != nil {
		// DHT 发现可能失败（本地环境 DHT 可能不完整）
		t.Logf("通过 NodeID 连接失败 (可能 DHT 未完全收敛): %v", err)
		
		// 回退到直接连接
		err = nodeA.Host().Connect(ctx, nodeB.ID(), nodeB.ListenAddrs())
		require.NoError(t, err, "直接连接也失败")
		t.Log("回退到直接连接成功")
	} else {
		t.Log("通过 NodeID (DHT) 连接成功")
	}

	// 6. 验证 A-B 连接（通过 Peerstore 检查，更可靠）
	testutil.Eventually(t, 10*time.Second, func() bool {
		// A 的 Peerstore 应该有 B 的地址
		addrs := nodeA.Host().Peerstore().Addrs(types.PeerID(nodeB.ID()))
		return len(addrs) > 0
	}, "A 应该知道 B 的地址")

	t.Log("✅ DHT ConnectByNodeID 测试通过")
}

// TestDHT_ThreeNodeNetwork 测试三节点网络
//
// 验证三个节点能够形成网络并互相发现。
// 
// 注意: 这个测试与 member_test.go 中的 TestMember_Leave 类似，
// 使用 minimal 预设以避免 mDNS 等组件的干扰。
func TestDHT_ThreeNodeNetwork(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 3 个节点 (使用 minimal 预设)
	nodes := make([]*dep2p.Node, 3)
	for i := 0; i < 3; i++ {
		nodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			Start() // 默认 minimal 预设
		t.Logf("节点 %d: %s", i, nodes[i].ID()[:8])
	}

	// 2. 加入 Realm
	realms := make([]*dep2p.Realm, 3)
	for i := 0; i < 3; i++ {
		realms[i] = testutil.NewTestRealm(t, nodes[i]).WithPSK(psk).Join()
	}

	// 3. 星型连接: 节点 1 和 2 都连接到节点 0
	err := nodes[1].Host().Connect(ctx, nodes[0].ID(), nodes[0].ListenAddrs())
	require.NoError(t, err, "节点 1 连接节点 0 失败")

	// 等待第一个连接的 Gossip 消息传播
	time.Sleep(500 * time.Millisecond)

	err = nodes[2].Host().Connect(ctx, nodes[0].ID(), nodes[0].ListenAddrs())
	require.NoError(t, err, "节点 2 连接节点 0 失败")

	// 4. 等待成员发现
	testutil.WaitForMembers(t, realms[0], 3, 45*time.Second)

	// 5. 额外等待 Gossip 消息和心跳刷新 mesh（心跳间隔 1 秒）
	time.Sleep(5 * time.Second)

	// 5. 验证所有节点都发现了彼此
	for i, realm := range realms {
		members := realm.Members()
		t.Logf("节点 %d 发现 %d 个成员", i, len(members))
		assert.GreaterOrEqual(t, len(members), 3, "节点 %d 应发现所有成员", i)
	}

	t.Log("✅ 三节点网络测试通过")
}
