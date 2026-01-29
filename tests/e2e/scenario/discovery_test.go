//go:build e2e

package scenario_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestE2E_MDNS_Discovery 测试 mDNS 自动发现
//
// 验证:
//   - 节点启动后能通过 mDNS 自动发现
//   - 发现后自动连接
//   - 连接后能正常通信
func TestE2E_MDNS_Discovery(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	_, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点 (使用 desktop 预设，包含 mDNS)
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

	// 3. 等待 mDNS 发现和自动连接
	// 注意: mDNS 发现需要时间，可能需要 10-30 秒
	t.Log("等待 mDNS 发现...")

	testutil.Eventually(t, 60*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0 || nodeB.ConnectionCount() > 0
	}, "mDNS 应该发现并连接")

	// 4. 等待成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	membersA := realmA.Members()
	membersB := realmB.Members()

	assert.GreaterOrEqual(t, len(membersA), 2, "节点 A 应发现节点 B")
	assert.GreaterOrEqual(t, len(membersB), 2, "节点 B 应发现节点 A")

	t.Logf("✅ mDNS 发现测试通过: A=%d, B=%d", len(membersA), len(membersB))
}

// TestE2E_ManualConnect 测试手动连接场景
//
// 验证即使没有 mDNS，手动连接也能正常工作。
func TestE2E_ManualConnect(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点 (minimal 预设，不包含 mDNS)
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 2. 手动连接
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "手动连接失败")

	// 3. 等待成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	membersA := realmA.Members()
	assert.GreaterOrEqual(t, len(membersA), 2, "手动连接后应发现成员")

	t.Log("✅ 手动连接测试通过")
}
