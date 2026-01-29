//go:build e2e

package scenario_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestE2E_ChatScenario_Full 完整聊天场景测试
//
// 模拟 chat-local 的完整流程:
//   Phase 1: 启动 3 个节点 (模拟 3 个终端)
//   Phase 2: 加入 Realm + 自动连接
//   Phase 3: 群聊测试 (PubSub)
//   Phase 4: 私聊测试 (Streams)
//   Phase 5: 成员变化测试 (节点退出)
func TestE2E_ChatScenario_Full(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	psk := "chat-local-e2e-test"

	// ════════════════════════════════════════════════════════════════
	// Phase 1: 启动 3 个节点
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 1: 启动节点")

	alice := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	bob := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()
	charlie := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		WithPreset("minimal").
		Start()

	t.Logf("Alice: %s", alice.ID()[:8])
	t.Logf("Bob: %s", bob.ID()[:8])
	t.Logf("Charlie: %s", charlie.ID()[:8])

	// ════════════════════════════════════════════════════════════════
	// Phase 2: 加入 Realm
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 2: 加入 Realm")

	realmAlice := testutil.NewTestRealm(t, alice).WithPSK(psk).Join()
	realmBob := testutil.NewTestRealm(t, bob).WithPSK(psk).Join()
	realmCharlie := testutil.NewTestRealm(t, charlie).WithPSK(psk).Join()

	// 建立连接
	bob.Host().Connect(ctx, alice.ID(), alice.ListenAddrs())
	charlie.Host().Connect(ctx, alice.ID(), alice.ListenAddrs())

	// 等待所有节点成员发现完成
	testutil.WaitForMembers(t, realmAlice, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmBob, 3, 45*time.Second)
	testutil.WaitForMembers(t, realmCharlie, 3, 45*time.Second)
	t.Logf("成员发现完成: Alice=%d, Bob=%d, Charlie=%d",
		len(realmAlice.Members()), len(realmBob.Members()), len(realmCharlie.Members()))

	// ════════════════════════════════════════════════════════════════
	// Phase 3: 群聊测试
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 3: 群聊测试")

	topicAlice, err := realmAlice.PubSub().Join("chat/general")
	require.NoError(t, err)
	topicBob, err := realmBob.PubSub().Join("chat/general")
	require.NoError(t, err)
	topicCharlie, err := realmCharlie.PubSub().Join("chat/general")
	require.NoError(t, err)

	subBob, err := topicBob.Subscribe()
	require.NoError(t, err)
	defer subBob.Cancel()

	subCharlie, err := topicCharlie.Subscribe()
	require.NoError(t, err)
	defer subCharlie.Cancel()

	// 等待订阅传播
	time.Sleep(2 * time.Second)

	// Alice 发送群消息
	groupMsg := "Alice: 大家好!"
	err = topicAlice.Publish(ctx, []byte(groupMsg))
	require.NoError(t, err)
	t.Logf("Alice 发送群消息: %s", groupMsg)

	// Bob 和 Charlie 收到
	msgBob := testutil.WaitForMessage(t, subBob, 15*time.Second)
	msgCharlie := testutil.WaitForMessage(t, subCharlie, 15*time.Second)

	assert.Contains(t, string(msgBob.Data), "大家好", "Bob 应收到群消息")
	assert.Contains(t, string(msgCharlie.Data), "大家好", "Charlie 应收到群消息")
	assert.Equal(t, alice.ID(), msgBob.From, "消息来源应为 Alice")
	assert.Equal(t, alice.ID(), msgCharlie.From, "消息来源应为 Alice")

	t.Log("✅ 群聊测试通过")

	// ════════════════════════════════════════════════════════════════
	// Phase 4: 私聊测试
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 4: 私聊测试")

	privateReceived := make(chan string, 1)
	err = realmBob.Streams().RegisterHandler("private-chat", func(stream *dep2p.BiStream) {
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			stream.Close()
			return
		}
		privateReceived <- string(buf[:n])
	})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	stream, err := realmAlice.Streams().Open(ctx, bob.ID(), "private-chat")
	require.NoError(t, err, "打开私聊流失败")

	privateMsg := "Alice: 这是私密消息"
	_, err = stream.Write([]byte(privateMsg))
	require.NoError(t, err)

	// 等待 Bob 收到消息后再关闭流
	select {
	case msg := <-privateReceived:
		assert.Contains(t, msg, "私密消息", "Bob 应收到私聊消息")
		t.Log("✅ 私聊测试通过")
	case <-time.After(15 * time.Second):
		t.Fatal("私聊消息未收到")
	}
	
	stream.Close()

	// ════════════════════════════════════════════════════════════════
	// Phase 5: 成员退出测试
	// ════════════════════════════════════════════════════════════════
	t.Log("Phase 5: 成员退出测试")

	initialCountAlice := len(realmAlice.Members())
	initialCountBob := len(realmBob.Members())
	assert.Equal(t, 3, initialCountAlice, "初始应有 3 个成员")
	assert.Equal(t, 3, initialCountBob, "初始应有 3 个成员")

	// Charlie 退出
	charlie.Close()
	time.Sleep(3 * time.Second)

	// 验证成员减少
	testutil.Eventually(t, 15*time.Second, func() bool {
		return len(realmAlice.Members()) < initialCountAlice
	}, "Charlie 退出后成员数应减少")

	finalCountAlice := len(realmAlice.Members())
	finalCountBob := len(realmBob.Members())

	assert.Less(t, finalCountAlice, initialCountAlice, "Alice 的成员数应减少")
	assert.Less(t, finalCountBob, initialCountBob, "Bob 的成员数应减少")
	assert.GreaterOrEqual(t, finalCountAlice, 2, "至少应有 2 个成员 (Alice 和 Bob)")

	t.Logf("✅ 成员退出测试通过: Alice=%d→%d, Bob=%d→%d",
		initialCountAlice, finalCountAlice, initialCountBob, finalCountBob)

	t.Log("✅ 完整聊天场景测试通过")
}

// TestE2E_ChatScenario_GroupChatOnly 测试仅群聊场景
//
// 简化版测试，只验证群聊功能。
func TestE2E_ChatScenario_GroupChatOnly(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过 E2E 测试")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点
	alice := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	bob := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmAlice := testutil.NewTestRealm(t, alice).WithPSK(psk).Join()
	realmBob := testutil.NewTestRealm(t, bob).WithPSK(psk).Join()

	// 2. 连接并等待认证
	bob.Host().Connect(ctx, alice.ID(), alice.ListenAddrs())
	testutil.WaitForMembers(t, realmAlice, 2, 30*time.Second)

	// 3. 群聊
	topicAlice, _ := realmAlice.PubSub().Join("chat/general")
	topicBob, _ := realmBob.PubSub().Join("chat/general")
	subBob, _ := topicBob.Subscribe()
	defer subBob.Cancel()

	time.Sleep(2 * time.Second)

	topicAlice.Publish(ctx, []byte("Hello from Alice"))
	msg := testutil.WaitForMessage(t, subBob, 10*time.Second)

	assert.Contains(t, string(msg.Data), "Hello from Alice")
	t.Log("✅ 群聊场景测试通过")
}
