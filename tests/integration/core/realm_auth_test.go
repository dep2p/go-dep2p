//go:build integration

package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestRealm_AuthAndMemberDiscovery 测试 Realm 认证和成员发现
//
// 验证完整的 Realm 加入和认证流程:
//   1. 两节点加入同一 Realm (相同 PSK)
//   2. 建立连接 (触发 EvtPeerConnected)
//   3. PSK 认证自动进行
//   4. 认证成功后互相添加为成员
//   5. Members() 返回正确的成员列表
func TestRealm_AuthAndMemberDiscovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// ========================================
	// 阶段 1: 启动节点
	// ========================================
	t.Log("=== 阶段 1: 启动节点 ===")

	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	t.Logf("节点 A: %s", nodeA.ID()[:8])
	t.Logf("节点 B: %s", nodeB.ID()[:8])

	// ========================================
	// 阶段 2: 加入 Realm (先加入，再连接，确保事件订阅)
	// ========================================
	t.Log("=== 阶段 2: 加入 Realm ===")

	realmA := testutil.NewTestRealm(t, nodeA).
		WithPSK(psk).
		Join()
	realmB := testutil.NewTestRealm(t, nodeB).
		WithPSK(psk).
		Join()

	t.Logf("节点 A 加入 Realm: %s", realmA.ID()[:8])
	t.Logf("节点 B 加入 Realm: %s", realmB.ID()[:8])

	// 验证在同一 Realm
	assert.Equal(t, realmA.ID(), realmB.ID(), "两节点应在同一 Realm")

	// 初始成员数应为 1 (只有自己)
	assert.Equal(t, 1, len(realmA.Members()), "初始成员数应为 1")
	assert.Equal(t, 1, len(realmB.Members()), "初始成员数应为 1")

	// ========================================
	// 阶段 3: 建立连接 (触发认证)
	// ========================================
	t.Log("=== 阶段 3: 建立连接 ===")

	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "节点 B 连接到节点 A 失败")

	// 等待连接建立
	testutil.Eventually(t, 5*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "连接应该建立")

	// ========================================
	// 阶段 4: 等待成员发现 (PSK 认证)
	// ========================================
	t.Log("=== 阶段 4: 等待成员发现 ===")

	// 等待认证完成 (最多 30 秒)
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	membersA := realmA.Members()
	membersB := realmB.Members()

	t.Logf("认证成功! Realm A 成员: %v", membersA)
	t.Logf("认证成功! Realm B 成员: %v", membersB)

	// 验证成员数量
	assert.GreaterOrEqual(t, len(membersA), 2, "节点 A 应至少有 2 个成员")
	assert.GreaterOrEqual(t, len(membersB), 2, "节点 B 应至少有 2 个成员")

	// 验证双方都在对方的成员列表中
	assert.Contains(t, membersA, nodeB.ID(), "节点 A 的成员列表应包含节点 B")
	assert.Contains(t, membersB, nodeA.ID(), "节点 B 的成员列表应包含节点 A")

	t.Log("✅ Realm 认证和成员发现测试通过")
}

// TestRealm_MultiNodeAuth 测试多节点 Realm 认证
//
// 验证 3 个节点加入同一 Realm 后，都能互相发现。
func TestRealm_MultiNodeAuth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 3 个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeC := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 2. 加入 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()
	realmC := testutil.NewTestRealm(t, nodeC).WithPSK(psk).Join()

	// 3. 建立连接 (B 和 C 都连接到 A)
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	nodeC.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())

	// 4. 等待所有成员发现
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmB, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmC, 3, 45*time.Second)

	// 5. 验证所有节点都在成员列表中
	membersA := realmA.Members()
	assert.Contains(t, membersA, nodeB.ID(), "A 应包含 B")
	assert.Contains(t, membersA, nodeC.ID(), "A 应包含 C")

	membersB := realmB.Members()
	assert.Contains(t, membersB, nodeA.ID(), "B 应包含 A")
	assert.Contains(t, membersB, nodeC.ID(), "B 应包含 C")

	membersC := realmC.Members()
	assert.Contains(t, membersC, nodeA.ID(), "C 应包含 A")
	assert.Contains(t, membersC, nodeB.ID(), "C 应包含 B")

	t.Logf("✅ 多节点认证测试通过: A=%d, B=%d, C=%d",
		len(membersA), len(membersB), len(membersC))
}

// TestRealm_DifferentPSK 测试不同 PSK 的节点不能互相认证
//
// 验证使用不同 PSK 的节点会加入不同的 Realm。
func TestRealm_DifferentPSK(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// PSK 至少需要 16 字节
	psk1 := "realm-1-secret-key-long-enough"
	psk2 := "realm-2-secret-key-long-enough"

	// 1. 启动节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 2. 加入不同 PSK 的 Realm
	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk1).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk2).Join()

	// 3. 验证是不同的 Realm
	assert.NotEqual(t, realmA.ID(), realmB.ID(), "不同 PSK 应产生不同 Realm")

	// 4. 建立连接
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.Eventually(t, 5*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 && nodeB.ConnectionCount() > 0
	}, "连接应该建立")

	// 5. 等待一段时间，验证不会互相认证
	time.Sleep(5 * time.Second)

	// 6. 验证成员数仍为 1 (只有自己)
	assert.Equal(t, 1, len(realmA.Members()), "Realm A 成员数应为 1")
	assert.Equal(t, 1, len(realmB.Members()), "Realm B 成员数应为 1")

	t.Log("✅ 不同 PSK 隔离测试通过")
}
