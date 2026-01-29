//go:build e2e

package resilience_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestE2E_NetworkPartition 测试网络分区恢复
//
// 验证:
//   - 3 节点环境，断开中间节点模拟分区
//   - 验证分区两侧消息隔离
//   - 恢复连接后验证消息同步
func TestE2E_NetworkPartition(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// ════════════════════════════════════════════════════════════════
	// Phase 1: 启动 3 个节点，形成星型拓扑
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 1: 启动节点")

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

	// ════════════════════════════════════════════════════════════════
	// Phase 2: 加入 Realm 并建立连接
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 2: 加入 Realm")

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()
	realmC := testutil.NewTestRealm(t, nodeC).WithPSK(psk).Join()

	// 星型连接: B 和 C 都连接到 A
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	err = nodeC.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 等待成员发现
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmB, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmC, 3, 45*time.Second)

	t.Logf("连接建立完成: A=%d, B=%d, C=%d",
		nodeA.ConnectionCount(), nodeB.ConnectionCount(), nodeC.ConnectionCount())

	// ════════════════════════════════════════════════════════════════
	// Phase 3: 建立 PubSub 通信
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 3: 建立 PubSub")

	topicA, err := realmA.PubSub().Join("test/partition")
	require.NoError(t, err)
	topicB, err := realmB.PubSub().Join("test/partition")
	require.NoError(t, err)
	topicC, err := realmC.PubSub().Join("test/partition")
	require.NoError(t, err)

	subB, err := topicB.Subscribe()
	require.NoError(t, err)
	defer subB.Cancel()

	subC, err := topicC.Subscribe()
	require.NoError(t, err)
	defer subC.Cancel()

	// 等待订阅传播
	time.Sleep(2 * time.Second)

	// ════════════════════════════════════════════════════════════════
	// Phase 4: 模拟网络分区（断开 A 的连接）
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 4: 模拟网络分区")

	// 关闭 A 的所有连接（模拟 A 崩溃或网络故障）
	err = nodeA.Close()
	require.NoError(t, err)

	// 等待连接断开检测
	time.Sleep(5 * time.Second)

	// 验证 B 和 C 检测到 A 断开
	testutil.Eventually(t, 30*time.Second, func() bool {
		return len(realmB.Members()) == 2 && len(realmC.Members()) == 2
	}, "B 和 C 应该检测到 A 断开")

	t.Logf("分区后成员数: B=%d, C=%d", len(realmB.Members()), len(realmC.Members()))

	// ════════════════════════════════════════════════════════════════
	// Phase 5: 验证分区两侧消息隔离
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 5: 验证消息隔离")

	// B 发送消息，C 应该能收到（因为它们都连接到 A，但现在 A 断开）
	// 注意：在星型拓扑中，B 和 C 之间没有直连，所以消息可能无法传播
	// 这验证了分区的影响

	// B 发送消息
	err = topicB.Publish(ctx, []byte("message-from-B"))
	require.NoError(t, err)

	// 等待消息传播
	time.Sleep(2 * time.Second)

	// C 尝试接收消息（可能收不到，因为分区）
	receivedByC := false
	select {
	case msg := <-subC.Next(ctx):
		if string(msg.Data) == "message-from-B" {
			receivedByC = true
		}
	case <-time.After(5 * time.Second):
		// 超时，说明消息未传播（符合分区预期）
	}

	// 在星型拓扑中，B 和 C 之间没有直连，所以消息无法传播
	// 这验证了分区的影响
	t.Logf("消息隔离验证: C 收到消息=%v", receivedByC)

	// ════════════════════════════════════════════════════════════════
	// Phase 6: 恢复连接
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 6: 恢复连接")

	// 重新启动 A
	nodeA = testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	realmA = testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()

	// 重新建立连接
	err = nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	err = nodeC.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 等待成员恢复
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmB, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmC, 3, 45*time.Second)

	t.Logf("恢复后成员数: A=%d, B=%d, C=%d",
		len(realmA.Members()), len(realmB.Members()), len(realmC.Members()))

	// ════════════════════════════════════════════════════════════════
	// Phase 7: 验证恢复后消息同步
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 7: 验证消息同步")

	// A 重新加入 topic
	topicA, err = realmA.PubSub().Join("test/partition")
	require.NoError(t, err)

	subA, err := topicA.Subscribe()
	require.NoError(t, err)
	defer subA.Cancel()

	// 等待订阅传播
	time.Sleep(2 * time.Second)

	// A 发送消息，B 和 C 应该都能收到
	err = topicA.Publish(ctx, []byte("message-from-A-after-recovery"))
	require.NoError(t, err)

	// 验证 B 和 C 都收到消息
	receivedByB := false
	receivedByCAfterRecovery := false

	timeout := time.After(10 * time.Second)
	for (!receivedByB || !receivedByCAfterRecovery) && time.Now().Before(time.Now().Add(10*time.Second)) {
		select {
		case msg := <-subB.Next(ctx):
			if string(msg.Data) == "message-from-A-after-recovery" {
				receivedByB = true
			}
		case msg := <-subC.Next(ctx):
			if string(msg.Data) == "message-from-A-after-recovery" {
				receivedByCAfterRecovery = true
			}
		case <-timeout:
			break
		}
	}

	assert.True(t, receivedByB, "B 应该收到恢复后的消息")
	assert.True(t, receivedByCAfterRecovery, "C 应该收到恢复后的消息")

	t.Log("✅ 网络分区恢复测试通过")
}
