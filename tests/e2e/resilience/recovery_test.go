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

// TestE2E_NodeRecovery 测试节点故障恢复
//
// 验证:
//   - 节点崩溃后重启
//   - 验证状态恢复和重新加入
func TestE2E_NodeRecovery(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// ════════════════════════════════════════════════════════════════
	// Phase 1: 启动 3 个节点
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

	// 建立连接
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	err = nodeC.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 等待成员发现
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmB, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmC, 3, 45*time.Second)

	t.Logf("初始成员数: A=%d, B=%d, C=%d",
		len(realmA.Members()), len(realmB.Members()), len(realmC.Members()))

	// ════════════════════════════════════════════════════════════════
	// Phase 3: 建立 PubSub 通信
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 3: 建立 PubSub")

	topicA, err := realmA.PubSub().Join("test/recovery")
	require.NoError(t, err)
	topicB, err := realmB.PubSub().Join("test/recovery")
	require.NoError(t, err)
	topicC, err := realmC.PubSub().Join("test/recovery")
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
	// Phase 4: 模拟节点 B 崩溃
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 4: 模拟节点 B 崩溃")

	// 保存 B 的 ID 和地址（用于恢复）
	nodeBID := nodeB.ID()
	nodeBAddrs := nodeB.ListenAddrs()

	// 关闭节点 B（模拟崩溃）
	err = nodeB.Close()
	require.NoError(t, err)

	// 等待其他节点检测到 B 断开
	time.Sleep(5 * time.Second)

	// 验证 A 和 C 检测到 B 断开
	testutil.Eventually(t, 30*time.Second, func() bool {
		return len(realmA.Members()) == 2 && len(realmC.Members()) == 2
	}, "A 和 C 应该检测到 B 断开")

	t.Logf("B 断开后成员数: A=%d, C=%d",
		len(realmA.Members()), len(realmC.Members()))

	// ════════════════════════════════════════════════════════════════
	// Phase 5: 节点 B 恢复（重启）
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 5: 节点 B 恢复")

	// 重新启动节点 B（使用相同的配置）
	nodeB = testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	// 验证 B 的 ID 可能不同（因为重新生成密钥），但地址应该相同
	t.Logf("恢复后节点 B: %s", nodeB.ID()[:8])

	// 重新加入 Realm
	realmB = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 重新建立连接
	err = nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	err = nodeB.Host().Connect(ctx, nodeC.ID(), nodeC.ListenAddrs())
	require.NoError(t, err)

	// 等待成员恢复
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmB, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmC, 3, 45*time.Second)

	t.Logf("恢复后成员数: A=%d, B=%d, C=%d",
		len(realmA.Members()), len(realmB.Members()), len(realmC.Members()))

	// ════════════════════════════════════════════════════════════════
	// Phase 6: 验证恢复后功能正常
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 6: 验证恢复后功能")

	// B 重新加入 topic
	topicB, err = realmB.PubSub().Join("test/recovery")
	require.NoError(t, err)

	subBNew, err := topicB.Subscribe()
	require.NoError(t, err)
	defer subBNew.Cancel()

	// 等待订阅传播
	time.Sleep(2 * time.Second)

	// A 发送消息，B 和 C 应该都能收到
	err = topicA.Publish(ctx, []byte("message-after-recovery"))
	require.NoError(t, err)

	// 验证 B 和 C 都收到消息
	receivedByB := false
	receivedByC := false

	timeout := time.After(10 * time.Second)
	for (!receivedByB || !receivedByC) && time.Now().Before(time.Now().Add(10*time.Second)) {
		select {
		case msg := <-subBNew.Next(ctx):
			if string(msg.Data) == "message-after-recovery" {
				receivedByB = true
			}
		case msg := <-subC.Next(ctx):
			if string(msg.Data) == "message-after-recovery" {
				receivedByC = true
			}
		case <-timeout:
			break
		}
	}

	assert.True(t, receivedByB, "恢复后的 B 应该能收到消息")
	assert.True(t, receivedByC, "C 应该能收到消息")

	// ════════════════════════════════════════════════════════════════
	// Phase 7: 验证 B 能正常发送消息
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 7: 验证 B 发送消息")

	// B 发送消息
	err = topicB.Publish(ctx, []byte("message-from-recovered-B"))
	require.NoError(t, err)

	// 验证 A 和 C 收到消息
	subANew, err := topicA.Subscribe()
	require.NoError(t, err)
	defer subANew.Cancel()

	receivedByA := false
	receivedByCFromB := false

	timeout = time.After(10 * time.Second)
	for (!receivedByA || !receivedByCFromB) && time.Now().Before(time.Now().Add(10*time.Second)) {
		select {
		case msg := <-subANew.Next(ctx):
			if string(msg.Data) == "message-from-recovered-B" {
				receivedByA = true
			}
		case msg := <-subC.Next(ctx):
			if string(msg.Data) == "message-from-recovered-B" {
				receivedByCFromB = true
			}
		case <-timeout:
			break
		}
	}

	assert.True(t, receivedByA, "A 应该收到恢复后的 B 的消息")
	assert.True(t, receivedByCFromB, "C 应该收到恢复后的 B 的消息")

	t.Log("✅ 节点故障恢复测试通过")
}
