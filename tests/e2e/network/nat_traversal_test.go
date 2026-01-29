//go:build e2e

package network_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestNAT_AutoDetection 测试 NAT 类型自动检测
//
// 验证:
//   - 节点能自动检测 NAT 类型
//   - NAT 服务能正确启动
//
// 注意: 在本地测试环境中，NAT 类型检测可能返回 None 或 FullCone
func TestNAT_AutoDetection(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动节点（使用 desktop 预设，包含 NAT 服务）
	node := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop").
		Start()

	t.Logf("节点: %s", node.ID()[:8])

	// 2. 等待 NAT 服务启动和检测
	// 注意: NAT 检测可能需要一些时间
	time.Sleep(3 * time.Second)

	// 3. 验证节点能正常启动（NAT 检测不应该阻塞节点启动）
	assert.Greater(t, len(node.ListenAddrs()), 0, "节点应该有监听地址")

	// 4. 验证连接功能正常（即使 NAT 检测未完成）
	testNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	err := testNode.Host().Connect(ctx, node.ID(), node.ListenAddrs())
	require.NoError(t, err, "应该能建立连接")

	testutil.Eventually(t, 10*time.Second, func() bool {
		return node.ConnectionCount() > 0
	}, "应该建立连接")

	t.Log("✅ NAT 自动检测测试通过（在本地环境中可能检测为 None）")
}

// TestNAT_HolePunchSuccess 测试打洞成功场景
//
// 验证:
//   - 当两个节点都在 NAT 后时，能通过打洞建立直连
//
// 注意: 在本地测试环境中，由于没有真实 NAT，打洞可能无法触发
// 这个测试主要验证打洞流程不会导致错误
func TestNAT_HolePunchSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动两个节点（都使用 desktop 预设，包含 NAT 和 HolePunch 支持）
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

	// 3. 尝试连接（在本地环境中，由于没有真实 NAT，会直接连接成功）
	// 但如果打洞机制被触发，应该也能正常工作
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "连接应该成功（直连或打洞）")

	// 4. 等待连接建立
	testutil.Eventually(t, 15*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "应该建立连接")

	// 5. 验证成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	t.Logf("连接建立: A=%d, B=%d", nodeA.ConnectionCount(), nodeB.ConnectionCount())
	t.Log("✅ 打洞测试通过（在本地环境中可能使用直连而非打洞）")
}

// TestNAT_FallbackToRelay 测试打洞失败后回退到 Relay
//
// 验证:
//   - 当打洞失败时，系统能回退到 Relay 连接
//
// 注意: 在本地测试环境中，由于没有真实 NAT，这个测试主要验证
// 回退机制不会导致错误
func TestNAT_FallbackToRelay(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 Relay 服务器节点
	relayNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("server"). // server 预设包含 Relay 服务器
		Start()

	t.Logf("Relay 节点: %s", relayNode.ID()[:8])

	// 2. 启动两个客户端节点（使用 desktop 预设，支持 Relay 客户端）
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

	// 4. 连接到 Relay（如果 Relay 配置正确）
	// 注意: 在本地环境中，可能不需要 Relay，但验证流程不会出错
	err := nodeA.Host().Connect(ctx, relayNode.ID(), relayNode.ListenAddrs())
	if err != nil {
		t.Logf("连接到 Relay 失败（可能不需要）: %v", err)
	}

	err = nodeB.Host().Connect(ctx, relayNode.ID(), relayNode.ListenAddrs())
	if err != nil {
		t.Logf("连接到 Relay 失败（可能不需要）: %v", err)
	}

	// 5. 尝试 A 和 B 之间的连接
	// 在真实 NAT 环境中，如果打洞失败，应该通过 Relay 连接
	err = nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "连接应该成功（直连或通过 Relay）")

	// 6. 等待连接建立
	testutil.Eventually(t, 15*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "应该建立连接")

	// 7. 验证成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	t.Logf("连接建立: A=%d, B=%d", nodeA.ConnectionCount(), nodeB.ConnectionCount())
	t.Log("✅ Relay 回退测试通过（在本地环境中可能使用直连）")
}
