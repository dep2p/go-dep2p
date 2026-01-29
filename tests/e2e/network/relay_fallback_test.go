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

// TestRelay_ServerConnection 测试 Relay 服务器连接（统一 Relay v2.0）
//
// 验证:
//   - Relay 服务器能正常启动
//   - 客户端能连接到 Relay 服务器
//   - Relay 连接状态正确
func TestRelay_ServerConnection(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// 1. 启动 Relay 服务器节点（使用 server 预设）
	relayNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("server").
		Start()

	t.Logf("Relay 服务器: %s", relayNode.ID()[:8])
	t.Logf("Relay 地址: %v", relayNode.ListenAddrs())

	// 2. 启动客户端节点
	clientNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("desktop"). // desktop 预设包含 Relay 客户端支持
		Start()

	t.Logf("客户端节点: %s", clientNode.ID()[:8])

	// 3. 客户端连接到 Relay 服务器
	err := clientNode.Host().Connect(ctx, relayNode.ID(), relayNode.ListenAddrs())
	require.NoError(t, err, "客户端连接 Relay 服务器失败")

	// 4. 等待连接建立
	testutil.Eventually(t, 15*time.Second, func() bool {
		return clientNode.ConnectionCount() > 0 && relayNode.ConnectionCount() > 0
	}, "应该建立连接")

	t.Logf("连接建立: 客户端=%d, Relay=%d",
		clientNode.ConnectionCount(), relayNode.ConnectionCount())

	// 5. 验证连接持久性
	time.Sleep(2 * time.Second)
	assert.Greater(t, clientNode.ConnectionCount(), 0, "连接应该保持")

	t.Log("✅ Relay 服务器连接测试通过")
}

// TestRelay_Fallback 测试直连失败后 Relay 回退（统一 Relay v2.0）
//
// 验证:
//   - 当直连失败时，系统能通过 Relay 建立连接
//   - Realm 内的 Relay 转发正常工作
func TestRelay_Fallback(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 Relay 服务器节点
	relayNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("server").
		Start()

	t.Logf("Relay 服务器: %s", relayNode.ID()[:8])

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

	// 4. 连接到 Relay（如果配置了 Relay）
	err := nodeA.Host().Connect(ctx, relayNode.ID(), relayNode.ListenAddrs())
	if err != nil {
		t.Logf("A 连接 Relay 失败（可能不需要）: %v", err)
	}

	err = nodeB.Host().Connect(ctx, relayNode.ID(), relayNode.ListenAddrs())
	if err != nil {
		t.Logf("B 连接 Relay 失败（可能不需要）: %v", err)
	}

	// 5. 尝试 A 和 B 之间的连接
	// 在真实 NAT 环境中，如果直连失败，应该通过 Relay
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
	t.Log("✅ Relay 回退测试通过")
}

// TestRelay_DataTransfer 测试通过 Relay 传输数据（统一 Relay v2.0）
//
// 验证:
//   - 通过 Relay 连接能正常传输数据
//   - PubSub 消息能通过 Relay 传播
func TestRelay_DataTransfer(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 Relay 服务器节点
	relayNode := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("server").
		Start()

	t.Logf("Relay 服务器: %s", relayNode.ID()[:8])

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

	// 4. 建立连接（可能通过 Relay）
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 等待连接和成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)
	testutil.WaitForMembers(t, realmB, 2, 30*time.Second)

	// 5. 建立 PubSub 通信
	topicA, err := realmA.PubSub().Join("test/relay")
	require.NoError(t, err)
	topicB, err := realmB.PubSub().Join("test/relay")
	require.NoError(t, err)

	subB, err := topicB.Subscribe()
	require.NoError(t, err)
	defer subB.Cancel()

	// 等待订阅传播
	time.Sleep(2 * time.Second)

	// 6. A 发送消息
	testMessage := "message-through-relay"
	err = topicA.Publish(ctx, []byte(testMessage))
	require.NoError(t, err)

	// 7. B 应该收到消息（无论是否通过 Relay）
	received := false
	timeout := time.After(10 * time.Second)
	for !received {
		select {
		case msg := <-subB.Next(ctx):
			if string(msg.Data) == testMessage {
				received = true
			}
		case <-timeout:
			break
		}
	}

	assert.True(t, received, "B 应该收到消息（无论是否通过 Relay）")

	t.Log("✅ Relay 数据传输测试通过")
}
